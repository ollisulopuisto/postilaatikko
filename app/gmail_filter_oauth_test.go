package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
)

func filterConsentFixture(t *testing.T) *App {
	a, _ := contactOAuthFixture(t)
	oldTransport := http.DefaultTransport
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if req.Form.Get("client_id") != "test-google-mail" {
			t.Fatal("wrong refresh client")
		}
		body, _ := json.Marshal(map[string]any{"access_token": "filter-access", "expires_in": 3600, "scope": strings.Join(append(oauth2.GoogleProvider().Scopes, gmail.ScopeSettingsBasic), " ")})
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	old := oauth2.UserOverrideLookup
	oauth2.UserOverrideLookup = func(id string) (oauth2.ClientCredentials, bool) {
		return oauth2.ClientCredentials{ClientID: "test-" + id, ClientSecret: "test"}, true
	}
	t.Cleanup(func() { oauth2.UserOverrideLookup = old })
	return a
}
func TestGmailFilterConsentAccountAndGrantGuards(t *testing.T) {
	for _, tc := range []string{"wrong identity", "partial settings", "missing scope report", "grant changed", "project changed", "remote switched", "success without new refresh", "success rotated refresh"} {
		t.Run(tc, func(t *testing.T) {
			a := filterConsentFixture(t)
			email, previous, cfg, err := a.prepareGmailFilterConsent("mail")
			if err != nil {
				t.Fatal(err)
			}
			defer previous.Clear()
			response := &oauth2.TokenResponse{AccessToken: "filter-access", Scope: strings.Join(append(append([]string(nil), previous.Scopes...), gmail.ScopeSettingsBasic), " "), ExpiresIn: 3600}
			actual := email
			switch tc {
			case "wrong identity":
				actual = "other@example.com"
			case "partial settings":
				response.Scope = gmail.ScopeSettingsBasic
			case "missing scope report":
				response.Scope = ""
			case "grant changed":
				replacement := *previous
				replacement.RefreshToken = "newer-refresh"
				if err := a.credStore.SetOAuthTokens("mail", &replacement); err != nil {
					t.Fatal(err)
				}
			case "project changed":
				cfg.ClientID = "old-client"
			case "remote switched":
				a.mailboxService = &mailboxServiceConnector{endpoint: "https://studio.example.test:8029"}
			case "success rotated refresh":
				response.RefreshToken = "rotated"
			}
			err = a.persistGmailFilterConsent("mail", email, previous, cfg.ClientID, response, actual)
			success := strings.HasPrefix(tc, "success")
			if (err == nil) != success {
				t.Fatalf("err=%v", err)
			}
			stored, err := a.credStore.GetOAuthTokens("mail")
			if err != nil {
				t.Fatal(err)
			}
			defer stored.Clear()
			if !success {
				if stored.AccessToken != "mail-access" {
					t.Fatal("failed consent changed access")
				}
				return
			}
			if stored.AccessToken != "filter-access" || !gmail.ScopesCoverFilters(stored.Scopes) {
				t.Fatal("grant missing")
			}
			want := "mail-refresh"
			if tc == "success rotated refresh" {
				want = "rotated"
			}
			if stored.RefreshToken != want {
				t.Fatal("refresh token lost")
			}
			if cfg.ClientID != "test-google-mail" {
				t.Fatal("not resolved mail OAuth client")
			}
		})
	}
}
func TestGmailFilterConsentRejectsUnownedContext(t *testing.T) {
	a := filterConsentFixture(t)
	if _, _, _, err := a.prepareGmailFilterConsent("missing"); err == nil {
		t.Fatal("missing account accepted")
	}
	a.mailboxService = &mailboxServiceConnector{endpoint: "https://studio.example.test:8029"}
	if _, _, _, err := a.prepareGmailFilterConsent("mail"); err == nil {
		t.Fatal("remote account used local credentials")
	}
}
func TestGmailFilterConsentDoesNotConsumeMailCallback(t *testing.T) {
	a := filterConsentFixture(t)
	a.pendingOAuthTokens = &oauth2.TokenResponse{AccessToken: "pending-mail"}
	email, old, cfg, err := a.prepareGmailFilterConsent("mail")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Clear()
	response := &oauth2.TokenResponse{AccessToken: "filter-access", Scope: strings.Join(append(append([]string(nil), old.Scopes...), gmail.ScopeSettingsBasic), " "), ExpiresIn: 3600}
	if err := a.persistGmailFilterConsent("mail", email, old, cfg.ClientID, response, email); err != nil {
		t.Fatal(err)
	}
	if a.pendingOAuthTokens.AccessToken != "pending-mail" {
		t.Fatal("Mail callback consumed")
	}
}

