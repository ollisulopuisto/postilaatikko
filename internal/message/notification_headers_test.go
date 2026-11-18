package message

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

func TestNotificationHeadersUseLocalReceiveTimeNotSenderDate(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (id,name,email,imap_host,smtp_host,username) VALUES ('a','A','me@example.com','imap.example.com','smtp.example.com','me')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"inbox", "other"} {
		if _, err := db.Exec(`INSERT INTO folders (id,account_id,name,path,folder_type) VALUES (?,'a',?,?,'inbox')`, id, id, id); err != nil {
			t.Fatal(err)
		}
	}
	s := NewStore(db)
	start := time.Now().UTC()
	rows := []*Message{
		{ID: "old-muted", AccountID: "a", FolderID: "inbox", UID: 1, FromEmail: "muted@example.com", Date: start.Add(time.Hour), ReceivedAt: start.Add(-time.Hour)},
		{ID: "new-important", ThreadID: "important-thread", Subject: "Important backdated arrival", AccountID: "a", FolderID: "inbox", UID: 2, FromEmail: "important@example.com", Date: start.Add(-24 * time.Hour), ReceivedAt: start.Add(time.Millisecond)},
		{ID: "elsewhere", AccountID: "a", FolderID: "other", UID: 1, FromEmail: "other@example.com", Date: start, ReceivedAt: start.Add(time.Millisecond)},
	}
	for _, m := range rows {
		if err := s.Create(m); err != nil {
			t.Fatal(err)
		}
	}
	ordinary, err := s.ListByFolder("inbox", 0, 1)
	if err != nil || len(ordinary) != 1 || ordinary[0].ID != "old-muted" {
		t.Fatalf("fixture should reproduce old date-sorted sample: %#v %v", ordinary, err)
	}
	recent, err := s.ListByFolderReceivedSince("inbox", start, 101)
	if err != nil || len(recent) != 1 || recent[0].ID != "new-important" {
		t.Fatalf("omitted the backdated arrival or leaked another folder: %#v %v", recent, err)
	}
	limited, err := s.ListByFolderReceivedSince("inbox", start.Add(-2*time.Hour), 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("limit ignored: %#v %v", limited, err)
	}
}

func TestNotificationTargetAlignsHeadersAndClickConversation(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "notification-target.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (id,name,email,imap_host,smtp_host,username) VALUES ('a','A','me@example.com','imap.example.com','smtp.example.com','me')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('inbox','a','Inbox','Inbox','inbox')`); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	for i, thread := range []string{"important-thread", ""} {
		id := fmt.Sprintf("message-%d", i)
		if err := s.Create(&Message{ID: id, AccountID: "a", FolderID: "inbox", UID: uint32(i + 1), ThreadID: thread, Subject: "Backdated important mail", FromName: "Important", FromEmail: "important@example.com", Date: time.Now().Add(-24 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
		target, err := s.NotificationTarget(id, "a", "inbox")
		if err != nil {
			t.Fatal(err)
		}
		want := thread
		if want == "" {
			want = id
		}
		if target.ThreadID != want || target.Subject != "Backdated important mail" || target.FromEmail != "important@example.com" {
			t.Fatalf("mismatched display/click target: %#v", target)
		}
		if _, err := s.NotificationTarget(id, "wrong-account", "inbox"); err == nil {
			t.Fatal("cross-account notification target accepted")
		}
		if _, err := s.NotificationTarget(id, "a", "wrong-folder"); err == nil {
			t.Fatal("wrong-folder notification target accepted")
		}
	}
}
