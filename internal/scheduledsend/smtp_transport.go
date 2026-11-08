package scheduledsend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/certificate"
	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/imap"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/oauth2"
	"github.com/hkdb/aerion/internal/smtp"
	"github.com/hkdb/aerion/internal/syncowner"
)

// SMTPTransport uses accounts and credentials configured locally in the helper's
// Postilaatikko installation. No credential is accepted from a queued command.
type SMTPTransport struct {
	db            *database.DB
	schemaOwner   *syncowner.Lock
	accounts      *account.Store
	creds         *credentials.Store
	certs         *certificate.Store
	folders       *folder.Store
	oauth         *oauth2.Manager
	triageLabelMu sync.Mutex
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
	owner, err := syncowner.TryAcquireShared(filepath.Join(dataDirectory, "mail-schema.owner.lock"))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("protect helper database schema: %w", err)
	}
	current, err := db.CurrentSchemaVersion()
	if err != nil {
		_ = owner.Release()
		_ = db.Close()
		return nil, err
	}
	if current != database.MaxSchemaVersion() {
		_ = owner.Release()
		_ = db.Close()
		return nil, fmt.Errorf("helper database schema is %d; run helper bootstrap for schema %d", current, database.MaxSchemaVersion())
	}
	creds, err := credentials.NewStore(db.DB, dataDirectory)
	if err != nil {
		_ = owner.Release()
		_ = db.Close()
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
	return &SMTPTransport{db: db, schemaOwner: owner, accounts: account.NewStore(db), creds: creds,
		certs: certificate.NewStore(db.DB), folders: folder.NewStore(db), oauth: oauth2.NewManager()}, nil
}

// Close releases the helper-local account database after delivery has stopped.
func (t *SMTPTransport) Close() error {
	return errors.Join(t.db.Close(), t.schemaOwner.Release())
}

// DeleteAccountCredentials removes helper-local credentials before account data is deleted.
func (t *SMTPTransport) DeleteAccountCredentials(accountID string) error {
	if t == nil || t.creds == nil {
		return errors.New("helper credential store is unavailable")
	}
	return t.creds.DeleteAllCredentials(accountID)
}

// AccountMutationFingerprint returns a device-keyed request digest that does
// not enable offline guesses of password values stored in mutation receipts.
func (t *SMTPTransport) AccountMutationFingerprint(value []byte) []byte {
	if t == nil || t.creds == nil {
		return nil
	}
	return t.creds.Fingerprint(value)
}

// ApplyAccountPasswordMutation updates helper-local IMAP/SMTP passwords and
// commits the account row only after the secret writes succeed. Any failed
// apply or account update restores the original credential snapshots.
func (t *SMTPTransport) ApplyAccountPasswordMutation(accountID string, mutation v1.PasswordCredentialMutation, commitAccount func() error) error {
	defer mutation.ClearSecrets()
	if err := mutation.Normalize(); err != nil {
		return err
	}
	if t == nil || t.creds == nil {
		return errors.New("helper credential store is unavailable")
	}
	if !mutation.HasChanges() {
		if commitAccount != nil {
			return commitAccount()
		}
		return nil
	}

	var imapSnapshot, smtpSnapshot credentials.CredentialSnapshot
	var captureIMAP, captureSMTP bool
	var err error
	if mutation.IMAPAction != v1.PasswordKeep {
		imapSnapshot, err = t.creds.CapturePassword(accountID)
		if err != nil {
			return fmt.Errorf("capture IMAP password state: %w", err)
		}
		captureIMAP = true
	}
	if mutation.SMTPAction != v1.PasswordKeep {
		smtpSnapshot, err = t.creds.CaptureSMTPPassword(accountID)
		if err != nil {
			return fmt.Errorf("capture SMTP password state: %w", err)
		}
		captureSMTP = true
	}

	attemptedIMAP, attemptedSMTP := false, false
	apply := func(action v1.PasswordAction, password string, smtpPassword bool) error {
		if action == v1.PasswordKeep {
			return nil
		}
		if smtpPassword {
			attemptedSMTP = true
			if action == v1.PasswordReplace {
				return t.creds.SetSMTPPassword(accountID, password)
			}
			return t.creds.DeleteSMTPPassword(accountID)
		}
		attemptedIMAP = true
		if action == v1.PasswordReplace {
			return t.creds.SetPassword(accountID, password)
		}
		return t.creds.DeletePassword(accountID)
	}

	operationErr := apply(mutation.IMAPAction, mutation.IMAPPassword, false)
	if operationErr == nil {
		operationErr = apply(mutation.SMTPAction, mutation.SMTPPassword, true)
	}
	if operationErr == nil && commitAccount != nil {
		operationErr = commitAccount()
	}
	if operationErr == nil {
		return nil
	}

	var restoreErrs []error
	if attemptedSMTP && captureSMTP {
		if err := t.creds.RestoreSMTPPassword(accountID, smtpSnapshot); err != nil {
			restoreErrs = append(restoreErrs, err)
		}
	}
	if attemptedIMAP && captureIMAP {
		if err := t.creds.RestorePassword(accountID, imapSnapshot); err != nil {
			restoreErrs = append(restoreErrs, err)
		}
	}
	if restoreErr := errors.Join(restoreErrs...); restoreErr != nil {
		return errors.Join(v1.ErrCredentialRepairRequired, operationErr, restoreErr)
	}
	return operationErr
}

