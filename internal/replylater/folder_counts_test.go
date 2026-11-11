package replylater

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/database"
)

// FolderCounts powers the folder-panel filter: the tree needs to know which
// folders contain Reply Later conversations. Seed messages across folders and
// require per-folder counts keyed by account and folder, covering provider
// thread IDs, local single-message IDs, multi-folder threads, and queue
// entries whose messages are not synced yet.
func TestFolderCounts(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "reply-later-counts.db"))
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
	for i, name := range []string{"Inbox", "Archive", "Sent"} {
		if _, err := db.Exec(
			`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES (?, 'acc', ?, ?, ?)`,
			fmt.Sprintf("fld%d", i+1), name, name, "inbox"); err != nil {
			t.Fatalf("seed folder %s: %v", name, err)
		}
	}
	uid := 0
	insert := func(id, folderID string, threadID interface{}) {
		uid++
		if _, err := db.Exec(`INSERT INTO messages (
			id, account_id, folder_id, uid, message_id, thread_id,
			subject, from_name, from_email, date, snippet, is_read, size, received_at
		) VALUES (?, 'acc', ?, ?, ?, ?, 's', 'Sender', 's@example.com', 1, 'snippet', 1, 1000, 1)`,
			id, folderID, uid, id+"-mid", threadID); err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}
	// Thread "thr1" spans Inbox and Archive; thread "thr2" sits in Sent;
	// message "msg1" has no thread (local single-message conversation).
	insert("m1", "fld1", "thr1")
	insert("m2", "fld2", "thr1")
	insert("m3", "fld3", "thr2")
	insert("msg1", "fld2", nil)

	store := NewStore(db)
	for _, ref := range []Ref{
		{AccountID: "acc", ConversationID: "thr1"},
		{AccountID: "acc", ConversationID: "thr2"},
		{AccountID: "acc", ConversationID: "msg1"},
		{AccountID: "acc", ConversationID: "not-synced"},
	} {
		if _, err := store.Add(ref); err != nil {
			t.Fatalf("add %v: %v", ref, err)
		}
	}

	counts, err := store.FolderCounts()
	if err != nil {
		t.Fatalf("FolderCounts: %v", err)
	}
	acc := counts["acc"]
	if acc == nil {
		t.Fatalf("no counts for account: %v", counts)
	}
	if got := acc["fld1"]; got != 1 {
		t.Errorf("Inbox count = %d, want 1", got)
	}
	// Archive holds thread thr1 and local message msg1.
	if got := acc["fld2"]; got != 2 {
		t.Errorf("Archive count = %d, want 2", got)
	}
	if got := acc["fld3"]; got != 1 {
		t.Errorf("Sent count = %d, want 1", got)
	}
	// The unsynced queue entry must not appear anywhere.
	for folderID, n := range acc {
		if n <= 0 {
			t.Errorf("non-positive count for %s: %d", folderID, n)
		}
	}
	if len(acc) != 3 {
		t.Errorf("folders with counts = %d, want 3 (%v)", len(acc), acc)
	}
}
