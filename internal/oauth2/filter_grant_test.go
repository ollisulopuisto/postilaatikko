package oauth2

import (
	"net/url"
	"slices"
	"testing"
)

func TestGoogleFilterIncrementalConsentIsIsolated(t *testing.T) {
	original := GoogleProvider()
	provider, err := GoogleFilterManagementProvider("me@example.test", original.Scopes)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(buildAuthURL(provider, "state", "challenge", "http://127.0.0.1/callback"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("include_granted_scopes") != "true" || u.Query().Get("login_hint") != "me@example.test" {
		t.Fatal("incremental account-bound grant missing")
	}
	for _, scope := range original.Scopes {
		if !slices.Contains(provider.Scopes, scope) {
			t.Fatal("existing scope removed")
		}
	}
	if slices.Contains(original.Scopes, "https://www.googleapis.com/auth/gmail.settings.basic") {
		t.Fatal("default mail login gained restricted scope")
	}
	for _, p := range []ProviderConfig{GoogleProvider(), GoogleContactsOnlyProvider(), GoogleCalendarProvider()} {
		parsed, _ := url.Parse(buildAuthURL(p, "state", "challenge", "http://127.0.0.1/callback"))
		if parsed.Query().Get("include_granted_scopes") != "false" {
			t.Fatal("unrelated grant changed")
		}
	}
	if _, err := GoogleFilterManagementProvider("me@example.test", nil); err == nil {
		t.Fatal("unknown grant guessed")
	}
}
func TestValidateGoogleFilterGrant(t *testing.T) {
	basic := "https://www.googleapis.com/auth/gmail.settings.basic"
	for _, tc := range []struct {
		name, email, scope string
		wantErr            bool
	}{
		{"complete", "ME@example.test", "https://mail.google.com/ openid " + basic, false},
		{"wrong account", "other@example.test", "https://mail.google.com/ openid " + basic, true},
		{"missing settings", "me@example.test", "https://mail.google.com/ openid", true},
		{"missing old scope", "me@example.test", "openid " + basic, true},
		{"unreported scopes", "me@example.test", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := &TokenResponse{AccessToken: "test-token", Scope: tc.scope}
			err := ValidateGoogleFilterGrant("me@example.test", tc.email, []string{"https://mail.google.com/", "openid"}, response)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestFilterConsentUsesResolvedMailCredentials(t *testing.T) {
	old := UserOverrideLookup
	defer func() { UserOverrideLookup = old }()
	UserOverrideLookup = func(id string) (ClientCredentials, bool) {
		if id != "google-mail" {
			t.Fatal("wrong client slot", id)
		}
		return ClientCredentials{ClientID: "custom-mail-client", ClientSecret: "custom-mail-secret"}, true
	}
	provider, err := GoogleFilterManagementProvider("me@example.test", []string{"https://mail.google.com/"})
	if err != nil || provider.ClientID != "custom-mail-client" || provider.ClientSecret != "custom-mail-secret" {
		t.Fatal("selected Mail project not used", err)
	}
}
