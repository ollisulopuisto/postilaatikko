package message

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

// Folder clicks run CountConversationsByFolder and ListConversationsByFolder
// over the whole folder. With the unindexed legacy/native Gmail dedupe clause
// both blew their store deadlines (4s/8s) on a Gmail-archive-scale folder, so
// a folder click stalled for tens of seconds. Seed a folder of legacy
// IMAP-cache rows (the worst case: every row evaluates the native-copy
// subquery) and require both calls to complete. The store deadlines make the
// pre-fix code fail this test with context deadline exceeded.
func TestFolderClickStaysUnderStoreDeadlines(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "folder-click.db"))
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
		`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES ('fld', 'acc', 'Archive', 'Archive', 'archive')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	const messages = 20_000
	const threads = 3_000
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO messages (
		id, account_id, folder_id, uid, message_id, thread_id,
		subject, from_name, from_email, date, snippet, is_read, size, received_at
	) VALUES (?, 'acc', 'fld', ?, ?, ?, ?, 'Sender', 's@example.com', ?, 'snippet', 1, 1000, ?)`)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	base := time.Now().Add(-365 * 24 * time.Hour)
	for i := 0; i < messages; i++ {
		thread := fmt.Sprintf("thread-%d", i%threads)
		date := base.Add(time.Duration(i) * time.Minute)
		if _, err := stmt.Exec(fmt.Sprintf("msg-%d", i), i+1,
			fmt.Sprintf("<%d@example.com>", i), thread,
			"Subject "+thread, date, date); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatalf("stmt close: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	store := NewStore(db)

	start := time.Now()
	count, err := store.CountConversationsByFolder("fld", "")
	countDur := time.Since(start)
	if err != nil {
		t.Fatalf("CountConversationsByFolder: %v", err)
	}
	if count != threads {
		t.Fatalf("expected %d conversations, got %d", threads, count)
	}

	start = time.Now()
	convs, err := store.ListConversationsByFolder("fld", 0, 50, "newest", "")
	listDur := time.Since(start)
	if err != nil {
		t.Fatalf("ListConversationsByFolder: %v", err)
	}
	if len(convs) != 50 {
		t.Fatalf("expected 50 conversations, got %d", len(convs))
	}

	t.Logf("folder click on %d messages/%d threads: count %v, first page %v", messages, threads, countDur, listDur)
}
