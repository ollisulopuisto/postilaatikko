package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/carddav"
	"github.com/hkdb/aerion/internal/logging"
	"github.com/hkdb/aerion/internal/oauth2"
	"github.com/hkdb/aerion/internal/platform"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// A Contacts session cannot consume or overwrite pending Mail authorization.
type pendingContactOAuth struct {
	manager  *oauth2.Manager
	provider oauth2.ProviderConfig
	tokens   *oauth2.TokenResponse
	email    string
}

// Legacy standalone sources used shipped credentials and stored "google" /
// "microsoft". Preserve that client for refresh, but use Contacts configuration.
// Newly authorized sources store the explicit *-contacts provider.
// contactRefreshProvider delegates to the shared helper so the desktop and
// the headless helper resolve contact token providers identically.
func contactRefreshProvider(name string) (oauth2.ProviderConfig, error) {
	return oauth2.ContactRefreshProvider(name)
}

func (a *App) prepareContactOAuth(provider, email string) (*pendingContactOAuth, string, error) {
	if provider != "google" && provider != "microsoft" {
		return nil, "", fmt.Errorf("unsupported provider for contacts: %s", provider)
	}
	cfg, err := oauth2.GetProvider(provider + "-contacts")
	if err != nil {
		return nil, "", err
	}
	cfg.LoginHint = email
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	if a.contactOAuthManager != nil {
		return nil, "", fmt.Errorf("a Contacts sign-in is already in progress")
	}
	if a.pendingContactOAuth != nil {
		a.pendingContactOAuth.tokens.Clear()
		a.pendingContactOAuth = nil
	}
	session := &pendingContactOAuth{manager: oauth2.NewManager(), provider: cfg}
	authURL, err := session.manager.StartAuthFlowWithProvider(a.ctx, &cfg)
	if err != nil {
		return nil, "", err
	}
	a.contactOAuthManager = session.manager
	return session, authURL, nil
}

func (a *App) beginContactOAuth(provider, email string) (*pendingContactOAuth, error) {
	session, authURL, err := a.prepareContactOAuth(provider, email)
	if err != nil {
		return nil, err
	}
	wailsRuntime.EventsEmit(a.ctx, "contact-source-oauth:started", map[string]interface{}{
		"provider": provider, "authURL": authURL,
	})
	if err := platform.PortalOpenURI(authURL); err != nil {
		wailsRuntime.BrowserOpenURL(a.ctx, authURL)
	}
	return session, nil
}

func (a *App) waitContactOAuth(session *pendingContactOAuth, expectedEmail string) error {
	tokens, email, err := session.manager.WaitForCallback(a.ctx)
	if err != nil {
		return err
	}
	if err := validateContactGrant(session.provider, tokens, email, expectedEmail); err != nil {
		tokens.Clear()
		return err
	}
	session.tokens, session.email = tokens, email
	return nil
}

func validateContactGrant(provider oauth2.ProviderConfig, tokens *oauth2.TokenResponse, email, expectedEmail string) error {
	if tokens == nil || tokens.AccessToken == "" {
		return fmt.Errorf("Contacts authorization returned no access token")
	}
	if strings.TrimSpace(email) == "" {
		return fmt.Errorf("Contacts authorization did not return an email address; sign in again")
	}
	if expectedEmail != "" && !strings.EqualFold(email, expectedEmail) {
		return fmt.Errorf("Contacts account %q does not match selected account %q", email, expectedEmail)
	}
	// OAuth permits omission of scope when it equals the requested set.
	// A partial grant must not be recorded as if Contacts access were granted.
	if tokens.Scope != "" {
		granted := strings.Fields(tokens.Scope)
		covered := slices.Contains(granted, provider.Scopes[0])
		// Microsoft may return the short Graph scope instead of its URI.
		if provider.Name == "microsoft-contacts" {
			covered = covered || slices.Contains(granted, "Contacts.Read")
		}
		if !covered {
			return fmt.Errorf("Contacts read permission was not granted; sign in again and allow Contacts")
		}
	}
	return nil
}

