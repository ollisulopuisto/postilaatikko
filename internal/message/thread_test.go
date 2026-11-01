package message

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

func setupThreadTestDB(t testing.TB) (*Store, *database.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "thread-test.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO accounts (id, name, email, imap_host, smtp_host, username)
		 VALUES ('acc-1', 'Test Account', 'test@example.com', 'imap.example.com', 'smtp.example.com', 'test')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES ('fld-1', 'acc-1', 'INBOX', 'INBOX', 'inbox')`); err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	store := NewStore(db)
	return store, db
}

func TestFindThreadID(t *testing.T) {
	store, db := setupThreadTestDB(t)
	defer db.Close()

	// Seed existing messages in the DB:
	// Message 1: root message with message_id "root-msg@example.com" and thread_id "thread-root"
	// Message 2: reply with message_id "<reply-bracketed@example.com>" and thread_id "thread-reply"
	now := time.Now()
	msg1 := &Message{
		ID:        "msg-1",
		AccountID: "acc-1",
		FolderID:  "fld-1",
		UID:       101,
		MessageID: "root-msg@example.com",
		ThreadID:  "thread-root",
		Date:      now,
	}
	msg2 := &Message{
		ID:        "msg-2",
		AccountID: "acc-1",
		FolderID:  "fld-1",
		UID:       102,
		MessageID: "<reply-bracketed@example.com>",
		ThreadID:  "thread-reply",
		Date:      now,
	}
	if err := store.Create(msg1); err != nil {
		t.Fatalf("store.Create msg1: %v", err)
	}
	if err := store.Create(msg2); err != nil {
		t.Fatalf("store.Create msg2: %v", err)
	}

	tests := []struct {
		name       string
		accountID  string
		messageID  string
		inReplyTo  string
		references []string
		want       string
	}{
		{
			name:       "No threading info returns normalized own message ID",
			accountID:  "acc-1",
			messageID:  "<new-thread@example.com>",
			inReplyTo:  "",
			references: nil,
			want:       "new-thread@example.com",
		},
		{
			name:       "Matches In-Reply-To to existing message",
			accountID:  "acc-1",
			messageID:  "msg-3@example.com",
			inReplyTo:  "root-msg@example.com",
			references: []string{"other-msg@example.com"},
			want:       "thread-root",
		},
		{
			name:       "Matches In-Reply-To with bracket variation",
			accountID:  "acc-1",
			messageID:  "msg-4@example.com",
			inReplyTo:  "reply-bracketed@example.com", // stored in DB with angle brackets
			references: nil,
			want:       "thread-reply",
		},
		{
			name:       "Matches Reference when In-Reply-To is missing or doesn't match DB",
			accountID:  "acc-1",
			messageID:  "msg-5@example.com",
			inReplyTo:  "unmatched-parent@example.com",
			references: []string{"unknown-1@example.com", "root-msg@example.com"},
			want:       "thread-root",
		},
		{
			name:       "Preserves priority order: inReplyTo wins over references when both match",
			accountID:  "acc-1",
			messageID:  "msg-6@example.com",
			inReplyTo:  "reply-bracketed@example.com",
			references: []string{"root-msg@example.com"},
			want:       "thread-reply",
		},
		{
			name:       "Preserves reference order: earlier reference wins over later reference",
			accountID:  "acc-1",
			messageID:  "msg-7@example.com",
			inReplyTo:  "",
			references: []string{"reply-bracketed@example.com", "root-msg@example.com"},
			want:       "thread-reply",
		},
		{
			name:       "Fallback to first reference when none in DB",
			accountID:  "acc-1",
			messageID:  "msg-8@example.com",
			inReplyTo:  "",
			references: []string{"<nonexistent-root@example.com>", "nonexistent-2@example.com"},
			want:       "nonexistent-root@example.com",
		},
		{
			name:       "Fallback to inReplyTo when none in DB and no references",
			accountID:  "acc-1",
			messageID:  "msg-9@example.com",
			inReplyTo:  "<nonexistent-reply@example.com>",
			references: nil,
			want:       "nonexistent-reply@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.FindThreadID(tt.accountID, tt.messageID, tt.inReplyTo, tt.references)
			if err != nil {
				t.Fatalf("FindThreadID returned unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("FindThreadID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkFindThreadID(b *testing.B) {
	store, db := setupThreadTestDB(b)
	defer db.Close()

	// Seed 20 existing messages
	now := time.Now()
	for i := 1; i <= 20; i++ {
		msg := &Message{
			ID:        fmt.Sprintf("seed-msg-%d", i),
			AccountID: "acc-1",
			FolderID:  "fld-1",
			UID:       uint32(200 + i),
			MessageID: fmt.Sprintf("msg-%d@example.com", i),
			ThreadID:  fmt.Sprintf("thread-%d", i),
			Date:      now,
		}
		if err := store.Create(msg); err != nil {
			b.Fatalf("seed message: %v", err)
		}
	}

	references := []string{
		"unknown-1@example.com",
		"unknown-2@example.com",
		"unknown-3@example.com",
		"unknown-4@example.com",
		"msg-15@example.com",
		"msg-10@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		threadID, err := store.FindThreadID("acc-1", "incoming@example.com", "unknown-parent@example.com", references)
		if err != nil {
			b.Fatalf("FindThreadID error: %v", err)
		}
		if threadID != "thread-15" {
			b.Fatalf("unexpected threadID: %s", threadID)
		}
	}
}
