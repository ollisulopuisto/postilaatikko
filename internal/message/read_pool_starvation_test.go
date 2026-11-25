package message

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/database"
)

// TestInboxReadsSurviveWriterPoolExhaustion reproduces the multi-second inbox
// stall: one connection holds the write lock while enough writers queue in
// BEGIN IMMEDIATE to occupy every connection of the main pool. WAL reads never
// need the write lock, so list/count/open must still answer from the separate
// read-only pool instead of waiting for a pooled connection.
func TestInboxReadsSurviveWriterPoolExhaustion(t *testing.T) {
	store := seedProfileStore(t, 2000)

	hold, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hold.Exec("UPDATE messages SET is_read=0 WHERE id='acc-1-msg-0'"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < database.MaxOpenConns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := store.db.BeginTx(ctx, nil)
			if err == nil {
				_ = tx.Rollback()
			}
		}()
	}
	t.Cleanup(func() {
		cancel()
		_ = hold.Rollback()
		wg.Wait()
	})
	// Let the queued writers take every main-pool connection.
	deadline := time.Now().Add(5 * time.Second)
	for store.db.Stats().InUse < database.MaxOpenConns && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := store.db.Stats().InUse; got < database.MaxOpenConns {
		t.Fatalf("main pool not exhausted: %d of %d connections in use", got, database.MaxOpenConns)
	}

	const budget = 2 * time.Second
	start := time.Now()
	convs, err := store.ListConversationsByFolder("acc-1-inbox", 0, 50, "newest", "")
	if err != nil || len(convs) == 0 || time.Since(start) > budget {
		t.Fatalf("list during writer pool exhaustion: %d rows, err=%v, %v", len(convs), err, time.Since(start))
	}
	start = time.Now()
	if n, err := store.CountConversationsByFolder("acc-1-inbox", ""); err != nil || n == 0 || time.Since(start) > budget {
		t.Fatalf("count during writer pool exhaustion: %d, err=%v, %v", n, err, time.Since(start))
	}
	start = time.Now()
	if _, _, err := store.ListConversationsByFolderKeyset("acc-1-inbox", "", 50); err != nil || time.Since(start) > budget {
		t.Fatalf("keyset list during writer pool exhaustion: err=%v, %v", err, time.Since(start))
	}
	start = time.Now()
	if conv, err := store.GetConversation(convs[0].ThreadID, "acc-1-inbox"); err != nil || conv == nil || time.Since(start) > budget {
		t.Fatalf("conversation open during writer pool exhaustion: err=%v, %v", err, time.Since(start))
	}
}
