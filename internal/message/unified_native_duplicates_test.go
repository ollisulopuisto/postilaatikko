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

func TestGetConversationDeduplicatesCopiesSharingRFCMessageID(t *testing.T) {
	s, account, inboxFolder := newBodyFailedTestStore(t)
	// Create all-mail folder in same account
	allMailFolder := "all-mail-folder"
	if _, err := s.db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES(?,?,?,?,'all')`, allMailFolder, account, "All Mail", "[Gmail]/All Mail"); err != nil {
		t.Fatal(err)
	}
	sentFolder := "sent-folder"
	if _, err := s.db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES(?,?,?,?,'sent')`, sentFolder, account, "Sent", "[Gmail]/Sent Mail"); err != nil {
		t.Fatal(err)
	}

	threadID := "gmail:" + account + ":thread-123"
	now := time.Now().UTC()

	// 1. Incoming message copy in Inbox
	inboxCopy := &Message{
		ID:        "inbox-msg-1",
		AccountID: account,
		FolderID:  inboxFolder,
		UID:       101,
		MessageID: "<test-msg-1@example.com>",
		ThreadID:  threadID,
		Subject:   "Moro moro",
		FromName:  "Antti",
		FromEmail: "antti@example.com",
		Date:      now.Add(-time.Minute),
		BodyText:  "Tämä on testi viesti!",
		BodyFetched: true,
	}
	if err := s.Create(inboxCopy); err != nil {
		t.Fatal(err)
	}

	// 2. Incoming message copy in All Mail (same RFC Message-ID)
	allMailCopy := &Message{
		ID:        "allmail-msg-1",
		AccountID: account,
		FolderID:  allMailFolder,
		UID:       102,
		MessageID: "<test-msg-1@example.com>",
		ThreadID:  threadID,
		Subject:   "Moro moro",
		FromName:  "Antti",
		FromEmail: "antti@example.com",
		Date:      now.Add(-time.Minute),
		BodyText:  "Tämä on testi viesti!",
		BodyFetched: true,
	}
	if err := s.Create(allMailCopy); err != nil {
		t.Fatal(err)
	}

	// 3. Third copy (e.g. legacy/native duplicate or another label)
	thirdCopy := &Message{
		ID:        "third-msg-1",
		AccountID: account,
		FolderID:  allMailFolder,
		UID:       103,
		MessageID: "test-msg-1@example.com", // without brackets
		ThreadID:  threadID,
		Subject:   "Moro moro",
		FromName:  "Antti",
		FromEmail: "antti@example.com",
		Date:      now.Add(-time.Minute),
		BodyText:  "Tämä on testi viesti!",
		BodyFetched: false,
	}
	if err := s.Create(thirdCopy); err != nil {
		t.Fatal(err)
	}

	// 4. Outgoing reply in Sent folder
	replyMsg := &Message{
		ID:        "sent-msg-1",
		AccountID: account,
		FolderID:  sentFolder,
		UID:       104,
		MessageID: "<reply-msg-1@example.com>",
		InReplyTo: "<test-msg-1@example.com>",
		ThreadID:  threadID,
		Subject:   "Re: Moro moro",
		FromName:  "Me",
		FromEmail: "me@example.com",
		Date:      now,
		BodyText:  "Ja tässä vastaus",
		BodyFetched: true,
	}
	if err := s.Create(replyMsg); err != nil {
		t.Fatal(err)
	}

	conv, err := s.GetConversation(threadID, inboxFolder)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if conv == nil {
		t.Fatal("GetConversation returned nil")
	}

	if len(conv.Messages) != 2 {
		t.Fatalf("conv.Messages length = %d, want 2 (1 incoming + 1 reply). IDs: %+v", len(conv.Messages), func() []string {
			ids := make([]string, len(conv.Messages))
			for i, m := range conv.Messages {
				ids[i] = fmt.Sprintf("%s (%s, folder %s)", m.ID, m.MessageID, m.FolderID)
			}
			return ids
		}())
	}
	if conv.MessageCount != 2 {
		t.Fatalf("conv.MessageCount = %d, want 2", conv.MessageCount)
	}
	if conv.Messages[0].FolderID != inboxFolder {
		t.Errorf("dedup preferred folder %s, want inbox %s", conv.Messages[0].FolderID, inboxFolder)
	}
}
