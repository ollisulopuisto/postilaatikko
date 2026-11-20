package message

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/database"
)

// seedProfileStore builds a synthetic mailbox of totalMessages messages
// across one account's inbox+sent folders for query profiling (PERF-SC-005).
// It mirrors the apibench fixture shape: 4-message threads, every tenth
// message in Sent.
func seedProfileStore(t *testing.T, totalMessages int) *Store {
	t.Helper()
	return seedProfileStoreOpts(t, totalMessages, false)
}

// seedProfileStoreOpts seeds the profile fixture; dropKeysetIndexes removes
// the PERF-SC-005 indexes first so write-cost comparisons run against the
// pre-migration-84 schema.
func seedProfileStoreOpts(t *testing.T, totalMessages int, dropKeysetIndexes bool) *Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if dropKeysetIndexes {
		for _, idx := range []string{"idx_messages_folder_date_conv", "idx_messages_account_conv"} {
			if _, err := db.Exec("DROP INDEX " + idx); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`INSERT INTO accounts (id, name, email, imap_host, smtp_host, username) VALUES ('acc-1', 'Bench', 'bench@example.com', 'imap.example.com', 'smtp.example.com', 'bench')`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct{ id, kind string }{
		{"acc-1-inbox", "inbox"}, {"acc-1-sent", "sent"},
	} {
		if _, err := db.Exec(`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES (?, 'acc-1', ?, ?, ?)`, f.id, f.id, f.id, f.kind); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO messages (id, account_id, folder_id, uid, message_id, thread_id, subject, from_name, from_email, to_list, date, snippet, is_read, has_attachments, is_starred, smime_encrypted, pgp_encrypted, inbox_category)
		VALUES (?, 'acc-1', ?, ?, ?, ?, ?, ?, ?, '["bench@example.com"]', ?, 'snippet', 1, 0, 0, 0, 0, '')`)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-400 * 24 * time.Hour).UTC()
	for i := 0; i < totalMessages; i++ {
		folderID := "acc-1-inbox"
		if i%10 == 9 {
			folderID = "acc-1-sent"
		}
		threadID := fmt.Sprintf("thread-%d", i/4)
		msgID := fmt.Sprintf("<m-%d@bench>", i)
		id := fmt.Sprintf("acc-1-msg-%d", i)
		date := base.Add(time.Duration(i) * 30 * time.Minute).Format("2006-01-02 15:04:05")
		if _, err := stmt.Exec(id, folderID, i+1, msgID, threadID, fmt.Sprintf("Subject %d", i), "Sender", "s@x.net", date); err != nil {
			t.Fatal(err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

// TestConversationListQueryProfile prints the EXPLAIN QUERY PLAN and page
// timings for the conversation list query at several offsets. Run with:
//
//	PERF_PROFILE=1 go test ./internal/message/ -run QueryProfile -v
func TestConversationListQueryProfile(t *testing.T) {
	if os.Getenv("PERF_PROFILE") != "1" {
		t.Skip("set PERF_PROFILE=1 to run query profiling")
	}
	const total = 20000 // 5000 threads
	store := seedProfileStore(t, total)
	folders := []string{"acc-1-inbox", "acc-1-sent"}

	query := buildConversationListQuery(len(folders), "newest", "", false)
	rows, err := store.db.Query("EXPLAIN QUERY PLAN " + query[:len(query)-len("LIMIT ? OFFSET ?\n\t")] + "LIMIT 50 OFFSET 0")
	if err != nil {
		// Fall back to explaining with placeholders bound via args below.
		rows, err = store.db.Query("EXPLAIN QUERY PLAN "+query, folders[0], folders[1], 50, 0)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	t.Log("EXPLAIN QUERY PLAN for the conversation list query:")
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		t.Logf("  %s", detail)
	}

	for _, offset := range []int{0, 1250, 2500, 3750} {
		start := time.Now()
		convs, err := store.ListConversationsByFolderIDs(folders, offset, 50, "newest", "")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("offset path  page at offset %5d: %6d conversations in %v", offset, len(convs), time.Since(start))
	}

	// Keyset path: walk to the same positions by cursor.
	cursor := ""
	pageSize := 50
	targets := map[int]bool{0: true, 24: true, 49: true, 74: true} // 50 per page: 0, 1200, 2450, 3700
	for page := 0; page < 100; page++ {
		start := time.Now()
		convs, next, err := store.ListConversationsByFolderIDsKeyset(folders, cursor, pageSize, "newest", false)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if targets[page] {
			t.Logf("keyset path  page %4d (rows %5d+): %6d conversations in %v", page, page*pageSize, len(convs), elapsed)
		}
		if next == "" {
			break
		}
		cursor = next
	}
}

// TestConversationListIndexWriteCost measures the insert cost of the
// PERF-SC-005 indexes by timing fixture seeding with and without them.
//
//	PERF_PROFILE=1 go test ./internal/message/ -run IndexWriteCost -v
func TestConversationListIndexWriteCost(t *testing.T) {
	if os.Getenv("PERF_PROFILE") != "1" {
		t.Skip("set PERF_PROFILE=1 to run index write-cost profiling")
	}
	const total = 20000
	start := time.Now()
	seedProfileStoreOpts(t, total, true)
	without := time.Since(start)
	start = time.Now()
	seedProfileStoreOpts(t, total, false)
	with := time.Since(start)
	t.Logf("seeding %d messages: without PERF-SC-005 indexes %v, with %v (delta %v, %.1f%%)",
		total, without, with, with-without, 100*float64(with-without)/float64(without))
}
