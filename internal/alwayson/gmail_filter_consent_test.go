package alwayson

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
)

func helperConsentFixture(t *testing.T) *SyncRuntime {
	t.Helper()
	r := helperNativeFixture(t)
	tokens, err := r.contacts.creds.GetOAuthTokens("helper-google")
	if err != nil {
		t.Fatal(err)
	}
	tokens.Scopes = []string{"https://mail.google.com/", "openid"}
	if err = r.contacts.creds.SetOAuthTokens("helper-google", tokens); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestHelperConsentRetainsPKCEAndCredentialsOnOwner(t *testing.T) {
	r := helperConsentFixture(t)
	start := v1.GmailConsentRequest{Operation: "start", AccountID: "helper-google", RedirectURI: "http://127.0.0.1:12345/callback"}
	reply, err := r.GmailConsent(context.Background(), "desktop-client", start)
	if err != nil || reply.Authorized || reply.SessionID == "" {
		t.Fatal(reply, err)
	}
	parsed, err := url.Parse(reply.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("client_id") != "helper-project" || query.Get("login_hint") != "helper@example.com" || query.Get("include_granted_scopes") != "true" || query.Get("code_challenge") == "" {
		t.Fatal("wrong helper authorization context")
	}
	encoded, _ := json.Marshal(reply)
	if strings.Contains(string(encoded), "helper-access") || strings.Contains(string(encoded), "helper-refresh") || strings.Contains(string(encoded), "codeVerifier") {
		t.Fatal("helper secrets exposed")
	}
	exchanges := 0
	old := http.DefaultTransport
	http.DefaultTransport = helperNativeRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"emailAddress":"helper@example.com"}`
		if req.Method == http.MethodPost {
			exchanges++
			if err = req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if req.Form.Get("code") != "one-code" || req.Form.Get("code_verifier") == "" || req.Form.Get("redirect_uri") != start.RedirectURI || req.Form.Get("client_id") != "helper-project" {
				t.Error("code not exchanged on owning helper")
			}
			body = `{"access_token":"new-helper-access","expires_in":3600,"scope":"https://mail.google.com/ openid ` + gmail.ScopeSettingsBasic + `"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	complete := v1.GmailConsentRequest{Operation: "complete", AccountID: "helper-google", SessionID: reply.SessionID, Code: "one-code", State: query.Get("state")}
	if _, err = r.GmailConsent(context.Background(), "other-client", complete); err == nil || exchanges != 0 {
		t.Fatal("other client consumed flow")
	}
	done, err := r.GmailConsent(context.Background(), "desktop-client", complete)
	if err != nil || !done.Authorized || done.AuthURL != "" || done.SessionID != "" || exchanges != 1 {
		t.Fatal(done, exchanges, err)
	}
	stored, err := r.contacts.creds.GetOAuthTokens("helper-google")
	if err != nil || stored.AccessToken != "new-helper-access" || stored.RefreshToken != "helper-refresh" || !gmail.ScopesCoverFilters(stored.Scopes) {
		t.Fatal("helper grant not preserved", err)
	}
	if _, err = r.GmailConsent(context.Background(), "desktop-client", complete); err == nil || exchanges != 1 {
		t.Fatal("consent callback replayed")
	}
	done, err = r.GmailConsent(context.Background(), "desktop-client", start)
	if err != nil || !done.Authorized || done.AuthURL != "" {
		t.Fatal("existing grant asked again", done, err)
	}
}
func TestHelperConsentRejectsChangedExpiredCancelledAndInvalidGrants(t *testing.T) {
	for _, mode := range []string{"wrong state", "wrong identity", "missing scope", "grant changed", "project changed", "expired", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			r := helperConsentFixture(t)
			start := v1.GmailConsentRequest{Operation: "start", AccountID: "helper-google", RedirectURI: "http://127.0.0.1:12345/callback"}
			reply, err := r.GmailConsent(context.Background(), "client", start)
			if err != nil {
				t.Fatal(err)
			}
			parsed, _ := url.Parse(reply.AuthURL)
			state := parsed.Query().Get("state")
			exchanges := 0
			old := http.DefaultTransport
			http.DefaultTransport = helperNativeRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := `{"emailAddress":"helper@example.com"}`
				if mode == "wrong identity" {
					body = `{"emailAddress":"other@example.com"}`
				}
				if req.Method == http.MethodPost {
					exchanges++
					scope := "https://mail.google.com/ openid " + gmail.ScopeSettingsBasic
					if mode == "missing scope" {
						scope = "https://mail.google.com/ openid"
					}
					body = `{"access_token":"should-not-save","expires_in":3600,"scope":"` + scope + `"}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = old })
			switch mode {
			case "wrong state":
				state = "wrong-state"
			case "grant changed":
				tokens, err := r.contacts.creds.GetOAuthTokens("helper-google")
				if err != nil {
					t.Fatal(err)
				}
				tokens.RefreshToken = "newer-refresh"
				if err = r.contacts.creds.SetOAuthTokens("helper-google", tokens); err != nil {
					t.Fatal(err)
				}
			case "project changed":
				oldLookup := oauth2.UserOverrideLookup
				oauth2.UserOverrideLookup = func(string) (oauth2.ClientCredentials, bool) {
					return oauth2.ClientCredentials{ClientID: "different-project"}, true
				}
				t.Cleanup(func() { oauth2.UserOverrideLookup = oldLookup })
			case "expired":
				r.gmailConsent.mu.Lock()
				r.gmailConsent.pending[reply.SessionID].expires = time.Now().Add(-time.Second)
				r.gmailConsent.mu.Unlock()
			case "cancelled":
				_, err = r.GmailConsent(context.Background(), "client", v1.GmailConsentRequest{Operation: "cancel", AccountID: "helper-google", SessionID: reply.SessionID})
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err = r.GmailConsent(context.Background(), "client", v1.GmailConsentRequest{Operation: "complete", AccountID: "helper-google", SessionID: reply.SessionID, Code: "one-code", State: state})
			if err == nil {
				t.Fatal("unsafe consent accepted", mode)
			}
			stored, err := r.contacts.creds.GetOAuthTokens("helper-google")
			if err != nil || stored.AccessToken != "helper-access" || gmail.ScopesCoverFilters(stored.Scopes) {
				t.Fatal("rejected callback overwrote helper grant", err)
			}
			if mode != "wrong identity" && mode != "missing scope" && exchanges != 0 {
				t.Fatal("dead/changed flow reached token endpoint", mode, exchanges)
			}
		})
	}
}
func TestHelperConsentReplacesOnlySameClientAccountFlow(t *testing.T) {
	r := helperConsentFixture(t)
	req := v1.GmailConsentRequest{Operation: "start", AccountID: "helper-google", RedirectURI: "http://127.0.0.1:12345/callback"}
	first, err := r.GmailConsent(context.Background(), "client-a", req)
	if err != nil {
		t.Fatal(err)
	}
	other, err := r.GmailConsent(context.Background(), "client-b", req)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := r.GmailConsent(context.Background(), "client-a", req)
	if err != nil {
		t.Fatal(err)
	}
	r.gmailConsent.mu.Lock()
	defer r.gmailConsent.mu.Unlock()
	if r.gmailConsent.pending[first.SessionID] != nil || r.gmailConsent.pending[other.SessionID] == nil || r.gmailConsent.pending[replacement.SessionID] == nil {
		t.Fatal("consent replacement crossed client scope")
	}
}

func TestHelperConsentRuntimeCloseDiscardsSessions(t *testing.T) {
	r := helperConsentFixture(t)
	req := v1.GmailConsentRequest{Operation: "start", AccountID: "helper-google", RedirectURI: "http://127.0.0.1:12345/callback"}
	if _, err := r.GmailConsent(context.Background(), "client", req); err != nil {
		t.Fatal(err)
	}
	r.gmailConsent.close()
	if len(r.gmailConsent.pending) != 0 {
		t.Fatal("pending consent retained after close")
	}
	if _, err := r.GmailConsent(context.Background(), "client", req); err == nil {
		t.Fatal("closed consent owner accepted new flow")
	}
}

func TestProductionMailboxHandlerMountsGmailConsent(t *testing.T) {
	r := openContactsAPITestRuntime(t)
	handler, err := r.MailboxReadAPIHandler()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/filters/gmail/consent", strings.NewReader(`{"operation":"start","accountId":"helper"}`)))
	if rec.Code == 404 {
		t.Fatal("helper consent route missing")
	}
}

func TestHelperConsentRejectsInvalidationDuringExchange(t *testing.T) {
	for _, mode := range []string{"expired", "closed", "cancelled", "new grant"} {
		t.Run(mode, func(t *testing.T) {
			r := helperConsentFixture(t)
			reply, err := r.GmailConsent(context.Background(), "client", v1.GmailConsentRequest{Operation: "start", AccountID: "helper-google", RedirectURI: "http://127.0.0.1:12345/callback"})
			if err != nil {
				t.Fatal(err)
			}
			parsed, _ := url.Parse(reply.AuthURL)
			session := r.gmailConsent.pending[reply.SessionID]
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			old := http.DefaultTransport
			http.DefaultTransport = helperNativeRoundTrip(func(req *http.Request) (*http.Response, error) {
				body := `{"emailAddress":"helper@example.com"}`
				if req.Method == http.MethodPost {
					body = `{"access_token":"must-not-save","expires_in":3600,"scope":"https://mail.google.com/ openid ` + gmail.ScopeSettingsBasic + `"}`
					switch mode {
					case "expired":
						session.expires = time.Now().Add(-time.Second)
					case "closed":
						r.gmailConsent.close()
					case "cancelled":
						cancel()
					case "new grant":
						tokens, e := r.contacts.creds.GetOAuthTokens("helper-google")
						if e != nil {
							t.Fatal(e)
						}
						tokens.RefreshToken = "replacement"
						if e = r.contacts.creds.SetOAuthTokens("helper-google", tokens); e != nil {
							t.Fatal(e)
						}
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = old })
			done, err := r.GmailConsent(ctx, "client", v1.GmailConsentRequest{Operation: "complete", AccountID: "helper-google", SessionID: reply.SessionID, Code: "code", State: parsed.Query().Get("state")})
			if err == nil || done.Authorized {
				t.Fatal("invalidated exchange authorized", mode)
			}
			stored, err := r.contacts.creds.GetOAuthTokens("helper-google")
			if err != nil || stored.AccessToken != "helper-access" || gmail.ScopesCoverFilters(stored.Scopes) {
				t.Fatal("invalidated exchange saved", err)
			}
		})
	}
}