// ApplyAccountOAuthMutation persists OAuth callback credentials and propagates
// Microsoft replacements to linked mailboxes as one compensating operation.
func (t *SMTPTransport) ApplyAccountOAuthMutation(accountID string, mutation v1.OAuthCredentialMutation) error {
	defer mutation.ClearSecrets()
	if err := mutation.Normalize(); err != nil {
		return err
	}
	if t == nil || t.creds == nil || t.accounts == nil {
		return errors.New("helper OAuth credential store is unavailable")
	}
	if mutation.Provider != "google" && mutation.Provider != "microsoft" && mutation.Provider != "custom" {
		return fmt.Errorf("unsupported OAuth provider %q", mutation.Provider)
	}
	accountRecord, err := t.accounts.Get(accountID)
	if err != nil {
		return err
	}
	if accountRecord.AuthType != account.AuthOAuth2 {
		return v1.InvalidRequest("OAuth credentials require an OAuth account", nil)
	}
	currentProvider, err := t.creds.GetOAuthProvider(accountID)
	if err != nil {
		return fmt.Errorf("read current OAuth provider: %w", err)
	}
	if currentProvider != "" && currentProvider != mutation.Provider {
		return v1.InvalidRequest("OAuth provider does not match the account's existing authorization", nil)
	}
	accountIDs := []string{accountID}
	if mutation.Provider == "microsoft" {
		children, err := t.accounts.ListBySharedMailboxParent(accountID)
		if err != nil {
			return fmt.Errorf("list linked Microsoft mailboxes: %w", err)
		}
		for _, child := range children {
			accountIDs = append(accountIDs, child.ID)
		}
	}

	type snapshot struct {
		id        string
		tokens    *credentials.OAuthTokens
		hasTokens bool
		custom    credentials.CustomOAuthProvider
		hasCustom bool
		stableID  string
	}
	snapshots := make([]snapshot, 0, len(accountIDs))
	for _, id := range accountIDs {
		current := snapshot{id: id}
		tokens, err := t.creds.GetOAuthTokens(id)
		if err == nil {
			copyTokens := *tokens
			copyTokens.Scopes = append([]string(nil), tokens.Scopes...)
			current.tokens, current.hasTokens = &copyTokens, true
		} else if !errors.Is(err, credentials.ErrCredentialNotFound) {
			return fmt.Errorf("capture OAuth token state: %w", err)
		}
		provider, ok, err := t.creds.GetCustomOAuthProvider(id)
		if err != nil {
			return fmt.Errorf("capture custom provider state: %w", err)
		}
		if ok {
			provider.Scopes = append([]string(nil), provider.Scopes...)
			current.custom, current.hasCustom = provider, true
		}
		current.stableID, err = t.accounts.GetOAuthStableID(id)
		if err != nil {
			return fmt.Errorf("capture OAuth identity state: %w", err)
		}
		snapshots = append(snapshots, current)
	}

	tokens := &credentials.OAuthTokens{
		Provider: mutation.Provider, AccessToken: mutation.AccessToken,
		RefreshToken: mutation.RefreshToken, ExpiresAt: mutation.ExpiresAt,
		Scopes: append([]string(nil), mutation.Scopes...),
	}
	var custom *credentials.CustomOAuthProvider
	if mutation.Custom != nil {
		custom = &credentials.CustomOAuthProvider{
			AuthURL: mutation.Custom.AuthURL, TokenURL: mutation.Custom.TokenURL,
			UserinfoEndpoint: mutation.Custom.UserinfoEndpoint,
			Scopes:           append([]string(nil), mutation.Custom.Scopes...),
			ClientID:         mutation.Custom.ClientID, ClientSecret: mutation.Custom.ClientSecret,
		}
	}

	attempted := 0
	var operationErr error
	for _, current := range snapshots {
		attempted++
		if err := t.creds.SetOAuthTokens(current.id, tokens); err != nil {
			operationErr = fmt.Errorf("store OAuth tokens: %w", err)
			break
		}
		if custom != nil {
			if err := t.creds.SetCustomOAuthProvider(current.id, *custom); err != nil {
				operationErr = fmt.Errorf("store custom OAuth provider: %w", err)
				break
			}
		}
		if mutation.StableID != "" {
			if err := t.accounts.SetOAuthStableID(current.id, mutation.StableID); err != nil {
				operationErr = fmt.Errorf("store OAuth stable identity: %w", err)
				break
			}
		}
	}
	if operationErr == nil {
		return nil
	}

	var restoreErrs []error
	for i := attempted - 1; i >= 0; i-- {
		current := snapshots[i]
		if current.hasTokens {
			if err := t.creds.SetOAuthTokens(current.id, current.tokens); err != nil {
				restoreErrs = append(restoreErrs, err)
			}
		} else if err := t.creds.DeleteOAuthMailTokens(current.id); err != nil {
			restoreErrs = append(restoreErrs, err)
		}
		if current.hasCustom {
			if err := t.creds.SetCustomOAuthProvider(current.id, current.custom); err != nil {
				restoreErrs = append(restoreErrs, err)
			}
		} else if err := t.creds.DeleteCustomOAuthProvider(current.id); err != nil {
			restoreErrs = append(restoreErrs, err)
		}
		if err := t.accounts.SetOAuthStableID(current.id, current.stableID); err != nil {
			restoreErrs = append(restoreErrs, err)
		}
	}
	if restoreErr := errors.Join(restoreErrs...); restoreErr != nil {
		return errors.Join(v1.ErrCredentialRepairRequired, operationErr, restoreErr)
	}
	return operationErr
}

