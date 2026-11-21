package main

import (
	"fmt"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRollbackCommandRefusesMissingOrWrongDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typo.db")
	if err := run([]string{"--db", path, "--account", "a", "--message", "m"}); err == nil {
		t.Fatal("created missing database")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("typo created a file")
	}
	if err := run(nil); err == nil {
		t.Fatal("missing ownership accepted")
	}
}

func TestRollbackCommandRestoresBackedUpCache(t *testing.T) {
	for _, version := range []int{79, 85, 86} {
		t.Run(fmt.Sprint(version), func(t *testing.T) { testRollbackSchema(t, version) })
	}
}

func testRollbackSchema(t *testing.T, version int) {
	path := filepath.Join(t.TempDir(), "mail.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('a','A','a@example.test','imap','smtp','a')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('f','a','Inbox','Inbox','inbox')`); err != nil {
		t.Fatal(err)
	}
	if err = message.NewStore(db).Create(&message.Message{ID: "mail", AccountID: "a", FolderID: "f", UID: 1, ThreadID: "mail", Date: time.Now(), ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	store := message.NewAttachmentStore(db)
	if err = store.Create(&message.Attachment{ID: "legacy", MessageID: "mail", Filename: "old", Size: 3}); err != nil {
		t.Fatal(err)
	}
	backup, err := store.RepairFromMIME("a", "mail", "digest", []message.RepairPart{{Metadata: message.Attachment{ID: "new", MessageID: "mail", Filename: "new", Size: 3}, Payload: []byte("one")}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if version < 86 {
		// Reproduce the pre-integrity schema accepted by the offline command.
		if _, err = db.Exec(`DROP TABLE attachment_download_integrity`); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`DELETE FROM migrations WHERE version>?`, version); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{"--db", path, "--account", "a", "--message", "mail"}
	if err = run(args); err != nil {
		t.Fatal(err)
	}
	if err = run(append(args, "--restore", backup)); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := message.NewAttachmentStore(db).GetByMessage("mail")
	if err != nil || len(got) != 1 || got[0].ID != "legacy" {
		t.Fatalf("CLI restore lost original: %v %v", got, err)
	}
	next, err := message.NewAttachmentStore(db).RepairBackupID("mail")
	if err != nil || next == "" || next == backup {
		t.Fatal("missing reversible generation", next, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = run(append(args, "--restore", next)); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err = message.NewAttachmentStore(db).GetByMessage("mail")
	if err != nil || len(got) != 1 || got[0].ID != "new" {
		t.Fatal("redo lost repaired state", got, err)
	}
	if version < 86 {
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='attachment_download_integrity'`).Scan(&count); err != nil || count != 0 {
			t.Fatal("offline rollback changed schema", count, err)
		}
	}
}
