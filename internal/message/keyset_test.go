package message

import (
	"fmt"
	"testing"
	"time"
)

// seedKeysetMailbox builds two accounts with inbox+sent folders and
// totalThreads conversations, each thread spanning inbox and sent messages.
// Thread dates strictly descend from the base time with some shared dates to
// exercise ordering tie-breaks. Returns the store and all folder IDs.
func seedKeysetMailbox(t *testing.T, totalThreads int) (*Store, []string) {
	t.Helper()
	s, account, folder := newBodyFailedTestStore(t) // account "acct-1", folder "folder-1"
	if _, err := s.db.Exec(`INSERT INTO accounts (id, name, email, imap_host, smtp_host, username) VALUES ('acct-2', 'Two', 'two@example.com', 'imap.example.com', 'smtp.example.com', 'two@example.com')`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct{ id, account, name, kind string }{
		{"folder-1-sent", account, "Sent", "sent"},
		{"folder-2", "acct-2", "Inbox", "inbox"},
		{"folder-2-sent", "acct-2", "Sent", "sent"},
	} {
		if _, err := s.db.Exec(`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES (?, ?, ?, ?, ?)`, f.id, f.account, f.name, f.id, f.kind); err != nil {
			t.Fatal(err)
		}
	}
	_ = folder
	folders := []string{"folder-1", "folder-1-sent", "folder-2", "folder-2-sent"}
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	uid := 1
	for i := 0; i < totalThreads; i++ {
		threadID := fmt.Sprintf("thread-%03d", i)
		acct := "acct-1"
		inbox, sent := "folder-1", "folder-1-sent"
		if i%2 == 1 {
			acct = "acct-2"
			inbox, sent = "folder-2", "folder-2-sent"
		}
		// Every fourth thread shares its latest date with the previous one.
		latest := base.Add(-time.Duration(i-i%4) * time.Hour)
		msgs := []struct {
			folder string
			when   time.Time
		}{
			{inbox, latest.Add(-2 * time.Hour)},
			{sent, latest.Add(-1 * time.Hour)},
			{inbox, latest},
		}
		for j, m := range msgs {
			msg := &Message{
				AccountID: acct,
				FolderID:  m.folder,
				UID:       uint32(uid),
				MessageID: fmt.Sprintf("<%s-%d@example.com>", threadID, j),
				ThreadID:  threadID,
				Subject:   "Subject " + threadID,
				Date:      m.when,
			}
			uid++
			if err := s.Create(msg); err != nil {
				t.Fatal(err)
			}
		}
	}
	return s, folders
}

func walkKeysetPages(t *testing.T, s *Store, folders []string, limit int, sortOrder string, excludeInboxCopies bool) []*Conversation {
	t.Helper()
	var all []*Conversation
	cursor := ""
	for pages := 0; pages < 100; pages++ {
		page, next, err := s.ListConversationsByFolderIDsKeyset(folders, cursor, limit, sortOrder, excludeInboxCopies)
		if err != nil {
			t.Fatalf("keyset page %d: %v", pages, err)
		}
		if len(page) > limit {
			t.Fatalf("page %d returned %d conversations, limit %d", pages, len(page), limit)
		}
		all = append(all, page...)
		if next == "" {
			return all
		}
		if next == cursor {
			t.Fatalf("cursor did not advance on page %d", pages)
		}
		cursor = next
	}
	t.Fatal("pagination did not terminate within 100 pages")
	return nil
}

func conversationIDs(convs []*Conversation) []string {
	out := make([]string, len(convs))
	for i, c := range convs {
		out[i] = c.AccountID + "/" + c.ThreadID
	}
	return out
}

func TestKeysetMatchesOffsetPathNewest(t *testing.T) {
	s, folders := seedKeysetMailbox(t, 37)
	want, err := s.ListConversationsByFolderIDs(folders, 0, 1000, "newest", "")
	if err != nil {
		t.Fatal(err)
	}
	got := walkKeysetPages(t, s, folders, 7, "newest", false)
	if len(got) != len(want) {
		t.Fatalf("keyset returned %d conversations, offset path %d", len(got), len(want))
	}
	for i := range want {
		if got[i].AccountID != want[i].AccountID || got[i].ThreadID != want[i].ThreadID {
			t.Fatalf("position %d: keyset %v, offset %v", i, conversationIDs(got[i:i+1]), conversationIDs(want[i:i+1]))
		}
		if got[i].MessageCount != want[i].MessageCount || got[i].Subject != want[i].Subject || !got[i].LatestDate.Equal(want[i].LatestDate) {
			t.Fatalf("position %d content mismatch: keyset %+v, offset %+v", i, got[i], want[i])
		}
		if len(got[i].MessageIDs) != len(want[i].MessageIDs) {
			t.Fatalf("position %d message ids: keyset %v, offset %v", i, got[i].MessageIDs, want[i].MessageIDs)
		}
	}
}

func TestKeysetRejectsOldestOrdering(t *testing.T) {
	s, folders := seedKeysetMailbox(t, 3)
	if _, _, err := s.ListConversationsByFolderIDsKeyset(folders, "", 5, "oldest", false); err == nil {
		t.Fatal("expected oldest ordering to be rejected; it stays on the offset path")
	}
}

func TestKeysetSkipsConversationsInsertedBetweenPages(t *testing.T) {
	s, folders := seedKeysetMailbox(t, 20)
	before, err := s.ListConversationsByFolderIDs(folders, 0, 1000, "newest", "")
	if err != nil {
		t.Fatal(err)
	}
	_, cursor, err := s.ListConversationsByFolderIDsKeyset(folders, "", 5, "newest", false)
	if err != nil {
		t.Fatal(err)
	}
	if cursor == "" {
		t.Fatal("expected a cursor after the first page")
	}
	// A newer conversation arriving between pages belongs to the region the
	// client already saw; later pages must not shift or duplicate.
	fresh := &Message{AccountID: "acct-1", FolderID: "folder-1", UID: 90001, MessageID: "<fresh@example.com>", ThreadID: "thread-fresh", Subject: "Fresh", Date: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)}
	if err := s.Create(fresh); err != nil {
		t.Fatal(err)
	}
	rest := []*Conversation{}
	for cursor != "" {
		var page []*Conversation
		var err error
		page, cursor, err = s.ListConversationsByFolderIDsKeyset(folders, cursor, 5, "newest", false)
		if err != nil {
			t.Fatal(err)
		}
		rest = append(rest, page...)
	}
	if len(rest) != len(before)-5 {
		t.Fatalf("pages after insert returned %d conversations, want %d", len(rest), len(before)-5)
	}
	for i, c := range rest {
		want := before[5+i]
		if c.AccountID != want.AccountID || c.ThreadID != want.ThreadID {
			t.Fatalf("position %d after insert: got %s/%s, want %s/%s", i, c.AccountID, c.ThreadID, want.AccountID, want.ThreadID)
		}
	}
}