func TestFilterConsentRejectsUnverifiedDurableRefreshGrant(t *testing.T) {
	for i, scope := range []string{"", "https://mail.google.com/"} {
		t.Run([]string{"missing", "reduced"}[i], func(t *testing.T) {
			a := filterConsentFixture(t)
			email, previous, cfg, err := a.prepareGmailFilterConsent("mail")
			if err != nil {
				t.Fatal(err)
			}
			defer previous.Clear()
			http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]any{"access_token": "reduced", "expires_in": 3600, "scope": scope})
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})
			response := &oauth2.TokenResponse{AccessToken: "initial", ExpiresIn: 3600, Scope: strings.Join(append(append([]string(nil), previous.Scopes...), gmail.ScopeSettingsBasic), " ")}
			if err = a.persistGmailFilterConsent("mail", email, previous, cfg.ClientID, response, email); err == nil {
				t.Fatal("unverified durable grant accepted")
			}
			stored, err := a.credStore.GetOAuthTokens("mail")
			if err != nil || stored.AccessToken != "mail-access" {
				t.Fatal("failed consent overwrote grant", err)
			}
		})
	}
}

func TestMailRefreshCannotUndoConcurrentFilterConsent(t *testing.T) {
	a := filterConsentFixture(t)
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
		next, err := a.credStore.GetOAuthTokens("mail")
		if err != nil {
			t.Fatal(err)
		}
		next.AccessToken = "new-consent"
		next.Scopes = append(next.Scopes, gmail.ScopeSettingsBasic)
		if err = a.credStore.SetOAuthTokens("mail", next); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"stale-mail","expires_in":3600}`))}, nil
	})
	ops := &composeOps{credStore: a.credStore, oauth2Manager: oauth2.NewManager()}
	_, err := ops.forceRefreshOAuthToken(context.Background(), "mail")
	if !errors.Is(err, credentials.ErrOAuthTokensChanged) {
		t.Fatal("stale refresh was not rejected", err)
	}
	stored, err := a.credStore.GetOAuthTokens("mail")
	if err != nil || stored.AccessToken != "new-consent" || !gmail.ScopesCoverFilters(stored.Scopes) {
		t.Fatal("consent lost", err)
	}
}

func TestFilterConsentRejectsContextSwitchDuringRefreshVerification(t *testing.T) {
	for _, mode := range []string{"workspace", "project", "account"} {
		t.Run(mode, func(t *testing.T) {
			a := filterConsentFixture(t)
			email, previous, cfg, err := a.prepareGmailFilterConsent("mail")
			if err != nil {
				t.Fatal(err)
			}
			defer previous.Clear()
			old := http.DefaultTransport
			http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch mode {
				case "workspace":
					a.mailboxService = &mailboxServiceConnector{endpoint: "https://example.test"}
				case "project":
					oauth2.UserOverrideLookup = func(string) (oauth2.ClientCredentials, bool) {
						return oauth2.ClientCredentials{ClientID: "changed"}, true
					}
				case "account":
					if _, err := a.accountStore.Get("mail"); err != nil {
						t.Fatal(err)
					}
					if err := a.credStore.DeleteOAuthTokens("mail"); err != nil {
						t.Fatal(err)
					}
				}
				return old.RoundTrip(req)
			})
			response := &oauth2.TokenResponse{AccessToken: "initial", ExpiresIn: 3600, Scope: strings.Join(append(append([]string(nil), previous.Scopes...), gmail.ScopeSettingsBasic), " ")}
			if err = a.persistGmailFilterConsent("mail", email, previous, cfg.ClientID, response, email); err == nil {
				t.Fatal("context switch accepted")
			}
			if mode != "account" {
				stored, err := a.credStore.GetOAuthTokens("mail")
				if err != nil || stored.AccessToken != "mail-access" {
					t.Fatal("switched consent changed grant", err)
				}
			}
		})
	}
}

func TestDesktopRefreshWaiterReusesWinningGrantUnlessForced(t *testing.T) {
	a := filterConsentFixture(t)
	current, err := a.credStore.GetOAuthTokens("mail")
	if err != nil {
		t.Fatal(err)
	}
	current.ExpiresAt = time.Now().Add(time.Hour)
	if err = a.credStore.SetOAuthTokens("mail", current); err != nil {
		t.Fatal(err)
	}
	calls := 0
	old := http.DefaultTransport
	http.DefaultTransport = contactsRoundTrip(func(req *http.Request) (*http.Response, error) { calls++; return old.RoundTrip(req) })
	ops := &composeOps{credStore: a.credStore, oauth2Manager: oauth2.NewManager()}
	// Models a waiter whose initial expiry check preceded another consumer's refresh.
	got, err := ops.refreshStoredOAuthToken(context.Background(), "mail", false)
	if err != nil || got.AccessToken != "mail-access" || calls != 0 {
		t.Fatal("waiter refreshed usable grant", calls, err)
	}
	if _, err = ops.forceRefreshOAuthToken(context.Background(), "mail"); err != nil || calls != 1 {
		t.Fatal("401 forced refresh was skipped", calls, err)
	}
}
