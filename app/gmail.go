package app

import (
	"context"
	"fmt"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/gmail"
)

// gmailIMAPHost is how Gmail accounts are recognized throughout the app.
const gmailIMAPHost = "imap.gmail.com"

// gmailTokenSource gives the Gmail API a bearer token for an account. It returns
// gmail.ErrNotCapable for accounts that cannot use the API (not Gmail, not
// Google OAuth, or granted scopes that do not cover search), so callers fall
// back to IMAP. Token refresh and re-authorization prompts reuse the same path
// as IMAP/SMTP; there is no second refresh implementation.
func (a *App) gmailTokenSource(_ context.Context, accountID string, forceRefresh bool) (string, error) {
	tokens, err := a.gmailTokensFor(accountID, gmail.ScopesCoverSearch, forceRefresh)
	if err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

func (a *App) gmailTokensFor(accountID string, scopesOK func([]string) bool, forceRefresh bool) (*credentials.OAuthTokens, error) {
	acc, err := a.accountStore.Get(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	if acc == nil || acc.AuthType != account.AuthOAuth2 || acc.IMAPHost != gmailIMAPHost {
		return nil, gmail.ErrNotCapable
	}

	stored, err := a.credStore.GetOAuthTokens(accountID)
	if err != nil || stored.Provider != "google" || !scopesOK(stored.Scopes) {
		return nil, gmail.ErrNotCapable
	}

	if forceRefresh {
		return a.composeOps.forceRefreshOAuthToken(a.ctx, accountID)
	}
	return a.composeOps.getValidOAuthToken(a.ctx, accountID)
}
