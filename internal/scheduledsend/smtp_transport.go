package scheduledsend

import (
	"context"
	"errors"
	"fmt"
	"github.com/hkdb/aerion/internal/gmail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/certificate"
	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/oauth2"
	"github.com/hkdb/aerion/internal/smtp"
)

// SMTPTransport uses accounts and credentials configured locally in the helper's
// Postilaatikko installation. No credential is accepted from a queued command.
type SMTPTransport struct {
	db       *database.DB
	accounts *account.Store
	creds    *credentials.Store
	certs    *certificate.Store
	folders  *folder.Store
	oauth    *oauth2.Manager
}

// OpenSMTPTransport opens an existing helper-local app database. Setup accounts
// in the mail app first; this command never creates a new empty account store.
func OpenSMTPTransport(dataDirectory string) (*SMTPTransport, error) {
	path := filepath.Join(dataDirectory, "aerion.db")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("configure the helper's mail accounts in Postilaatikko before starting delivery")
	}
	db, err := database.Open(path)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		return nil, err
	}
	creds, err := credentials.NewStore(db.DB, dataDirectory)
	if err != nil {
		db.Close()
		return nil, err
	}
	// These resolver hooks are process-wide, just as in app preflight. This
	// constructor is intended for the separate helper process, not a live app.
	oauth2.UserOverrideLookup = func(id string) (oauth2.ClientCredentials, bool) {
		clientID, secret, ok, err := creds.GetUserClientCreds(id)
		return oauth2.ClientCredentials{ClientID: clientID, ClientSecret: secret}, ok && err == nil
	}
	oauth2.SlotAliasLookup = func(id string) (string, bool) {
		target, ok, err := creds.GetOAuthSlotAlias(id)
		return target, ok && err == nil
	}
	oauth2.ActiveChoiceLookup = func(id string) (string, bool) {
		choice, err := creds.GetOAuthActiveChoice(id)
		return choice, err == nil && choice != ""
	}
	return &SMTPTransport{db: db, accounts: account.NewStore(db), creds: creds,
		certs: certificate.NewStore(db.DB), folders: folder.NewStore(db), oauth: oauth2.NewManager()}, nil
}

// Close releases the helper-local account database after delivery has stopped.
func (t *SMTPTransport) Close() error { return t.db.Close() }

// Accounts lists helper-local account IDs for the explicit pairing/setup flow.
func (t *SMTPTransport) Accounts() ([]*account.Account, error) { return t.accounts.List() }

