package app

import (
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/email"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/platform"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttachmentRepairDownloadUsesExactPartNotFilename(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "repair.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('a','A','a@example.test','imap','smtp','a')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('f','a','Inbox','Inbox','inbox')`); err != nil {
		t.Fatal(err)
	}
	ms := message.NewStore(db)
	if err = ms.Create(&message.Message{ID: "mail", AccountID: "a", FolderID: "f", UID: 1, ThreadID: "mail", Date: time.Now(), ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	store := message.NewAttachmentStore(db)
	parts := []message.RepairPart{{Metadata: message.Attachment{ID: "part1", MessageID: "mail", Filename: "same.png", ContentType: "image/png", Size: 3}, Payload: []byte("one")}, {Metadata: message.Attachment{ID: "part2", MessageID: "mail", Filename: "same.png", ContentType: "image/png", Size: 3}, Payload: []byte("two")}}
	if _, err = store.RepairFromMIME("a", "mail", "digest", parts, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE messages SET uid=-rowid WHERE id='mail'`); err != nil {
		t.Fatal(err)
	}
	app := &App{attachmentStore: store, messageStore: ms, paths: &platform.Paths{Data: t.TempDir()}}
	downloader := email.NewAttachmentDownloader(t.TempDir())
	for i, want := range []string{"one", "two"} {
		got, err := app.storedAttachmentContent(downloader, []byte("not even parseable MIME"), &parts[i].Metadata)
		if err != nil || string(got) != want {
			t.Fatalf("part %d: %q %v", i, got, err)
		}
	}
	// Single Save As and both bulk loops share this provider-free resolver.
	msg, err := ms.Get("mail")
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	for i, want := range []string{"one", "two"} {
		content, err := app.loadAttachmentContent(msg, &parts[i].Metadata, downloader, &raw)
		if err != nil || string(content) != want {
			t.Fatal("bulk repaired content tried provider", string(content), err)
		}
		path := filepath.Join(t.TempDir(), parts[i].Metadata.ID+".png")
		if _, err := app.DownloadAttachment(parts[i].Metadata.ID, path); err != nil {
			t.Fatal("single repaired Save As tried provider", err)
		}
		saved, err := os.ReadFile(path)
		if err != nil || string(saved) != want {
			t.Fatal("saved wrong repaired occurrence", string(saved), err)
		}
	}
	app.accountStore = account.NewStore(db)
	if _, err := db.Exec(`UPDATE messages SET message_id='same@test',body_fetched=1,body_text='offline original' WHERE id='mail'`); err != nil {
		t.Fatal(err)
	}
	if err := ms.Create(&message.Message{ID: "live", AccountID: "a", FolderID: "f", UID: 8, MessageID: "same@test", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	forward, err := app.PrepareReply("live", "forward")
	if err != nil || len(forward.Attachments) != 2 || string(forward.Attachments[0].Content) != "one" || string(forward.Attachments[1].Content) != "two" {
		t.Fatal("offline forward lost exact repaired occurrences", forward, err)
	}
	if _, err = app.RepairMessageAttachments("a", "mail"); err == nil {
		t.Fatal("repair accepted missing live owner engine")
	}
}
