package alwayson

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/credentials"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/oauth2"
)

type helperGmailConsentSession struct {
	id, clientID, accountID, email, binding, refreshDigest string
	scopes                                                 []string
	expires                                                time.Time
	flow                                                   *oauth2.ExternalLoopbackFlow
	timer                                                  *time.Timer
}
type helperGmailConsentSessions struct {
	mu      sync.Mutex
	pending map[string]*helperGmailConsentSession
	closed  bool
}

func gmailRefreshDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *helperGmailConsentSessions) remove(id string) *helperGmailConsentSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.pending[id]
	delete(s.pending, id)
	return session
}
func (s *helperGmailConsentSessions) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for id, session := range s.pending {
		delete(s.pending, id)
		session.timer.Stop()
		session.flow.Cancel()
	}
}

// GmailConsent holds the selected account's PKCE exchange on its helper. The
// desktop sees an authorization URL and forwards only a one-use callback code.
// No existing helper refresh token, project secret or verifier crosses the API.
func (r *SyncRuntime) GmailConsent(ctx context.Context, clientID string, req v1.GmailConsentRequest) (v1.GmailConsentReply, error) {
	result := v1.GmailConsentReply{AccountID: req.AccountID}
	if clientID == "" || req.AccountID == "" {
		return result, fmt.Errorf("authenticated client and selected account required")
	}
	p, granted, err := r.gmailPublicationOwner(req.AccountID)
	if err != nil {
		return result, err
	}
	result.Email = p.email
	switch req.Operation {
	case "start":
		if granted {
			result.Authorized = true
			return result, nil
		}
		previous, err := r.contacts.creds.GetOAuthTokens(req.AccountID)
		if err != nil {
			return result, err
		}
		defer previous.Clear()
		cfg, err := oauth2.GoogleFilterManagementProvider(p.email, previous.Scopes)
		if err != nil {
			return result, err
		}
		resolved, err := oauth2.GetProvider("google")
		if err != nil {
			return result, err
		}
		cfg.ClientID, cfg.ClientSecret = resolved.ClientID, resolved.ClientSecret
		if helperGmailBinding(p.email, cfg.ClientID) != p.binding {
			return result, fmt.Errorf("helper OAuth project changed; start again")
		}
		flow, link, err := oauth2.NewExternalLoopbackFlow(cfg, req.RedirectURI)
		if err != nil {
			return result, err
		}
		session := &helperGmailConsentSession{id: uuid.NewString(), clientID: clientID, accountID: req.AccountID, email: p.email, binding: p.binding, refreshDigest: gmailRefreshDigest(previous.RefreshToken), scopes: slices.Clone(previous.Scopes), expires: time.Now().Add(5 * time.Minute), flow: flow}
		r.gmailConsent.mu.Lock()
		if r.gmailConsent.closed {
			r.gmailConsent.mu.Unlock()
			flow.Cancel()
			return result, fmt.Errorf("helper consent owner closed")
		}
		if r.gmailConsent.pending == nil {
			r.gmailConsent.pending = map[string]*helperGmailConsentSession{}
		}
		// Bound memory use and replace only this client's same-account flow.
		for id, old := range r.gmailConsent.pending {
			if time.Now().After(old.expires) || (old.clientID == clientID && old.accountID == req.AccountID) {
				delete(r.gmailConsent.pending, id)
				old.timer.Stop()
				old.flow.Cancel()
			}
		}
		if len(r.gmailConsent.pending) >= 32 {
			r.gmailConsent.mu.Unlock()
			flow.Cancel()
			return result, fmt.Errorf("too many helper consent sessions")
		}
		r.gmailConsent.pending[session.id] = session
		session.timer = time.AfterFunc(5*time.Minute, func() {
			if expired := r.gmailConsent.remove(session.id); expired != nil {
				expired.flow.Cancel()
			}
		})
		r.gmailConsent.mu.Unlock()
		result.SessionID = session.id
		result.AuthURL = link
		result.ExpiresAt = session.expires.UTC().Format(time.RFC3339)
		return result, nil
	case "cancel", "complete":
		r.gmailConsent.mu.Lock()
		session := r.gmailConsent.pending[req.SessionID]
		if session == nil || session.accountID != req.AccountID || session.clientID != clientID {
			r.gmailConsent.mu.Unlock()
			return result, fmt.Errorf("consent session not owned by this client/account")
		}
		delete(r.gmailConsent.pending, session.id)
		session.timer.Stop()
		r.gmailConsent.mu.Unlock()
		defer session.flow.Cancel()
		if req.Operation == "cancel" {
			return result, nil
		}
		if time.Now().After(session.expires) {
			return result, fmt.Errorf("consent session expired")
		}
		if err = r.validateGmailConsentSnapshot(session); err != nil {
			return result, err
		}
		tokens, err := session.flow.Exchange(ctx, req.Code, req.State)
		if err != nil {
			return result, err
		}
		defer tokens.Clear()
		// Use a live Gmail profile for identity. Never trust caller-supplied email
		// or unverified ID-token text as proof of the callback's account.
		client := gmail.NewClient(func(_ context.Context, force bool) (string, error) {
			if force {
				return "", fmt.Errorf("consent token expired")
			}
			return tokens.AccessToken, nil
		})
		email, err := client.ProfileEmail(ctx)
		if err != nil {
			return result, err
		}
		if err = oauth2.ValidateGoogleFilterGrant(session.email, email, session.scopes, tokens); err != nil {
			return result, err
		}
		r.accountMutationMu.Lock()
		defer r.accountMutationMu.Unlock()
		if err = r.validateGmailConsentSnapshot(session); err != nil {
			return result, err
		}
		current, err := r.contacts.creds.GetOAuthTokens(session.accountID)
		if err != nil {
			return result, err
		}
		defer current.Clear()
		refresh := tokens.RefreshToken
		if refresh == "" {
			refresh = current.RefreshToken
		}
		stored := &credentials.OAuthTokens{Provider: "google", AccessToken: tokens.AccessToken, RefreshToken: refresh, ExpiresAt: time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second), Scopes: strings.Fields(tokens.Scope)}
		defer stored.Clear()
		if err = r.contacts.creds.SetOAuthTokens(session.accountID, stored); err != nil {
			return result, fmt.Errorf("persist helper settings grant: %w", err)
		}
		readback, err := r.contacts.creds.GetOAuthTokens(session.accountID)
		if err != nil {
			return result, fmt.Errorf("helper grant saved but readback unavailable: %w", err)
		}
		defer readback.Clear()
		if readback.Provider != "google" || readback.AccessToken != stored.AccessToken || readback.RefreshToken != stored.RefreshToken || !gmail.ScopesCoverFilters(readback.Scopes) {
			return result, fmt.Errorf("helper grant readback mismatch")
		}
		result.Authorized = true
		return result, nil
	default:
		return result, fmt.Errorf("unknown consent operation")
	}
}
func (r *SyncRuntime) validateGmailConsentSnapshot(session *helperGmailConsentSession) error {
	p, _, err := r.gmailPublicationOwner(session.accountID)
	if err != nil {
		return err
	}
	if p.binding != session.binding {
		return fmt.Errorf("helper account or OAuth project changed during consent")
	}
	tokens, err := r.contacts.creds.GetOAuthTokens(session.accountID)
	if err != nil {
		return err
	}
	defer tokens.Clear()
	previous, current := slices.Clone(session.scopes), slices.Clone(tokens.Scopes)
	slices.Sort(previous)
	slices.Sort(current)
	if gmailRefreshDigest(tokens.RefreshToken) != session.refreshDigest || !slices.Equal(previous, current) {
		return fmt.Errorf("helper grant changed during consent; start again")
	}
	return nil
}