func (a *App) finishContactOAuth(session *pendingContactOAuth) {
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	if a.contactOAuthManager == session.manager {
		a.contactOAuthManager = nil
	}
	session.manager.CancelAuthFlow()
	session.tokens.Clear()
}

// StartContactsOnlyOAuthFlow starts optional Contacts authorization. Mail has
// its own OAuth manager, pending response and credential namespace.
func (a *App) StartContactsOnlyOAuthFlow(provider string) error {
	// Remote mode: the interactive OAuth flow runs on the desktop; only
	// persistence goes through the helper (saveContactAuthorization branches).
	if !a.remoteContactsActive() {
		if err := a.requireLocalContacts(); err != nil {
			return err
		}
	}
	session, err := a.beginContactOAuth(provider, "")
	if err != nil {
		return err
	}
	go func() {
		defer recoverPanic("app.contacts-oauth", "Contacts OAuth callback")
		err := a.waitContactOAuth(session, "")
		a.contactOAuthMu.Lock()
		defer a.contactOAuthMu.Unlock()
		if a.contactOAuthManager != session.manager {
			session.tokens.Clear()
			return // cancelled; do not resurrect pending tokens
		}
		a.contactOAuthManager = nil
		session.manager.CancelAuthFlow()
		if err != nil {
			session.tokens.Clear()
			wailsRuntime.EventsEmit(a.ctx, "contact-source-oauth:error", map[string]interface{}{"error": err.Error()})
			return
		}
		a.pendingContactOAuth = session
		wailsRuntime.EventsEmit(a.ctx, "contact-source-oauth:success", map[string]interface{}{
			"provider": provider, "email": session.email, "expiresIn": session.tokens.ExpiresIn,
		})
	}()
	return nil
}

// CompleteContactSourceOAuthSetup only reads the dedicated Contacts response.
func (a *App) CompleteContactSourceOAuthSetup(name string, syncInterval int) (*carddav.Source, error) {
	// Remote mode: the interactive OAuth flow runs on the desktop; only
	// persistence goes through the helper (saveContactAuthorization branches).
	if !a.remoteContactsActive() {
		if err := a.requireLocalContacts(); err != nil {
			return nil, err
		}
	}
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	session := a.pendingContactOAuth
	if session == nil || session.tokens == nil {
		return nil, fmt.Errorf("no pending Contacts authorization; sign in to Contacts first")
	}
	source, err := a.saveContactAuthorization(session, "", "", name, syncInterval)
	if err != nil {
		return nil, err
	}
	session.tokens.Clear()
	a.pendingContactOAuth = nil
	return source, nil
}

// LinkAccountContactSource treats Mail as an identity hint, never as a token
// source. The explicit user action opens separate Contacts consent and waits.
func (a *App) LinkAccountContactSource(accountID, name string, syncInterval int) (*carddav.Source, error) {
	if a.remoteContactsActive() {
		return a.linkAccountContactSourceRemote(accountID, name, syncInterval)
	}
	if err := a.requireLocalContacts(); err != nil {
		return nil, err
	}
	acc, err := a.accountStore.Get(accountID)
	if err != nil {
		return nil, err
	}
	if acc == nil || acc.AuthType != account.AuthOAuth2 {
		return nil, fmt.Errorf("an OAuth email account is required")
	}
	provider, err := a.credStore.GetOAuthProvider(accountID)
	if err != nil {
		return nil, err
	}
	existing, err := a.carddavStore.GetSourceByAccountID(accountID)
	if err != nil {
		return nil, err
	}
	sourceID := ""
	if existing != nil {
		if a.credStore.HasContactSourceOAuthTokens(existing.ID) {
			return nil, fmt.Errorf("account already has a Contacts source; reauthorize that source in Contacts settings")
		}
		sourceID = existing.ID // migrate legacy source in place, preserving cache
	}
	session, err := a.beginContactOAuth(provider, acc.Email)
	if err != nil {
		return nil, err
	}
	defer a.finishContactOAuth(session)
	if err := a.waitContactOAuth(session, acc.Email); err != nil {
		return nil, err
	}
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	if a.contactOAuthManager != session.manager {
		return nil, fmt.Errorf("Contacts sign-in was cancelled")
	}
	return a.saveContactAuthorization(session, sourceID, accountID, name, syncInterval)
}