func TestKeysetRejectsMalformedCursors(t *testing.T) {
	s, folders := seedKeysetMailbox(t, 3)
	for _, cursor := range []string{"k1:not-base64!!", "k1:aGVsbG8", "10", KeysetCursorPrefix} {
		if _, _, err := s.ListConversationsByFolderIDsKeyset(folders, cursor, 5, "newest", false); err == nil {
			t.Fatalf("cursor %q: expected an error", cursor)
		}
	}
	// Sort order must not change mid-pagination.
	_, cursor, err := s.ListConversationsByFolderIDsKeyset(folders, "", 2, "newest", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ListConversationsByFolderIDsKeyset(folders, cursor, 2, "oldest", false); err == nil {
		t.Fatal("expected an error when sort order changes mid-pagination")
	}
}

func TestKeysetArchiveVariantHidesInboxCopies(t *testing.T) {
	s, account, inbox := newBodyFailedTestStore(t)
	if _, err := s.db.Exec(`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES ('folder-arch', ?, 'Archive', 'folder-arch', 'archive')`, account); err != nil {
		t.Fatal(err)
	}
	// Same RFC message in Inbox and Archive: the Archive list must hide it.
	shared := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	for i, f := range []string{inbox, "folder-arch"} {
		m := &Message{AccountID: account, FolderID: f, UID: uint32(500 + i), MessageID: "<shared@example.com>", ThreadID: "thread-shared", Subject: "Shared", Date: shared}
		if err := s.Create(m); err != nil {
			t.Fatal(err)
		}
	}
	archOnly := &Message{AccountID: account, FolderID: "folder-arch", UID: 600, MessageID: "<arch@example.com>", ThreadID: "thread-arch", Subject: "Archived", Date: shared.Add(-time.Hour)}
	if err := s.Create(archOnly); err != nil {
		t.Fatal(err)
	}
	got, next, err := s.ListConversationsByFolderIDsKeyset([]string{"folder-arch"}, "", 50, "newest", true)
	if err != nil {
		t.Fatal(err)
	}
	if next != "" {
		t.Fatalf("unexpected cursor %q", next)
	}
	if len(got) != 1 || got[0].ThreadID != "thread-arch" {
		t.Fatalf("archive keyset page = %v, want only thread-arch", conversationIDs(got))
	}
}

func TestKeysetInterleavedLongThreadsDoNotRepeat(t *testing.T) {
	s, folders := seedKeysetMailbox(t, 3)
	// A spans the cursor: A newest, B next, A old, C last.
	for thread, date := range map[string]string{"thread-000": "2026-11-01T12:00:00Z", "thread-001": "2026-11-01T11:00:00Z", "thread-002": "2026-11-01T09:00:00Z"} {
		if _, err := s.db.Exec(`UPDATE messages SET date=? WHERE thread_id=?`, date, thread); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`UPDATE messages SET date='2026-11-01T10:00:00Z' WHERE id=(SELECT id FROM messages WHERE thread_id='thread-000' LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	got := walkKeysetPages(t, s, folders, 2, "newest", false)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3 unique conversations: %v", len(got), conversationIDs(got))
	}
}

func TestKeysetDoesNotHoldOneConnectionPerFolder(t *testing.T) {
	s, folders := seedKeysetMailbox(t, 4)
	s.db.SetMaxOpenConns(1)
	defer s.db.SetMaxOpenConns(12)
	start := time.Now()
	_, _, err := s.ListConversationsByFolderIDsKeyset(folders, "", 2, "newest", false)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("pooled connection starvation")
	}
}

func TestPhysicalFolderKeysetPreservesLegacySemantics(t *testing.T) {
	for _, kind := range []string{"sent", "drafts", "inbox"} {
		s, _, fid := newBodyFailedTestStore(t)
		s.db.Exec(`UPDATE folders SET folder_type=? WHERE id=?`, kind, fid)
		if kind == "inbox" {
			s.db.Exec(`UPDATE accounts SET enabled=0`)
		}
		m := &Message{AccountID: "acct-1", FolderID: fid, UID: 1, MessageID: "<test@example.test>", ThreadID: "one", Subject: "hi", FromEmail: "me@example.test", Date: time.Now()}
		if err := s.Create(m); err != nil {
			t.Fatal(err)
		}
		s.db.Exec(`UPDATE messages SET to_list='[{"name":"Bob","email":"bob@example.test"}]'`)
		want, err := s.ListConversationsByFolder(fid, 0, 10, "newest", "")
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := s.ListConversationsByFolderKeyset(fid, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) || len(got) != 1 {
			t.Fatalf("%s rows: got %d want %d", kind, len(got), len(want))
		}
		if fmt.Sprint(got[0].Participants) != fmt.Sprint(want[0].Participants) {
			t.Fatalf("%s participants mismatch: got %v want %v", kind, got[0].Participants, want[0].Participants)
		}
	}
}