// ValidateSharedMailbox verifies Microsoft OAuth IMAP access using the primary account's token.
func (t *SMTPTransport) ValidateSharedMailbox(ctx context.Context, primaryAccountID, sharedEmail string) error {
	if t == nil || t.creds == nil || t.accounts == nil {
		return errors.New("helper account credentials are unavailable")
	}
	primary, err := t.accounts.Get(primaryAccountID)
	if err != nil {
		return err
	}
	if primary == nil || primary.AuthType != account.AuthOAuth2 {
		return errors.New("shared mailboxes require a Microsoft OAuth account")
	}
	provider, err := t.creds.GetOAuthProvider(primaryAccountID)
	if err != nil || provider != "microsoft" {
		return errors.New("shared mailboxes require Microsoft OAuth credentials")
	}
	tokens, err := t.tokens(ctx, primaryAccountID)
	if err != nil {
		return err
	}
	config := imap.DefaultConfig()
	config.Host, config.Port, config.Username = primary.IMAPHost, primary.IMAPPort, sharedEmail
	config.Security, config.AuthType, config.AccessToken = imap.SecurityType(primary.IMAPSecurity), imap.AuthTypeOAuth2, tokens.AccessToken
	config.TLSConfig = certificate.BuildTLSConfig(primary.IMAPHost, t.certs)
	client := imap.NewClient(config)
	if err := client.Connect(); err != nil {
		return fmt.Errorf("connect to shared mailbox IMAP: %w", err)
	}
	if err := client.Login(); err != nil {
		client.Close()
		return fmt.Errorf("shared mailbox authentication failed: %w", err)
	}
	client.Close()
	return nil
}

