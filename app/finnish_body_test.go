package app

import (
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedFinnishBodyRepairDoesNotModifyStoredOriginal(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('a','a','a@b','imap','smtp','a')`, `INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('f','a','Inbox','INBOX','inbox')`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	store := message.NewStore(db)
	body := "HyvÃ¤ pÃ¤ivÃ¤, nÃ¤hdÃ¤Ã¤n!"
	m := &message.Message{ID: "m", AccountID: "a", FolderID: "f", UID: 1, MessageID: "<m>", ThreadID: "thread", BodyFetched: true, BodyText: body, Date: time.Now()}
	if err = store.Create(m); err != nil {
		t.Fatal(err)
	}
	a := &App{folderStore: folder.NewStore(db), messageStore: store}
	conv, err := a.GetConversation("thread", "f")
	if err != nil || conv == nil || len(conv.Messages) != 1 || conv.Messages[0].BodyText != "Hyvä päivä, nähdään!" {
		t.Fatalf("display %#v %v", conv, err)
	}
	original, err := store.Get("m")
	if err != nil || original.BodyText != body {
		t.Fatalf("stored original modified: %#v %v", original, err)
	}
}
