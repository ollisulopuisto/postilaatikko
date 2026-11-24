package accountsetup

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/credentials"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/syncowner"
	gokeyring "github.com/zalando/go-keyring"
)

func TestCreateStoresHeadlessPasswordAndRejectsDuplicate(t *testing.T) {
	gokeyring.MockInitWithError(errors.New("no secret service"))
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	config := account.AccountConfig{Name: "Example", DisplayName: "Example", Email: "me@example.org", Username: "me@example.org", IMAPHost: "imap.example.org", SMTPHost: "smtp.example.org"}
	acc, err := s.Create(config, "test-only-password", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := credentials.NewStore(s.db.DB, dir)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := c.GetPassword(acc.ID); err != nil || p != "test-only-password" {
		t.Fatal("password not stored in fallback")
	}
	if _, err = s.Create(config, "different", "", nil); !errors.Is(err, account.ErrAccountExists) {
		t.Fatalf("duplicate=%v", err)
	}
	rows, err := account.NewStore(s.db).List()
	if err != nil || len(rows) != 1 {
		t.Fatal("duplicate changed accounts")
	}
}

func TestOpenRefusesActiveSchemaReaderWithoutChangingDatabase(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "aerion.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	db.Close()
	lock, err := syncowner.TryAcquireShared(filepath.Join(dir, "mail-schema.owner.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if s, err := Open(dir); err == nil {
		s.Close()
		t.Fatal("accepted active app/helper")
	}
}

func TestCreateRejectsMissingOAuthIdentityAndRefreshBeforeWrite(t *testing.T) {
	gokeyring.MockInitWithError(errors.New("no secret service"))
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cfg := account.AccountConfig{Name: "Google", DisplayName: "Me", Email: "me@example.org", Username: "me@example.org", IMAPHost: "imap.gmail.com", SMTPHost: "smtp.gmail.com", AuthType: account.AuthOAuth2}
	for _, tokens := range []*credentials.OAuthTokens{nil, {Provider: "google", AccessToken: "short-lived"}} {
		if _, err = s.Create(cfg, "", "", tokens); err == nil {
			t.Fatal("accepted incomplete OAuth")
		}
	}
	rows, err := account.NewStore(s.db).List()
	if err != nil || len(rows) != 0 {
		t.Fatal("incomplete auth created account")
	}
}

func TestCredentialFailureRollsBackNewAccount(t *testing.T) {
	gokeyring.MockInitWithError(errors.New("no secret service"))
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`CREATE TRIGGER fail_password BEFORE UPDATE OF encrypted_password ON accounts WHEN NEW.encrypted_password IS NOT NULL BEGIN SELECT RAISE(FAIL, 'fixture credential failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := account.AccountConfig{Name: "Example", DisplayName: "Me", Email: "me@example.org", Username: "me@example.org", IMAPHost: "imap.example.org", SMTPHost: "smtp.example.org"}
	if _, err = s.Create(cfg, "secret-fixture", "", nil); err == nil {
		t.Fatal("failed credential save accepted")
	}
	rows, err := s.accounts.List()
	if err != nil || len(rows) != 0 {
		t.Fatal("orphan account remains")
	}
}

func TestProviderHonorsExplicitShippedChoice(t *testing.T) {
	gokeyring.MockInitWithError(errors.New("no secret service"))
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.creds.SetUserClientCreds("google-mail", "custom-fixture", "desktop-fixture"); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Provider("google")
	if err != nil || cfg.ClientID != "custom-fixture" {
		t.Fatal("saved client not selected")
	}
	if err = s.creds.SetOAuthActiveChoice("google-mail", "aerion-shipped"); err != nil {
		t.Fatal(err)
	}
	cfg, err = s.Provider("google")
	if err != nil || cfg.ClientID == "custom-fixture" || cfg.ClientID == "" {
		t.Fatal("explicit shipped choice ignored")
	}
}

func TestIdentityInsertFailureDoesNotLeavePartialAccount(t *testing.T) {
	gokeyring.MockInitWithError(errors.New("no keyring"))
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`CREATE TRIGGER fail_identity BEFORE INSERT ON identities BEGIN SELECT RAISE(FAIL,'identity fixture'); END`); err != nil {
		t.Fatal(err)
	}
	cfg := account.AccountConfig{Name: "Example", DisplayName: "Me", Email: "me@example.org", Username: "me@example.org", IMAPHost: "imap.example.org", SMTPHost: "smtp.example.org"}
	if _, err = s.Create(cfg, "secret-fixture", "", nil); err == nil {
		t.Fatal("identity failure accepted")
	}
	rows, err := s.accounts.List()
	if err != nil || len(rows) != 0 {
		t.Fatal("partial account remains")
	}
}

func TestOAuthPersistsRefreshGrantAcrossReopenWithoutKeyring(t *testing.T) {
	gokeyring.MockInitWithError(errors.New("no keyring"))
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := account.AccountConfig{Name: "Google", DisplayName: "Me", Email: "me@example.org", Username: "me@example.org", IMAPHost: "imap.gmail.com", SMTPHost: "smtp.gmail.com", AuthType: account.AuthOAuth2}
	tokens := &credentials.OAuthTokens{Provider: "google", AccessToken: "access-fixture", RefreshToken: "refresh-fixture", ExpiresAt: time.Now().Add(time.Hour), Scopes: []string{"https://mail.google.com/"}}
	a, err := s.Create(cfg, "", "", tokens)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := s.creds.GetOAuthTokens(a.ID)
	if err != nil || restored.RefreshToken != "refresh-fixture" || restored.Provider != "google" {
		t.Fatal("OAuth grant not restored")
	}
}
