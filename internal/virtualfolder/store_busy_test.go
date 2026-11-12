package virtualfolder

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/hkdb/aerion/internal/database"
)

// Reproduces the virtual-folder "failed to begin transaction: database is
// locked (5) (SQLITE_BUSY)" failure: while another connection holds the write
// lock (sync churn, FTS batches), a short-timeout store hits SQLITE_BUSY
// immediately, and only the busy retry lets the write land once the holder
// releases. Uses a 50 ms busy_timeout so contention surfaces in milliseconds
// instead of the production 30 s.
func TestSaveCacheRetriesTransientLockContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vf-busy.db")
	db, err := database.Open(path, database.WithBusyTimeout(50))
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
	if err := store.Create(&VirtualFolder{ID: "vf", Name: "lukematta", FolderIDs: []string{"fld"}}); err != nil {
		t.Fatalf("create virtual folder: %v", err)
	}

	// A second connection simulates the competing writer (sync holding the
	// lock). It holds for 400 ms, far past the store's 50 ms busy_timeout.
	uri := url.URL{Scheme: "file", Path: path}
	holder, err := sql.Open("sqlite", uri.String()+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open holder: %v", err)
	}
	defer holder.Close()
	holderTx, err := holder.Begin()
	if err != nil {
		t.Fatalf("holder begin: %v", err)
	}
	if _, err := holderTx.Exec(`UPDATE virtual_folders SET updated_at = updated_at WHERE id = 'vf'`); err != nil {
		t.Fatalf("holder write: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(400 * time.Millisecond)
		_ = holderTx.Commit()
		close(released)
	}()

	entries := []CachedConversation{{AccountID: "acc", ConversationID: "conv-1"}}
	start := time.Now()
	if err := store.SaveCache("vf", entries); err != nil {
		t.Fatalf("SaveCache under contention: %v (want the busy retry to absorb the 400 ms lock hold)", err)
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Fatalf("SaveCache returned in %s, before the lock was released; retry did not wait for the holder", elapsed)
	}
	<-released

	refs, _, hasCache, err := store.LoadCache("vf")
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if !hasCache || len(refs) != 1 || refs[0].ConversationID != "conv-1" {
		t.Fatalf("cache = %#v hasCache=%v, want conv-1", refs, hasCache)
	}
}
