package oauth2

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type externalLoopbackTransport func(*http.Request) (*http.Response, error)

func (f externalLoopbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestExternalLoopbackRejectsNonDesktopCallbacks(t *testing.T) {
	for _, redirect := range []string{"https://example.com/callback", "http://localhost:1234/callback", "http://127.0.0.1/callback", "http://127.0.0.1:1234/other", "http://127.0.0.1:1234/callback?x=1", "http://u@127.0.0.1:1234/callback", "http://127.0.0.1:70000/callback"} {
		if _, _, err := NewExternalLoopbackFlow(GoogleProvider(), redirect); err == nil {
			t.Fatal("unsafe callback accepted", redirect)
		}
	}
}
func TestExternalLoopbackRetainsPKCEAndConsumesOnce(t *testing.T) {
	cfg := GoogleProvider()
	cfg.ClientID = "helper-project"
	cfg.ClientSecret = "host-config"
	cfg.IncrementalConsent = true
	flow, link, err := NewExternalLoopbackFlow(cfg, "http://127.0.0.1:12345/callback")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != "helper-project" || strings.Contains(link, "host-config") || q.Get("redirect_uri") != "http://127.0.0.1:12345/callback" {
		t.Fatal("authorization URL exposed config or lost PKCE")
	}
	calls := 0
	old := http.DefaultTransport
	http.DefaultTransport = externalLoopbackTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(req.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != q.Get("code_challenge") || req.Form.Get("code") != "callback-code" || req.Form.Get("client_id") != "helper-project" {
			t.Error("PKCE exchange not bound to helper session")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"test-access","expires_in":3600}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	if _, err = flow.Exchange(context.Background(), "callback-code", "wrong"); err == nil || calls != 0 {
		t.Fatal("wrong state exchanged")
	}
	tokens, err := flow.Exchange(context.Background(), "callback-code", q.Get("state"))
	if err != nil || tokens.AccessToken != "test-access" || calls != 1 {
		t.Fatal(calls, err)
	}
	if _, err = flow.Exchange(context.Background(), "callback-code", q.Get("state")); err == nil || calls != 1 {
		t.Fatal("callback replayed")
	}
	for _, mode := range []string{"expired", "cancelled"} {
		f, link, err := NewExternalLoopbackFlow(cfg, "http://127.0.0.1:12345/callback")
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(link)
		if mode == "expired" {
			f.expires = time.Now().Add(-time.Second)
		} else {
			f.Cancel()
		}
		if _, err = f.Exchange(context.Background(), "callback-code", u.Query().Get("state")); err == nil || calls != 1 {
			t.Fatal("dead flow exchanged", mode)
		}
	}
}
