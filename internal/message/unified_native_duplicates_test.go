package message

import (
	"fmt"
	"testing"
	"time"
)

func TestUnifiedInboxLegacyAndNativeCopiesDoNotDuplicate(t *testing.T) {
	s, account, folder := newBodyFailedTestStore(t)
	legacy := &Message{AccountID: account, FolderID: folder, UID: 12, MessageID: "message@example.org", ThreadID: "message@example.org", Subject: "One message", Date: time.Now()}
	if err := s.Create(legacy); err != nil {
		t.Fatal(err)
	}
	native := &Message{AccountID: account, FolderID: folder, MessageID: "<message@example.org>", ThreadID: "gmail:" + account + ":thread", Subject: "One message", Date: legacy.Date}
	native.UID = 13
	if err := s.Create(native); err != nil {
		t.Fatal(err)
	}
	if err := s.BindGmailID(account, native.ID, "gmail-provider"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListConversationsByFolderIDs([]string{folder}, 0, 50, "newest", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("duplicate legacy/native rows: %d %v", len(rows), err)
	}
	single, err := s.ListConversationsByFolder(folder, 0, 50, "newest", "")
	if err != nil || len(single) != 1 {
		t.Fatalf("single inbox duplicate: %d %v", len(single), err)
	}
	singleCount, err := s.CountConversationsByFolder(folder, "")
	if err != nil || singleCount != 1 {
		t.Fatalf("single inbox count: %d %v", singleCount, err)
	}
	count, err := s.CountConversationsByFolderIDs([]string{folder}, "")
	if err != nil || count != 1 {
		t.Fatalf("count = %d %v", count, err)
	}
}

func TestGmailUpsertAdoptsUniqueLegacyIDWithoutBrackets(t *testing.T) {
	s, account, folder := newBodyFailedTestStore(t)
	legacy := &Message{AccountID: account, FolderID: folder, UID: 12, MessageID: "message@example.org", ThreadID: "old", Date: time.Now(), BodyText: "preserve", BodyFetched: true}
	if err := s.Create(legacy); err != nil {
		t.Fatal(err)
	}
	native := &Message{AccountID: account, FolderID: folder, MessageID: "<message@example.org>", ThreadID: "gmail:thread", Date: legacy.Date}
	if err := s.UpsertGmail(native, "provider"); err != nil {
		t.Fatal(err)
	}
	if native.ID != legacy.ID {
		t.Fatal("Gmail API created a second row for the same legacy message")
	}
}

func TestInboxDuplicateSuppressionPreservesDifferentAccountsAndNativeMessages(t *testing.T) {
	s, account, folder := newBodyFailedTestStore(t)
	if _, err := s.db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('other','Other','other@example.org','imap.example.org','smtp.example.org','other')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('other-inbox','other','Inbox','INBOX','inbox')`); err != nil {
		t.Fatal(err)
	}
	for i, scope := range []struct{ account, folder, thread string }{{account, folder, "gmail:a:1"}, {account, folder, "gmail:a:2"}, {"other", "other-inbox", "gmail:other:1"}} {
		m := &Message{AccountID: scope.account, FolderID: scope.folder, MessageID: "<same@example.org>", ThreadID: scope.thread, Date: time.Now()}
		if err := s.UpsertGmail(m, fmt.Sprintf("provider-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListConversationsByFolderIDs([]string{folder, "other-inbox"}, 0, 50, "newest", "")
	if err != nil || len(rows) != 3 {
		t.Fatalf("distinct native messages/accounts lost: %d %v", len(rows), err)
	}
}
