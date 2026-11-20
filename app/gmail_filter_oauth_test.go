package app

import (
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/oauth2"
)

func filterConsentFixture(t *testing.T) *App {
	a, _ := contactOAuthFixture(t)
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
	for _, endpoint := range []string{"", "http://127.0.0.1:8029", "https://studio.example.test:8029"} {
		a.mailboxService = &mailboxServiceConnector{endpoint: endpoint}
		if _, _, _, err := a.prepareGmailFilterConsent("mail"); err == nil {
			t.Fatal("helper-owned account used desktop credentials", endpoint)
		}
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
