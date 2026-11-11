package message

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/database"
)

// FolderCountsForConversations powers the folder panel's Read Later / Reply
// Later filters: queue refs carry only account+conversation IDs, so folder
// membership is resolved through the message index. Conversation identity
// mirrors COALESCE(thread_id, id): provider thread IDs match
// messages.thread_id, local single-message IDs match messages.id.
func TestFolderCountsForConversations(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "folder-counts.db"))
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
	counts, err := store.FolderCountsForConversations(map[string][]string{
		"acc": {"thr1", "thr2", "msg1", "not-synced"},
	})
	if err != nil {
		t.Fatalf("FolderCountsForConversations: %v", err)
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
	if len(acc) != 3 {
		t.Errorf("folder count keys = %v, want 3 folders", acc)
	}

	// Unknown accounts and empty input produce no rows.
	other, err := store.FolderCountsForConversations(map[string][]string{"nope": {"thr1"}, "empty": {}})
	if err != nil {
		t.Fatalf("FolderCountsForConversations unknown account: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("unknown account counts = %v, want empty", other)
	}
}