// Deliver submits immutable MIME through SMTP. Once SMTP accepts the message,
// a Sent-copy failure is logged with a fixed code and never causes a resend.
func (t *SMTPTransport) Deliver(ctx context.Context, delivery Delivery) error {
	if err := ctx.Err(); err != nil {
		return &DeliveryError{Kind: SafeRetry, Err: err}
	}
	acc, err := t.accounts.Get(delivery.AccountID)
	if err != nil {
		return &DeliveryError{Kind: SafeRetry, Err: err}
	}
	if acc == nil || !acc.Enabled || acc.NoOutgoingServer || acc.SharedMailboxParentID != "" {
		return &DeliveryError{Kind: PermanentFailure, Err: errors.New("helper account unavailable or unsupported")}
	}
	if acc.SMTPSecurity != account.SecurityTLS && acc.SMTPSecurity != account.SecurityStartTLS {
		return &DeliveryError{Kind: PermanentFailure, Err: errors.New("scheduled delivery requires encrypted SMTP")}
	}
	identities, err := t.accounts.GetIdentities(acc.ID)
	if err != nil {
		return &DeliveryError{Kind: SafeRetry, Err: err}
	}
	allowed := strings.EqualFold(acc.Email, delivery.From)
	for _, identity := range identities {
		allowed = allowed || strings.EqualFold(identity.Email, delivery.From)
	}
	if !allowed {
		return &DeliveryError{Kind: PermanentFailure, Err: errors.New("sender is not a helper-local account identity")}
	}
	if acc.AuthType == account.AuthOAuth2 && strings.EqualFold(acc.IMAPHost, "imap.gmail.com") {
		c := gmail.NewClient(func(ctx context.Context, force bool) (string, error) {
			tokens, err := t.tokensWithForce(ctx, acc.ID, force)
			if err != nil {
				return "", err
			}
			if !gmail.ScopesCoverModify(tokens.Scopes) {
				return "", gmail.ErrReauthRequired
			}
			return tokens.AccessToken, nil
		})
		_, err := c.Send(ctx, delivery.RFC822, delivery.To...)
		if err == nil {
			return nil
		}
		kind := OutcomeUnknown
		if errors.Is(err, gmail.ErrReauthRequired) || errors.Is(err, gmail.ErrInsufficientScope) {
			kind = AuthorizationRequired
		}
		if errors.Is(err, gmail.ErrBadRequest) || errors.Is(err, gmail.ErrAPIDisabled) {
			kind = PermanentFailure
		}
		if errors.Is(err, gmail.ErrRateLimited) {
			kind = SafeRetry
		}
		return &DeliveryError{Kind: kind, Err: err}
	}

	config, err := t.smtpConfig(ctx, acc)
	if err != nil {
		return err
	}
	client := smtp.NewClient(config)
	if err := client.Connect(); err != nil {
		return &DeliveryError{Kind: SafeRetry, Err: err}
	}
	defer client.Abort()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Abort()
		case <-finished:
		}
	}()
	if err := ctx.Err(); err != nil {
		return &DeliveryError{Kind: SafeRetry, Err: err}
	}
	if err := client.Login(); err != nil {
		kind := AuthorizationRequired
		var response *textproto.Error
		if errors.As(err, &response) && response.Code >= 400 && response.Code < 500 {
			kind = SafeRetry
		}
		return &DeliveryError{Kind: kind, Err: err}
	}
	if err := client.SendMail(delivery.From, delivery.To, delivery.RFC822); err != nil {
		var failure *smtp.DeliveryError
		if !errors.As(err, &failure) || failure.MayHaveBeenAccepted {
			return &DeliveryError{Kind: OutcomeUnknown, Err: err}
		}
		kind := SafeRetry
		var response *textproto.Error
		if errors.As(err, &response) && response.Code >= 500 {
			kind = PermanentFailure
		}
		return &DeliveryError{Kind: kind, Err: err}
	}
	if !autoSavesSent(acc.IMAPHost) {
		if err := t.saveSent(ctx, acc, delivery.RFC822); err != nil {
			log := logging.WithComponent("scheduledsend")
			log.Warn().Str("code", "sent_copy_failed").Msg("Mail accepted; Sent copy needs attention")
			return &DeliveryError{Kind: AcceptedWithoutSentCopy, Err: err}
		}
	}
	return nil
}

func (t *SMTPTransport) smtpConfig(ctx context.Context, acc *account.Account) (smtp.ClientConfig, error) {
	config := smtp.DefaultConfig()
	config.Host, config.Port = acc.SMTPHost, acc.SMTPPort
	config.Security = smtp.SecurityType(acc.SMTPSecurity)
	config.AuthMechanism = string(acc.EffectiveSMTPAuthMechanism())
	config.Username = acc.Username
	config.TLSConfig = certificate.BuildTLSConfig(acc.SMTPHost, t.certs)
	config.RedactMessageMetadata = true
	if acc.AuthType == account.AuthOAuth2 {
		tokens, err := t.tokens(ctx, acc.ID)
		if err != nil {
			return config, &DeliveryError{Kind: AuthorizationRequired, Err: err}
		}
		config.AuthType, config.AccessToken = smtp.AuthTypeOAuth2, tokens.AccessToken
		return config, nil
	}
	config.AuthType = smtp.AuthTypePassword
	var err error
	if acc.SMTPUsername != "" {
		config.Username = acc.SMTPUsername
		config.Password, err = t.creds.GetSMTPPassword(acc.ID)
	}
	if acc.SMTPUsername == "" {
		config.Password, err = t.creds.GetPassword(acc.ID)
	}
	if err != nil {
		return config, &DeliveryError{Kind: AuthorizationRequired, Err: err}
	}
	return config, nil
}

