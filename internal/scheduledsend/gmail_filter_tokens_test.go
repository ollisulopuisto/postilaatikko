package scheduledsend

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
	keyring "github.com/zalando/go-keyring"
)

type nativeFilterRefreshTransport func(*http.Request) (*http.Response, error)

func (f nativeFilterRefreshTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHelperGmailFilterTokenSourceRequiresAndPreservesGrant(t *testing.T) {
	for _, responseScope := range []string{"", "https://mail.google.com/", "https://mail.google.com/ " + gmail.ScopeSettingsBasic} {
		name := "retained"
		if responseScope == "https://mail.google.com/" {
			name = "reduced"
		}
		if strings.Contains(responseScope, gmail.ScopeSettingsBasic) {
			name = "reported-full"
		}
		t.Run(name, func(t *testing.T) {
			keyring.MockInit()
			dir := t.TempDir()
			db, err := database.Open(filepath.Join(dir, "mail.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = db.Migrate(); err != nil {
				t.Fatal(err)
			}
			accounts := account.NewStore(db)
			acc, err := accounts.Create(&account.AccountConfig{Name: "Helper", DisplayName: "Helper", Email: "helper@example.com", IMAPHost: "imap.gmail.com", SMTPHost: "smtp.gmail.com", Username: "helper@example.com", AuthType: account.AuthOAuth2})
			if err != nil {
				t.Fatal(err)
			}
			creds, err := credentials.NewStore(db.DB, dir)
			if err != nil {
				t.Fatal(err)
			}
			tokens := &credentials.OAuthTokens{Provider: "google", AccessToken: "old-access", RefreshToken: "keep-refresh", ExpiresAt: time.Now().Add(-time.Hour), Scopes: []string{"https://mail.google.com/"}}
			if err = creds.SetOAuthTokens(acc.ID, tokens); err != nil {
				t.Fatal(err)
			}
			calls := 0
			old := http.DefaultTransport
			http.DefaultTransport = nativeFilterRefreshTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if err := req.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if req.Form.Get("refresh_token") != "keep-refresh" {
					t.Error("wrong refresh owner")
				}
				body := `{"access_token":"new-access","expires_in":3600}`
				if responseScope != "" {
					body = `{"access_token":"new-access","expires_in":3600,"scope":"` + responseScope + `"}`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = old })
			transport := &SMTPTransport{accounts: accounts, creds: creds, oauth: oauth2.NewManager()}
			if _, err = transport.GmailFilterTokenSource(context.Background(), acc.ID, false); err == nil || calls != 0 {
				t.Fatal("ordinary mail grant triggered settings refresh")
			}
			tokens.Scopes = append(tokens.Scopes, gmail.ScopeSettingsBasic)
			if err = creds.SetOAuthTokens(acc.ID, tokens); err != nil {
				t.Fatal(err)
			}
			access, err := transport.GmailFilterTokenSource(context.Background(), acc.ID, false)
			reduced := responseScope == "https://mail.google.com/"
			if reduced {
				if err == nil {
					t.Fatal("reduced settings scope accepted")
				}
			} else if err != nil || access != "new-access" {
				t.Fatal(access, err)
			}
			stored, err := creds.GetOAuthTokens(acc.ID)
			if err != nil || stored.RefreshToken != "keep-refresh" || stored.AccessToken != "new-access" || calls != 1 {
				t.Fatal("refresh not preserved", calls, err)
			}
			if reduced && gmail.ScopesCoverFilters(stored.Scopes) {
				t.Fatal("reported scope reduction lost")
			}
		})
	}
}

func TestHelperRefreshCannotOverwriteConcurrentConsent(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	accounts := account.NewStore(db)
	acc, err := accounts.Create(&account.AccountConfig{Name: "Helper", DisplayName: "Helper", Email: "helper@example.com", IMAPHost: "imap.gmail.com", SMTPHost: "smtp.gmail.com", Username: "helper@example.com", AuthType: account.AuthOAuth2})
	if err != nil {
		t.Fatal(err)
	}
	creds, err := credentials.NewStore(db.DB, dir)
	if err != nil {
		t.Fatal(err)
	}
	original := &credentials.OAuthTokens{Provider: "google", AccessToken: "expired", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Hour), Scopes: []string{"https://mail.google.com/"}}
	if err = creds.SetOAuthTokens(acc.ID, original); err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	http.DefaultTransport = nativeFilterRefreshTransport(func(req *http.Request) (*http.Response, error) {
		fresh := *original
		fresh.AccessToken = "consented"
		fresh.Scopes = append([]string(nil), original.Scopes...)
		fresh.Scopes = append(fresh.Scopes, gmail.ScopeSettingsBasic)
		fresh.ExpiresAt = time.Now().Add(time.Hour)
		if e := creds.SetOAuthTokens(acc.ID, &fresh); e != nil {
			t.Fatal(e)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"stale-refresh","expires_in":3600}`))}, nil
	})
	transport := &SMTPTransport{accounts: accounts, creds: creds, oauth: oauth2.NewManager()}
	if _, err = transport.tokens(context.Background(), acc.ID); err == nil {
		t.Fatal("stale refresh accepted")
	}
	stored, err := creds.GetOAuthTokens(acc.ID)
	if err != nil || stored.AccessToken != "consented" || !gmail.ScopesCoverFilters(stored.Scopes) {
		t.Fatal("concurrent consent overwritten", err)
	}
}