// ReauthorizeContactSource migrates legacy Google sources and repairs expired
// or revoked Contacts grants without touching Gmail. Cached contacts survive.
func (a *App) ReauthorizeContactSource(sourceID string) error {
	if a.remoteContactsActive() {
		return a.reauthorizeContactSourceRemote(sourceID)
	}
	if err := a.requireLocalContacts(); err != nil {
		return err
	}
	source, err := a.carddavStore.GetSource(sourceID)
	if err != nil {
		return err
	}
	if source == nil {
		return fmt.Errorf("contact source not found")
	}
	email := source.Username
	if email == "" && source.AccountID != nil {
		acc, err := a.accountStore.Get(*source.AccountID)
		if err != nil {
			return err
		}
		if acc != nil {
			email = acc.Email
		}
	}
	session, err := a.beginContactOAuth(string(source.Type), email)
	if err != nil {
		return err
	}
	defer a.finishContactOAuth(session)
	if err := a.waitContactOAuth(session, email); err != nil {
		return err
	}
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	if a.contactOAuthManager != session.manager {
		return fmt.Errorf("Contacts sign-in was cancelled")
	}
	accountID := ""
	if source.AccountID != nil {
		accountID = *source.AccountID
	}
	_, err = a.saveContactAuthorization(session, source.ID, accountID, source.Name, source.SyncInterval)
	return err
}

func (a *App) saveContactAuthorization(session *pendingContactOAuth, sourceID, accountID, name string, interval int) (*carddav.Source, error) {
	if a.remoteContactsActive() {
		client, err := a.remoteContactsClient()
		if err != nil {
			return nil, err
		}
		source, err := client.SaveContactAuthorization(a.ctx, v1.ContactsAuthorizationRequest{
			SourceID: sourceID, AccountID: accountID, Name: name, SyncInterval: interval,
			Provider: session.provider.Name, Email: session.email,
			Tokens: v1.ContactsTokenSet{
				AccessToken: session.tokens.AccessToken, RefreshToken: session.tokens.RefreshToken,
				ExpiresIn: session.tokens.ExpiresIn,
			},
		}, contactsIdempotencyKey("authorize"))
		if err != nil {
			return nil, remoteContactsError(err, v1.CapabilityContactsManage)
		}
		return source, nil
	}
	source, err := carddav.SaveAuthorization(a.carddavStore, a.credStore, &a.contactTokenMu, carddav.AuthorizationParams{
		SourceID: sourceID, AccountID: accountID, Name: name,
		SyncInterval: interval, Provider: session.provider.Name, Email: session.email,
		AccessToken: session.tokens.AccessToken, RefreshToken: session.tokens.RefreshToken,
		ExpiresIn: session.tokens.ExpiresIn, Scopes: session.provider.Scopes,
	})
	if err != nil {
		return nil, err
	}
	if a.carddavSyncer != nil {
		log := logging.WithComponent("app")
		go func(id string) {
			if err := a.carddavSyncer.SyncSource(id); err != nil {
				log.Warn().Err(err).Str("source_id", id).Msg("Background contact source sync failed")
			}
		}(source.ID)
	}
	return source, nil
}

func (a *App) clearContactOAuth() {
	a.contactOAuthMu.Lock()
	defer a.contactOAuthMu.Unlock()
	if a.contactOAuthManager != nil {
		a.contactOAuthManager.CancelAuthFlow()
		a.contactOAuthManager = nil
	}
	if a.pendingContactOAuth != nil {
		a.pendingContactOAuth.tokens.Clear()
		a.pendingContactOAuth = nil
	}
}

func (a *App) CancelContactSourceOAuthFlow() {
	// The interactive flow always runs on the desktop, including remote
	// mode, so the desktop-local OAuth state always gets cleared.
	a.clearContactOAuth()
	wailsRuntime.EventsEmit(a.ctx, "contact-source-oauth:cancelled", nil)
}