// CopySharedMailboxCredentials stores the primary OAuth grant for a child account.
func (t *SMTPTransport) CopySharedMailboxCredentials(primaryAccountID, childAccountID string) error {
	if t == nil || t.creds == nil {
		return errors.New("helper account credentials are unavailable")
	}
	provider, err := t.creds.GetOAuthProvider(primaryAccountID)
	if err != nil || provider != "microsoft" {
		return errors.New("shared mailbox credentials require Microsoft OAuth")
	}
	tokens, err := t.creds.GetOAuthTokens(primaryAccountID)
	if err != nil {
		return err
	}
	return t.creds.SetOAuthTokens(childAccountID, tokens)
}

// Accounts lists helper-local account IDs for the explicit pairing/setup flow.
func (t *SMTPTransport) Accounts() ([]*account.Account, error) { return t.accounts.List() }

// IMAPClientConfig returns the same locally configured credentials used by
// scheduled delivery, for the headless metadata-sync worker. It never accepts
// credentials over the service API.
func (t *SMTPTransport) IMAPClientConfig(ctx context.Context, accountID string) (*imap.ClientConfig, error) {
	acc, err := t.accounts.Get(accountID)
	if err != nil {
		return nil, err
	}
	if acc == nil || !acc.Enabled || acc.SharedMailboxParentID != "" {
		return nil, errors.New("helper sync account unavailable or unsupported")
	}
	config := imap.DefaultConfig()
	config.Host, config.Port, config.Username = acc.IMAPHost, acc.IMAPPort, acc.Username
	config.Security, config.AuthMechanism = imap.SecurityType(acc.IMAPSecurity), string(acc.IMAPAuthMechanism)
	config.TLSConfig = certificate.BuildTLSConfig(acc.IMAPHost, t.certs)
	if acc.AuthType == account.AuthOAuth2 {
		tokens, err := t.tokens(ctx, acc.ID)
		if err != nil {
			return nil, err
		}
		config.AuthType, config.AccessToken = imap.AuthTypeOAuth2, tokens.AccessToken
	} else {
		password, err := t.creds.GetPassword(acc.ID)
		if err != nil {
			return nil, err
		}
		config.AuthType, config.Password = imap.AuthTypePassword, password
	}
	return &config, nil
}

// GmailTokenSource supplies access tokens to the headless sync engine from
// helper-local OAuth storage. Tokens never cross the mailbox API boundary.
func (t *SMTPTransport) GmailTokenSource(ctx context.Context, accountID string, forceRefresh bool) (string, error) {
	acc, err := t.accounts.Get(accountID)
	if err != nil {
		return "", err
	}
	if acc == nil || acc.AuthType != account.AuthOAuth2 || !strings.EqualFold(strings.TrimSpace(acc.IMAPHost), "imap.gmail.com") {
		return "", gmail.ErrNotCapable
	}
	tokens, err := t.creds.GetOAuthTokens(accountID)
	if errors.Is(err, credentials.ErrCredentialNotFound) || (err == nil && (tokens == nil || (tokens.Provider != "google" && tokens.Provider != "custom"))) {
		return "", gmail.ErrNotCapable
	}
	if err != nil {
		return "", fmt.Errorf("load helper Gmail OAuth credentials: %w", err)
	}
	if !gmail.ScopesCoverSearch(tokens.Scopes) {
		return "", gmail.ErrReauthRequired
	}
	if forceRefresh {
		tokens, err = t.tokensWithForce(ctx, accountID, true)
	} else {
		tokens, err = t.tokens(ctx, accountID)
	}
	if err != nil {
		return "", err
	}
	if tokens == nil || strings.TrimSpace(tokens.AccessToken) == "" {
		return "", gmail.ErrReauthRequired
	}
	return tokens.AccessToken, nil
}

