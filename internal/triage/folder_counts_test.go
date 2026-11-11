package triage

import (
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/database"
)

// ReadLaterFolderCounts powers the folder-panel filter. The Reply Later
// package holds the exhaustive cases (thread IDs, local message IDs,
// multi-folder threads, unsynced entries); here a compact check covers the
// shared query shape against the read_later_queue table.
func TestReadLaterFolderCounts(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "read-later-counts.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO accounts (id, name, email, imap_host, smtp_host, username)
		 VALUES ('acc', 'Account', 'a@example.com', 'imap.example.com', 'smtp.example.com', 'a')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES ('fld', 'acc', 'Inbox', 'Inbox', 'inbox')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO messages (
		id, account_id, folder_id, uid, message_id, thread_id,
		subject, from_name, from_email, date, snippet, is_read, size, received_at
	) VALUES ('m1', 'acc', 'fld', 1, 'm1-mid', 'thr1', 's', 'Sender', 's@example.com', 1, 'snippet', 1, 1000, 1)`); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	store := NewStore(db)
	if _, err := store.SetReadLater(Ref{AccountID: "acc", ConversationID: "thr1"}, true); err != nil {
		t.Fatalf("SetReadLater: %v", err)
	}
	if _, err := store.SetReadLater(Ref{AccountID: "acc", ConversationID: "not-synced"}, true); err != nil {
		t.Fatalf("SetReadLater unsynced: %v", err)
	}

	counts, err := store.ReadLaterFolderCounts()
	if err != nil {
		t.Fatalf("ReadLaterFolderCounts: %v", err)
	}
	if got := counts["acc"]["fld"]; got != 1 {
		t.Errorf("Inbox count = %d, want 1 (%v)", got, counts)
	}
	if len(counts["acc"]) != 1 {
		t.Errorf("folders with counts = %d, want 1 (%v)", len(counts["acc"]), counts["acc"])
	}
}
