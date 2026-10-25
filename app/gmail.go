package app

import (
	"context"
	"fmt"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
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

// newGmailModifyClient returns a Gmail API client for label changes. Its token
// source requires a scope that allows modifying labels.
func (a *App) newGmailModifyClient(accountID string) *gmail.Client {
	return gmail.NewClient(func(_ context.Context, forceRefresh bool) (string, error) {
		tokens, err := a.gmailTokensFor(accountID, gmail.ScopesCoverModify, forceRefresh)
		if err != nil {
			return "", err
		}
		return tokens.AccessToken, nil
	})
}

// gmailSpamMove decides whether a move can be done as a Gmail label change.
// That is the case only for Inbox -> Spam (spam=true) and Spam -> Inbox
// (spam=false) on an account whose granted scopes allow label changes, when
// every message has a Message-ID to look up. The IMAP COPY+DELETE path is
// equivalent for these two moves, so any other move, and any doubt, returns
// ok=false and keeps the existing behavior.
func (a *App) gmailSpamMove(accountID string, msgs []*message.Message, source, dest *folder.Folder) (rfcIDs []string, spam, ok bool) {
	switch {
	case source.Type == folder.TypeInbox && dest.Type == folder.TypeSpam:
		spam = true
	case source.Type == folder.TypeSpam && dest.Type == folder.TypeInbox:
		spam = false
	default:
		return nil, false, false
	}
	if _, err := a.gmailTokensFor(accountID, gmail.ScopesCoverModify, false); err != nil {
		return nil, false, false
	}
	for _, m := range msgs {
		if m.MessageID == "" {
			return nil, false, false
		}
		rfcIDs = append(rfcIDs, m.MessageID)
	}
	return rfcIDs, spam, len(rfcIDs) > 0
}
