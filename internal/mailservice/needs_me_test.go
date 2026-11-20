package mailservice

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/replylater"
	mailSync "github.com/hkdb/aerion/internal/sync"
	"github.com/hkdb/aerion/internal/triage"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

func TestNeedsMeProviderFilterHonorsSnoozeDeadline(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES ('a','A','a@example.org','imap.example.org','smtp.example.org','a',1)`); err != nil {
		t.Fatal(err)
	}
	fs := folder.NewStore(db)
	if err = fs.Create(&folder.Folder{ID: "retained", AccountID: "a", Name: "Retained", Path: "Retained", Type: folder.TypeFolder}); err != nil {
		t.Fatal(err)
	}
	if err = fs.SetHiddenBatch("a", []string{"retained"}, true); err != nil {
		t.Fatal(err)
	}
	ms := message.NewStore(db)
	ts := triage.NewStore(db)
	rs := replylater.NewStore(db)
	for i, id := range []string{"unread", "later", "expired", "awake"} {
		if err = ms.Create(&message.Message{ID: id, ThreadID: id, AccountID: "a", FolderID: "retained", UID: uint32(i + 1), Date: time.Now(), IsRead: id == "later"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = ts.SetReadLater(triage.Ref{AccountID: "a", ConversationID: "later"}, true); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"unread", "later", "expired"} {
		if err = ts.Snooze(triage.Ref{AccountID: "a", ConversationID: id}, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`UPDATE snoozed_conversations SET wake_at=? WHERE conversation_id='expired'`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// This unrelated account must not suppress the same conversation in account a.
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES ('other','Other','other@example.org','imap.example.org','smtp.example.org','other',0)`); err != nil {
		t.Fatal(err)
	}
	if err = ts.Snooze(triage.Ref{AccountID: "other", ConversationID: "awake"}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	vs := virtualfolder.NewStore(db)
	if err = vs.Create(&virtualfolder.VirtualFolder{ID: "v", Name: "Retained", FolderIDs: []string{"retained"}}); err != nil {
		t.Fatal(err)
	}
	as := account.NewStore(db)
	svc := New(Options{Accounts: as, Folders: fs, Messages: ms, Triage: ts, ReplyLater: rs, VirtualFolders: vs, Sync: mailSync.NewEngine(nil, as, fs, ms, message.NewAttachmentStore(db))})
	page, err := svc.Search(context.Background(), MailSearchRequest{VirtualFolderID: "v", Filter: "needsMe", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range page.Conversations {
		got[c.ThreadID] = true
	}
	if len(got) != 2 || !got["awake"] || !got["expired"] {
		t.Fatalf("Needs me returned snoozed or lost awake conversations: %v", got)
	}
	// Other search modes retain their existing behavior.
	page, err = svc.Search(context.Background(), MailSearchRequest{VirtualFolderID: "v", Filter: "unread", Limit: 100})
	if err != nil || len(page.Conversations) != 3 {
		t.Fatalf("unread behavior changed: %#v %v", page, err)
	}
}
