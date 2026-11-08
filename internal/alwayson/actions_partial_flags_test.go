package alwayson

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
)

func TestSetFlagsPersistsProviderConfirmedAccountsBeforeLaterFailure(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "partial-flags.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES
		 ('a','A','a@example.com','imap','smtp','a',1),('b','B','b@example.com','imap','smtp','b',1)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES
		 ('fa','a','Inbox','INBOX','inbox'),('fb','b','Inbox','INBOX','inbox')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	messages := message.NewStore(db)
	for _, item := range []message.Message{
		{ID: "ma", AccountID: "a", FolderID: "fa", UID: 1, ThreadID: "ma", Date: time.Now()},
		{ID: "mb", AccountID: "b", FolderID: "fb", UID: 2, ThreadID: "mb", Date: time.Now()},
	} {
		if err := messages.Create(&item); err != nil {
			t.Fatal(err)
		}
	}

	operations := &mailboxActionOperations{
		runtime: &SyncRuntime{messages: messages, folders: folder.NewStore(db)},
		gmailClientFunc: func(string) (*gmail.Client, bool, error) {
			return nil, false, nil
		},
		setIMAPFlagsFunc: func(accountID, _ string, _ []*message.Message, _ string, _ bool) error {
			if accountID == "b" {
				return errors.New("provider unavailable")
			}
			return nil
		},
	}
	err = operations.MarkAsRead([]string{"ma", "mb"})
	if err == nil {
		t.Fatal("MarkAsRead succeeded despite the second account failure")
	}
	updated, err := messages.GetByIDs([]string{"ma", "mb"})
	if err != nil {
		t.Fatal(err)
	}
	readByID := map[string]bool{}
	for _, item := range updated {
		readByID[item.ID] = item.IsRead
	}
	if !readByID["ma"] || readByID["mb"] {
		t.Fatalf("persisted read state = %v, want confirmed account a only", readByID)
	}
}
