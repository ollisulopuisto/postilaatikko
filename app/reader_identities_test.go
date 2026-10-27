package app

import (
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"path/filepath"
	"testing"
)

func TestReaderIdentityIncludesDisabledHistoricalAccount(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES('old','Old account','historic@example.com','imap.example.com','smtp.example.com','me',0)`); err != nil {
		t.Fatal(err)
	}
	app := &App{accountStore: account.NewStore(db)}
	emails, err := app.GetReaderIdentityEmails()
	if err != nil || len(emails) < 1 || emails[0] != "historic@example.com" {
		t.Fatalf("identities: %v, %v", emails, err)
	}
	var enabled bool
	if err = db.QueryRow("SELECT enabled FROM accounts WHERE id='old'").Scan(&enabled); err != nil || enabled {
		t.Fatal("reading identities enabled the account")
	}
}