func (t *SMTPTransport) tokens(ctx context.Context, accountID string) (*credentials.OAuthTokens, error) {
	return t.tokensWithForce(ctx, accountID, false)
}
func (t *SMTPTransport) tokensWithForce(ctx context.Context, accountID string, force bool) (*credentials.OAuthTokens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tokens, err := t.creds.GetOAuthTokens(accountID)
	if err != nil || (!force && !tokens.IsExpiringSoon(5*time.Minute)) {
		return tokens, err
	}
	var refreshed *oauth2.TokenResponse
	if tokens.Provider == "custom" {
		provider, ok, err := t.creds.GetCustomOAuthProvider(accountID)
		if err != nil || !ok {
			return nil, errors.New("helper custom OAuth configuration unavailable")
		}
		config := oauth2.CustomProviderConfig(provider.AuthURL, provider.TokenURL, provider.UserinfoEndpoint, provider.Scopes, provider.ClientID, provider.ClientSecret)
		refreshed, err = t.oauth.RefreshTokenWithProvider(config, tokens.RefreshToken)
		if err != nil {
			return nil, err
		}
	}
	if tokens.Provider != "custom" {
		refreshed, err = t.oauth.RefreshToken(tokens.Provider, tokens.RefreshToken)
		if err != nil {
			return nil, err
		}
	}
	tokens.AccessToken, tokens.RefreshToken = refreshed.AccessToken, refreshed.RefreshToken
	tokens.ExpiresAt = time.Now().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
	if err := t.creds.SetOAuthTokens(accountID, tokens); err != nil {
		return nil, err
	}
	return tokens, ctx.Err()
}

func autoSavesSent(host string) bool {
	switch strings.ToLower(host) {
	case "imap.gmail.com", "outlook.office365.com", "imap-mail.outlook.com":
		return true
	default:
		return false
	}
}

func (t *SMTPTransport) saveSent(ctx context.Context, acc *account.Account, raw []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if acc.IMAPSecurity != account.SecurityTLS && acc.IMAPSecurity != account.SecurityStartTLS {
		return errors.New("scheduled Sent copy requires encrypted IMAP")
	}
	sent, err := t.folders.GetByPath(acc.ID, acc.SentFolderPath)
	if err != nil || sent == nil {
		sent, err = t.folders.GetByType(acc.ID, folder.TypeSent)
	}
	if err != nil || sent == nil {
		return errors.New("no helper-local Sent folder configured")
	}
	config := imap.DefaultConfig()
	config.Host, config.Port, config.Username = acc.IMAPHost, acc.IMAPPort, acc.Username
	config.Security, config.AuthMechanism = imap.SecurityType(acc.IMAPSecurity), string(acc.IMAPAuthMechanism)
	config.TLSConfig = certificate.BuildTLSConfig(acc.IMAPHost, t.certs)
	config.ConnectTimeout, config.ReadTimeout, config.WriteTimeout = 15*time.Second, 15*time.Second, 15*time.Second
	if acc.AuthType == account.AuthOAuth2 {
		tokens, err := t.tokens(ctx, acc.ID)
		if err != nil {
			return err
		}
		config.AuthType, config.AccessToken = imap.AuthTypeOAuth2, tokens.AccessToken
	}
	if acc.AuthType != account.AuthOAuth2 {
		config.AuthType = imap.AuthTypePassword
		config.Password, err = t.creds.GetPassword(acc.ID)
		if err != nil {
			return err
		}
	}
	client := imap.NewClient(config)
	if err := client.Connect(); err != nil {
		return err
	}
	defer client.ForceClose()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			_ = client.ForceClose()
		case <-finished:
		}
	}()
	if err := client.Login(); err != nil {
		return err
	}
	_, err = client.AppendMessage(sent.Path, []goimap.Flag{goimap.FlagSeen}, time.Now(), raw)
	return err
}

// ValidateSender checks helper-side configuration during local pairing/setup.
func (t *SMTPTransport) ValidateSender(accountID, from string) error {
	acc, err := t.accounts.Get(accountID)
	if err != nil || acc == nil || acc.NoOutgoingServer {
		return errors.New("helper sending account not available")
	}
	if strings.EqualFold(acc.Email, from) {
		return nil
	}
	identities, err := t.accounts.GetIdentities(accountID)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if strings.EqualFold(identity.Email, from) {
			return nil
		}
	}
	return fmt.Errorf("sender is not configured for helper account")
}
