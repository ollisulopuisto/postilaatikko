package oauth2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSetupPKCECallbackStoresNoSecretInError(t *testing.T) {
	for _, mode := range []string{"success", "state", "denied", "no-refresh", "no-identity"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					_ = r.ParseForm()
					if r.Form.Get("code_verifier") == "" || r.Form.Get("grant_type") != "authorization_code" {
						t.Error("missing PKCE")
					}
					w.Header().Set("Content-Type", "application/json")
					if mode == "no-refresh" {
						_, _ = w.Write([]byte(`{"access_token":"access-fixture","expires_in":3600}`))
						return
					}
					_, _ = w.Write([]byte(`{"access_token":"access-fixture","refresh_token":"refresh-fixture","expires_in":3600}`))
					return
				}
				if r.Header.Get("Authorization") != "Bearer access-fixture" {
					t.Error("userinfo not authenticated")
				}
				if mode == "no-identity" {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				_, _ = w.Write([]byte(`{"email":"me@example.org"}`))
			}))
			defer server.Close()
			manager := NewManager()
			defer manager.CancelAuthFlow()
			link, err := manager.StartAuthFlowWithProvider(context.Background(), &ProviderConfig{Name: "google", AuthURL: "https://example.org/auth", TokenURL: server.URL + "/token", UserinfoEndpoint: server.URL + "/userinfo", ClientID: "desktop-fixture", LoopbackHost: "127.0.0.1"})
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(link)
			if u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" {
				t.Fatal("missing challenge")
			}
			state := u.Query().Get("state")
			if mode == "state" {
				state = "wrong"
			}
			callback := u.Query().Get("redirect_uri") + "?code=code-fixture&state=" + state
			if mode == "denied" {
				callback += "&error=access_denied&error_description=secret-fixture"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan struct{})
			var tokens *TokenResponse
			var email string
			var got error
			go func() { tokens, email, got = manager.WaitForSetupCallback(ctx); close(done) }()
			resp, err := http.Get(callback)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			<-done
			if mode == "success" {
				if got != nil || email != "me@example.org" || tokens.RefreshToken != "refresh-fixture" {
					t.Fatal("setup failed")
				}
				tokens.Clear()
				return
			}
			if got == nil || tokens != nil {
				t.Fatal("incomplete/invalid consent accepted")
			}
			for _, secret := range []string{"access-fixture", "refresh-fixture", "code-fixture", "secret-fixture"} {
				if strings.Contains(got.Error(), secret) {
					t.Fatal("secret in error")
				}
			}
		})
	}
}
