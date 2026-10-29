package message

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

// The virtual-folder index cache stores ordered conversation refs: listing IDs
// applies the saved criteria and text query, and hydrating by refs preserves
// the cached order while reading row state (unread, stars) live.
func TestVirtualFolderConversationIDsAndHydration(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "vf-cache.db"))
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
		`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES ('fld', 'acc', 'INBOX', 'INBOX', 'inbox')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	store := NewStore(db)
	seed := []struct {
		id      string
		uid     uint32
		subject string
		isRead  bool
		age     time.Duration
	}{
		{"m1", 1, "invoice for march", false, time.Hour},
		{"m2", 2, "hello world", false, 2 * time.Hour},
		{"m3", 3, "invoice for april", true, 3 * time.Hour},
	}
	for _, m := range seed {
		if err := store.Create(&Message{
			ID: m.id, AccountID: "acc", FolderID: "fld", UID: m.uid,
			Subject: m.subject, ThreadID: m.id, IsRead: m.isRead,
			Date: time.Now().Add(-m.age),
		}); err != nil {
			t.Fatalf("create %s: %v", m.id, err)
		}
	}

	folders := []string{"fld"}

	t.Run("newest first without query", func(t *testing.T) {
		refs, err := store.ListVirtualFolderConversationIDs(folders, "", 200, VirtualFolderCriteria{})
		if err != nil {
			t.Fatalf("list ids: %v", err)
		}
		if len(refs) != 3 || refs[0].ConversationID != "m1" || refs[2].ConversationID != "m3" {
			t.Errorf("refs = %+v, want m1, m2, m3 newest first", refs)
		}
	})

	t.Run("text query narrows membership", func(t *testing.T) {
		refs, err := store.ListVirtualFolderConversationIDs(folders, "invoice", 200, VirtualFolderCriteria{})
		if err != nil {
			t.Fatalf("list ids: %v", err)
		}
		if len(refs) != 2 || refs[0].ConversationID != "m1" || refs[1].ConversationID != "m3" {
			t.Errorf("refs = %+v, want m1, m3", refs)
		}
	})

	t.Run("criteria combine with query", func(t *testing.T) {
		refs, err := store.ListVirtualFolderConversationIDs(folders, "invoice", 200, VirtualFolderCriteria{ReadStatus: "unread"})
		if err != nil {
			t.Fatalf("list ids: %v", err)
		}
		if len(refs) != 1 || refs[0].ConversationID != "m1" {
			t.Errorf("refs = %+v, want only m1", refs)
		}
	})

	t.Run("limit caps the index", func(t *testing.T) {
		refs, err := store.ListVirtualFolderConversationIDs(folders, "", 1, VirtualFolderCriteria{})
		if err != nil {
			t.Fatalf("list ids: %v", err)
		}
		if len(refs) != 1 || refs[0].ConversationID != "m1" {
			t.Errorf("refs = %+v, want only m1", refs)
		}
	})

	t.Run("hydration preserves cached order and reads state live", func(t *testing.T) {
		// Cached order is stale on purpose: m3 before m1.
		refs := []ConversationRef{
			{AccountID: "acc", ConversationID: "m3"},
			{AccountID: "acc", ConversationID: "m1"},
			{AccountID: "acc", ConversationID: "gone"},
		}
		conversations, err := store.ListConversationsByRefs(refs)
		if err != nil {
			t.Fatalf("hydrate: %v", err)
		}
		if len(conversations) != 2 {
			t.Fatalf("hydrated %d conversations, want 2 (missing ref skipped)", len(conversations))
		}
		if conversations[0].ThreadID != "m3" || conversations[1].ThreadID != "m1" {
			t.Errorf("order = %q, %q; want cached order m3, m1", conversations[0].ThreadID, conversations[1].ThreadID)
		}
		if conversations[1].UnreadCount != 1 {
			t.Errorf("m1 unread = %d, want 1", conversations[1].UnreadCount)
		}

		// Mark m1 read; hydration must reflect it without a cache refresh.
		if _, err := db.Exec(`UPDATE messages SET is_read = 1 WHERE id = 'm1'`); err != nil {
			t.Fatalf("mark read: %v", err)
		}
		conversations, err = store.ListConversationsByRefs(refs)
		if err != nil {
			t.Fatalf("re-hydrate: %v", err)
		}
		if conversations[1].UnreadCount != 0 {
			t.Errorf("m1 unread after read = %d, want 0 (state is live, not cached)", conversations[1].UnreadCount)
		}
	})
}
