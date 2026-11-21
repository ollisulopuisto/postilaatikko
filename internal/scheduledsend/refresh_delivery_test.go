package scheduledsend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/oauth2"
	keyring "github.com/zalando/go-keyring"
)

func refreshDeliveryFixture(t *testing.T, gmail bool) (*SMTPTransport, *account.Account) {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	accounts := account.NewStore(db)
	host := "imap.gmail.com"
	if !gmail {
		host = "imap.example.test"
	}
	acc, err := accounts.Create(&account.AccountConfig{Name: "Helper", DisplayName: "Helper", Email: "helper@example.com", IMAPHost: host, SMTPHost: "smtp.example.test", SMTPSecurity: account.SecurityStartTLS, Username: "helper@example.com", AuthType: account.AuthOAuth2})
	if err != nil {
		t.Fatal(err)
	}
	creds, err := credentials.NewStore(db.DB, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = creds.SetOAuthTokens(acc.ID, &credentials.OAuthTokens{Provider: "google", AccessToken: "expired", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Hour), Scopes: []string{"https://mail.google.com/"}}); err != nil {
		t.Fatal(err)
	}
	return &SMTPTransport{accounts: accounts, creds: creds, oauth: oauth2.NewManager()}, acc
}
func refreshHTTPResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestConcurrentRefreshDoesNotFailScheduledDelivery(t *testing.T) {
	transport, acc := refreshDeliveryFixture(t, true)
	// Separate credential Stores match the helper's independently owned consumers.
	otherStore := *transport.creds
	syncTransport := &SMTPTransport{accounts: transport.accounts, creds: &otherStore, oauth: oauth2.NewManager()}
	started, release := make(chan struct{}), make(chan struct{})
	var refreshes, sends atomic.Int32
	old := http.DefaultTransport
	http.DefaultTransport = refreshDeliveryRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "gmail.googleapis.com" {
			sends.Add(1)
			return refreshHTTPResponse(`{"id":"sent-id","threadId":"thread"}`), nil
		}
		n := refreshes.Add(1)
		if n == 1 {
			close(started)
			<-release
		}
		return refreshHTTPResponse(`{"access_token":"refreshed","expires_in":3600}`), nil
	})
	defer func() { http.DefaultTransport = old }()
	first := make(chan error, 1)
	go func() { _, err := syncTransport.tokens(context.Background(), acc.ID); first <- err }()
	<-started
	delivered := make(chan error, 1)
	go func() {
		delivered <- transport.Deliver(context.Background(), Delivery{AccountID: acc.ID, From: acc.Email, To: []string{"recipient@example.test"}, RFC822: []byte("Subject: test\r\n\r\nbody")})
	}()
	// Give the second consumer a chance to contend while the first provider request is in flight.
	time.Sleep(100 * time.Millisecond)
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("sync refresh failed: %v", err)
	}
	if err := <-delivered; err != nil {
		t.Fatalf("scheduled delivery failed: %v", err)
	}
	if refreshes.Load() != 1 || sends.Load() != 1 {
		t.Fatalf("refreshes=%d sends=%d", refreshes.Load(), sends.Load())
	}
}

func TestDeliveryRetriesCredentialReplacementBeforeSubmission(t *testing.T) {
	for _, isGmail := range []bool{true, false} {
		name := "smtp"
		if isGmail {
			name = "gmail"
		}
		t.Run(name, func(t *testing.T) {
			transport, acc := refreshDeliveryFixture(t, isGmail)
			old := http.DefaultTransport
			http.DefaultTransport = refreshDeliveryRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "gmail.googleapis.com" {
					t.Error("mail submitted after credential conflict")
				}
				current, err := transport.creds.GetOAuthTokens(acc.ID)
				if err != nil {
					return nil, err
				}
				current.AccessToken = "replacement-consent"
				current.RefreshToken = "replacement-refresh"
				current.ExpiresAt = time.Now().Add(time.Hour)
				if err = transport.creds.SetOAuthTokens(acc.ID, current); err != nil {
					return nil, err
				}
				return refreshHTTPResponse(`{"access_token":"stale-response","expires_in":3600}`), nil
			})
			defer func() { http.DefaultTransport = old }()
			err := transport.Deliver(context.Background(), Delivery{AccountID: acc.ID, From: acc.Email, To: []string{"recipient@example.test"}, RFC822: []byte("Subject: test\r\n\r\nbody")})
			var failure *DeliveryError
			if !errors.As(err, &failure) || failure.Kind != SafeRetry || !errors.Is(err, credentials.ErrOAuthTokensChanged) {
				t.Fatalf("want safe pre-submit retry, got %v", err)
			}
			stored, err := transport.creds.GetOAuthTokens(acc.ID)
			if err != nil || stored.AccessToken != "replacement-consent" {
				t.Fatalf("replacement lost: %v", err)
			}
		})
	}
}

type refreshDeliveryRoundTrip func(*http.Request) (*http.Response, error)

func (f refreshDeliveryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