// SetGmailTriageLabel mirrors Reply Later's Gmail label behavior for helper API
// operations. Accounts without Gmail modify scope keep app-local queue state.
func (t *SMTPTransport) SetGmailTriageLabel(ctx context.Context, accountID, conversationID, queue string, enabled bool) error {
	acc, err := t.accounts.Get(accountID)
	if err != nil {
		return err
	}
	if acc == nil || acc.AuthType != account.AuthOAuth2 || !strings.EqualFold(strings.TrimSpace(acc.IMAPHost), "imap.gmail.com") {
		return nil
	}
	tokens, err := t.creds.GetOAuthTokens(accountID)
	if errors.Is(err, credentials.ErrCredentialNotFound) || (err == nil && (tokens == nil || (tokens.Provider != "google" && tokens.Provider != "custom"))) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load helper Gmail OAuth credentials: %w", err)
	}
	if !gmail.ScopesCoverModify(tokens.Scopes) {
		return nil
	}
	if strings.TrimSpace(conversationID) == "" {
		return errors.New("conversation ID is required")
	}
	labelNames := map[string]string{"read-later": "Postilaatikko/Triage/Read Later", "reply-later": "Postilaatikko/Triage/Reply Later"}
	labelName, ok := labelNames[queue]
	if !ok {
		return fmt.Errorf("unsupported Gmail triage queue: %s", queue)
	}
	t.triageLabelMu.Lock()
	defer t.triageLabelMu.Unlock()
	client := gmail.NewClient(func(ctx context.Context, force bool) (string, error) {
		var refreshed *credentials.OAuthTokens
		var refreshErr error
		if force {
			refreshed, refreshErr = t.tokensWithForce(ctx, accountID, true)
		} else {
			refreshed, refreshErr = t.tokens(ctx, accountID)
		}
		if refreshErr != nil {
			return "", refreshErr
		}
		if refreshed == nil || !gmail.ScopesCoverModify(refreshed.Scopes) {
			return "", gmail.ErrReauthRequired
		}
		return refreshed.AccessToken, nil
	})
	var labelID string
	lookupErr := t.db.QueryRowContext(ctx, `SELECT label_id FROM gmail_triage_labels WHERE account_id=? AND queue=?`, accountID, queue).Scan(&labelID)
	if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("load Gmail triage label: %w", lookupErr)
	}
	if labelID == "" {
		labels, err := client.Labels(ctx)
		if err != nil {
			return fmt.Errorf("list Gmail labels: %w", err)
		}
		for _, label := range labels {
			if label.Name == labelName && label.Type == "user" {
				labelID = label.ID
				break
			}
		}
		if labelID == "" {
			created, err := client.CreateHiddenUserLabel(ctx, labelName)
			if err != nil {
				return fmt.Errorf("create hidden Gmail triage label: %w", err)
			}
			labelID = created.ID
		}
		if _, err := t.db.ExecContext(ctx, `INSERT INTO gmail_triage_labels(account_id,queue,label_id) VALUES(?,?,?) ON CONFLICT(account_id,queue) DO UPDATE SET label_id=excluded.label_id`, accountID, queue, labelID); err != nil {
			return fmt.Errorf("save Gmail triage label: %w", err)
		}
	}
	threadID := strings.TrimPrefix(conversationID, "gmail:"+accountID+":")
	var add, remove []string
	if enabled {
		add = []string{labelID}
	} else {
		remove = []string{labelID}
	}
	if err := client.ModifyThread(ctx, threadID, add, remove); err != nil {
		return fmt.Errorf("update Gmail triage label: %w", err)
	}
	return nil
}

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
				// Token lookup/refresh runs before submission, or after an
				// explicit 401 rejection. It cannot mean mail was accepted.
				return "", &DeliveryError{Kind: AuthorizationRequired, Err: err}
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
		var localFailure *DeliveryError
		if errors.As(err, &localFailure) {
			return localFailure
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
	defer func() { _ = client.Abort() }()
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
