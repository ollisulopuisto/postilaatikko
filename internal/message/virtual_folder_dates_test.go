package message

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

// A saved view with a date window must find messages whose date the app stored
// as a Go time.Time. Before this was fixed, unixepoch(date) was NULL for them,
// so every windowed view looked empty.
func TestVirtualFolderDateWindowFindsStoredGoTimes(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "vf-dates.db"))
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
	eest := time.FixedZone("EEST", 3*3600)
	seed := []struct {
		id   string
		uid  uint32
		date time.Time
	}{
		{"recent", 1, time.Now().Add(-24 * time.Hour).In(eest)},
		{"old", 2, time.Now().Add(-400 * 24 * time.Hour).In(eest)},
	}
	for _, m := range seed {
		if err := store.Create(&Message{
			ID: m.id, AccountID: "acc", FolderID: "fld", UID: m.uid,
			Subject: m.id, ThreadID: m.id, Date: m.date,
		}); err != nil {
			t.Fatalf("create %s: %v", m.id, err)
		}
	}

	folders := []string{"fld"}
	for _, tt := range []struct {
		window string
		want   int
	}{
		{"all", 2},
		{"7d", 1},
		{"1y", 1},
	} {
		t.Run(tt.window, func(t *testing.T) {
			criteria := VirtualFolderCriteria{DateWindow: tt.window}
			count, err := store.CountVirtualFolderConversations(folders, criteria)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if count != tt.want {
				t.Errorf("count = %d, want %d", count, tt.want)
			}
			list, err := store.ListVirtualFolderConversations(folders, 0, 50, "newest", criteria)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(list) != tt.want {
				t.Errorf("listed %d conversations, want %d", len(list), tt.want)
			}
		})
	}

	t.Run("newest first", func(t *testing.T) {
		list, err := store.ListVirtualFolderConversations(folders, 0, 50, "newest", VirtualFolderCriteria{})
		if err != nil || len(list) != 2 {
			t.Fatalf("list = %d, err = %v", len(list), err)
		}
		if list[0].Subject != "recent" {
			t.Errorf("first = %q, want the recent conversation", list[0].Subject)
		}
	})
}
