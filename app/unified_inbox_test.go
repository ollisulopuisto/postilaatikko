package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
)

func TestUnifiedInboxFallbackListAndCountsUseResolvedFolderIDs(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "unified-inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES
		 ('enabled','Enabled','enabled@example.com','imap','smtp','enabled',1),
		 ('disabled','Disabled','disabled@example.com','imap','smtp','disabled',0)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type,unread_count) VALUES
		 ('inbox-a','enabled','Inbox A','INBOX-A','inbox',3),
		 ('inbox-b','enabled','Inbox B','INBOX-B','inbox',5),
		 ('disabled-inbox','disabled','Inbox','INBOX','inbox',7)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	accounts := account.NewStore(db)
	folders := folder.NewStore(db)
	messages := message.NewStore(db)
	a := &App{accountStore: accounts, folderStore: folders, messageStore: messages}
	selected, err := a.unifiedSpecialFolderIDs(string(folder.TypeInbox))
	if err != nil || len(selected) != 1 {
		t.Fatalf("resolved inbox IDs = %v, err=%v; want one enabled inbox", selected, err)
	}
	other := "inbox-a"
	if selected[0] == other {
		other = "inbox-b"
	}
	for _, item := range []struct{ id, accountID, folderID string }{
		{id: "selected-message", accountID: "enabled", folderID: selected[0]},
		{id: "other-inbox-message", accountID: "enabled", folderID: other},
		{id: "disabled-message", accountID: "disabled", folderID: "disabled-inbox"},
	} {
		if err := messages.Create(&message.Message{ID: item.id, AccountID: item.accountID, FolderID: item.folderID, UID: 1, ThreadID: item.id, Subject: item.id, Date: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	conversations, err := a.GetUnifiedInboxConversations(0, 20, "newest", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].FolderID != selected[0] {
		t.Fatalf("fallback conversations = %#v, want only folder %q", conversations, selected[0])
	}
	count, err := a.GetUnifiedInboxCount("")
	if err != nil || count != 1 {
		t.Fatalf("unified inbox count = %d, err=%v; want 1", count, err)
	}
	viewCount, err := a.GetUnifiedFolderCount(string(folder.TypeInbox), "")
	if err != nil || viewCount != count {
		t.Fatalf("All Inboxes view count = %d, err=%v; explicit count = %d", viewCount, err, count)
	}
	unread, err := a.GetUnifiedInboxUnreadCount()
	if err != nil {
		t.Fatal(err)
	}
	wantUnread := 3
	if selected[0] == "inbox-b" {
		wantUnread = 5
	}
	if unread != wantUnread {
		t.Fatalf("unified inbox unread count = %d, want selected folder count %d", unread, wantUnread)
	}
}
