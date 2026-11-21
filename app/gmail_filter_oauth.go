package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
	"github.com/ollisulopuisto/postilaatikko/internal/platform"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AuthorizeGmailFilters requests settings access for one existing local account.
// It is called only by an explicit publication action, never by login or sync.
// Remote workspaces fail closed until helper-owned consent is implemented.
// No filter is created, deleted or applied by this method.
func (a *App) AuthorizeGmailFilters(accountID string) error {
	if !a.gmailFilterOAuthMu.TryLock() {
		return fmt.Errorf("Gmail filter authorization already in progress")
	}
	defer a.gmailFilterOAuthMu.Unlock()
	email, previous, cfg, err := a.prepareGmailFilterConsent(accountID)
	if err != nil {
		return err
	}
	defer previous.Clear()
	if gmail.ScopesCoverFilters(previous.Scopes) {
		return nil
	}
	if a.ctx == nil {
		return fmt.Errorf("application context unavailable")
	}
	manager := oauth2.NewManager()
	defer manager.CancelAuthFlow()
	authURL, err := manager.StartAuthFlowWithProvider(a.ctx, &cfg)
	if err != nil {
		return err
	}
	wailsRuntime.EventsEmit(a.ctx, "gmail-filter-oauth:started", map[string]interface{}{"accountId": accountID, "email": email, "authURL": authURL})
	if err := platform.PortalOpenURI(authURL); err != nil {
		wailsRuntime.BrowserOpenURL(a.ctx, authURL)
	}
	response, actualEmail, err := manager.WaitForCallback(a.ctx)
	if response != nil {
		defer response.Clear()
	}
	if err != nil {
		return err
	}
	return a.persistGmailFilterConsent(accountID, email, previous, cfg.ClientID, response, actualEmail)
}

func (a *App) prepareGmailFilterConsent(accountID string) (string, *credentials.OAuthTokens, oauth2.ProviderConfig, error) {
	empty := oauth2.ProviderConfig{}
	if a.mailboxService != nil && a.mailboxService.isRemote() {
		return "", nil, empty, fmt.Errorf("Gmail filter authorization requires the account-owning local workspace")
	}
	if a.accountStore == nil || a.credStore == nil {
		return "", nil, empty, fmt.Errorf("account credentials unavailable")
	}
	acc, err := a.accountStore.Get(accountID)
	if err != nil {
		return "", nil, empty, err
	}
	if acc == nil || !acc.Enabled || acc.AuthType != account.AuthOAuth2 || acc.IMAPHost != gmailIMAPHost || strings.TrimSpace(acc.Email) == "" {
		return "", nil, empty, fmt.Errorf("select an enabled Google OAuth Gmail account")
	}
	previous, err := a.credStore.GetOAuthTokens(accountID)
	if err != nil {
		return "", nil, empty, err
	}
	if previous == nil || previous.Provider != "google" || previous.RefreshToken == "" {
		if previous != nil {
			previous.Clear()
		}
		return "", nil, empty, fmt.Errorf("existing Google mail grant required")
	}
	cfg, err := oauth2.GoogleFilterManagementProvider(acc.Email, previous.Scopes)
	if err != nil {
		previous.Clear()
		return "", nil, empty, err
	}
	resolved, resolveErr := oauth2.GetProvider("google")
	if resolveErr != nil {
		previous.Clear()
		return "", nil, empty, resolveErr
	}
	cfg.ClientID, cfg.ClientSecret = resolved.ClientID, resolved.ClientSecret
	if cfg.ClientID == "" {
		previous.Clear()
		return "", nil, empty, fmt.Errorf("Google mail OAuth client not configured")
	}
	return acc.Email, previous, cfg, nil
}

func (a *App) persistGmailFilterConsent(accountID, expectedEmail string, previous *credentials.OAuthTokens, clientID string, response *oauth2.TokenResponse, actualEmail string) error {
	// Keep ordinary Mail refreshes outside the entire consent verification/write/
	// readback interval. CAS still rejects independent credential replacement.
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	releaseRefresh, err := credentials.LockOAuthRefresh(ctx, accountID)
	if err != nil {
		return err
	}
	defer releaseRefresh()
	if previous == nil {
		return fmt.Errorf("previous grant missing")
	}
	if err := oauth2.ValidateGoogleFilterGrant(expectedEmail, actualEmail, previous.Scopes, response); err != nil {
		return err
	}
	email, current, cfg, err := a.prepareGmailFilterConsent(accountID)
	if err != nil {
		return err
	}
	defer current.Clear()
	// An account removal, credential replacement or project switch while the
	// browser was open invalidates this session. Never overwrite newer grants.
	oldScopes, currentScopes := slices.Clone(previous.Scopes), slices.Clone(current.Scopes)
	slices.Sort(oldScopes)
	slices.Sort(currentScopes)
	if !strings.EqualFold(email, expectedEmail) || cfg.ClientID != clientID || current.RefreshToken != previous.RefreshToken || !slices.Equal(oldScopes, currentScopes) {
		return fmt.Errorf("Gmail account or grant changed during authorization; start again")
	}
	refreshToken := response.RefreshToken
	if refreshToken == "" {
		refreshToken = current.RefreshToken
	}
	// Prove that the durable refresh grant also carries settings access. The
	// incremental access token alone is not proof when Google reuses a grant.
	verified, err := oauth2.NewManager().VerifyGoogleFilterRefresh(cfg, refreshToken, previous.Scopes)
	if err != nil {
		return fmt.Errorf("verify durable Gmail settings grant: %w", err)
	}
	defer verified.Clear()
	response = verified
	refreshToken = verified.RefreshToken
	tokens := &credentials.OAuthTokens{Provider: "google", AccessToken: response.AccessToken, RefreshToken: refreshToken, ExpiresAt: time.Now().Add(time.Duration(response.ExpiresIn) * time.Second), Scopes: strings.Fields(response.Scope)}
	defer tokens.Clear()
	// The verification network call can outlive a workspace/account/client switch.
	// Revalidate context without replacing the pre-request credential CAS snapshot.
	verifiedEmail, verifiedCurrent, verifiedCfg, err := a.prepareGmailFilterConsent(accountID)
	if err != nil {
		return err
	}
	defer verifiedCurrent.Clear()
	if !strings.EqualFold(verifiedEmail, expectedEmail) || verifiedCfg.ClientID != clientID {
		return fmt.Errorf("Gmail context changed during verification; start again")
	}
	if err := a.credStore.CompareAndSwapOAuthTokens(accountID, current, tokens); err != nil {
		return fmt.Errorf("persist Gmail filter grant: %w", err)
	}
	saved, err := a.credStore.GetOAuthTokens(accountID)
	if err != nil {
		return fmt.Errorf("verify persisted Gmail filter grant: %w", err)
	}
	defer saved.Clear()
	if saved.Provider != "google" || saved.AccessToken != tokens.AccessToken || saved.RefreshToken != tokens.RefreshToken || !gmail.ScopesCoverFilters(saved.Scopes) {
		return fmt.Errorf("persisted Gmail filter grant could not be verified; sign in again")
	}
	return nil
}
