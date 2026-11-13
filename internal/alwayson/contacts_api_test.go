package alwayson

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/account"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/imap"
)

func openContactsAPITestRuntime(t *testing.T) *SyncRuntime {
	t.Helper()
	dataDir := t.TempDir()
	db, err := database.Open(filepath.Join(dataDir, "aerion.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := OpenSyncRuntime(dataDir, func(context.Context, string) (*imap.ClientConfig, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	})
	return runtime
}

// The production handler must mount the contacts surface: a remote desktop
// sends every contacts call here, and an unmounted route is a silent 404
// behind the capability check.
func TestMailboxReadAPIHandlerMountsContactsRoutes(t *testing.T) {
	runtime := openContactsAPITestRuntime(t)
	handler, err := runtime.MailboxReadAPIHandler()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/contacts/sources"},
		{http.MethodPost, "/v1/contacts/search"},
		{http.MethodPost, "/v1/contacts/mutations"},
		{http.MethodPost, "/v1/contacts/sources/tokens"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s %s returned 404: contacts routes are not mounted", tc.method, tc.path)
		}
	}
}

// A contacts.manage grant must not be able to overwrite another surface's
// credentials through the source-tokens route.
func TestStoreSourceTokensRejectsNonContactsClientConfig(t *testing.T) {
	runtime := openContactsAPITestRuntime(t)
	ops := &contactsOperations{runtime: runtime.contacts}

	err := ops.StoreSourceTokens(v1.ContactsSourceTokensRequest{
		Provider: "google", AccountID: "acc-1", ClientConfigID: "google-mail",
		Tokens: v1.ContactsTokenSet{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600},
	})
	assertInvalidRequestFault(t, err, "mail client config must be rejected")

	err = ops.StoreSourceTokens(v1.ContactsSourceTokensRequest{
		Provider: "custom", AccountID: "acc-1", ClientConfigID: "custom-contacts",
		Tokens: v1.ContactsTokenSet{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600},
	})
	assertInvalidRequestFault(t, err, "non-OAuth contacts provider must be rejected")

	// The contacts config for the matching provider passes the guard and
	// reaches the account lookup, which reports the unknown account.
	err = ops.StoreSourceTokens(v1.ContactsSourceTokensRequest{
		Provider: "google", AccountID: "acc-missing", ClientConfigID: "google-contacts",
		Tokens: v1.ContactsTokenSet{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600},
	})
	if !errors.Is(err, v1.ErrContactNotFound) {
		t.Fatalf("expected ErrContactNotFound for unknown account, got %v", err)
	}
}

func assertInvalidRequestFault(t *testing.T, err error, what string) {
	t.Helper()
	var fault *v1.Fault
	if !errors.As(err, &fault) || fault.Code != v1.CodeInvalidRequest {
		t.Fatalf("%s: expected invalid_request fault, got %v", what, err)
	}
}

// Bearer-token discovery must stay on the account's own mail server over
// HTTPS; anything else leaks mailbox tokens to a caller-chosen host.
func TestValidateContactsDiscoveryURL(t *testing.T) {
	acc := &account.Account{IMAPHost: "mail.example.com", SMTPHost: "smtp.example.com"}
	for _, tc := range []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"imap host https", "https://mail.example.com/dav/", false},
		{"smtp host https", "https://smtp.example.com/.well-known/carddav", false},
		{"subdomain of account host", "https://dav.mail.example.com/", false},
		{"plain http", "http://mail.example.com/dav/", true},
		{"unrelated host", "https://evil.example.net/", true},
		{"suffix lookalike", "https://mail.example.com.evil.net/", true},
		{"empty", "", true},
		{"not a url", "://bad", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateContactsDiscoveryURL(tc.url, acc)
			if tc.wantErr && err == nil {
				t.Fatalf("expected %q to be rejected", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected %q to pass, got %v", tc.url, err)
			}
		})
	}

	localAcc := &account.Account{IMAPHost: "localhost", SMTPHost: "localhost"}
	if err := validateContactsDiscoveryURL("http://localhost:8080/dav/", localAcc); err != nil {
		t.Fatalf("loopback http for a loopback account host should pass, got %v", err)
	}
	if err := validateContactsDiscoveryURL("http://localhost:8080/dav/", acc); err == nil {
		t.Fatal("loopback http must still match the account host")
	}
}
