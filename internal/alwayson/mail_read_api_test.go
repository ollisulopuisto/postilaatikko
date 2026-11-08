package alwayson

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
)

func setupThreadReadTest(t *testing.T) (*mailboxReadOperations, *database.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "thread-read.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		_ = db.Close()
		t.Fatalf("db.Migrate: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO accounts (id, name, email, imap_host, smtp_host, username)
		VALUES ('acc-1', 'Test', 'test@example.com', 'imap.example.com', 'smtp.example.com', 'test')
	`); err != nil {
		_ = db.Close()
		t.Fatalf("seed account: %v", err)
	}
	for _, values := range []struct{ id, name, path, kind string }{
		{id: "inbox", name: "Inbox", path: "INBOX", kind: "inbox"},
		{id: "sent", name: "Sent", path: "Sent", kind: "sent"},
	} {
		if _, err := db.Exec(`INSERT INTO folders (id, account_id, name, path, folder_type) VALUES (?, 'acc-1', ?, ?, ?)`, values.id, values.name, values.path, values.kind); err != nil {
			_ = db.Close()
			t.Fatalf("seed folder %s: %v", values.id, err)
		}
	}

	messages := message.NewStore(db)
	attachments := message.NewAttachmentStore(db)
	date := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for _, item := range []message.Message{
		{ID: "msg-inbox", AccountID: "acc-1", FolderID: "inbox", UID: 1, ThreadID: "thread-current", Subject: "Visible", BodyText: "PRIVATE BODY TEXT", BodyHTML: "<p>PRIVATE BODY HTML</p>", BodyFetched: true, Date: date},
		{ID: "msg-sent", AccountID: "acc-1", FolderID: "sent", UID: 2, ThreadID: "thread-current", Subject: "Context", BodyText: "PRIVATE CONTEXT BODY", BodyFetched: true, Date: date.Add(time.Minute)},
	} {
		if err := messages.Create(&item); err != nil {
			_ = db.Close()
			t.Fatalf("create message %s: %v", item.ID, err)
		}
	}
	if err := attachments.Create(&message.Attachment{
		ID: "attachment-1", MessageID: "msg-inbox", Filename: "secret.txt", ContentType: "text/plain",
		Size: 7, LocalPath: "/private/attachment-cache/secret.txt", IsInline: true, Content: []byte("BYTES!!"),
	}); err != nil {
		_ = db.Close()
		t.Fatalf("create attachment: %v", err)
	}

	return &mailboxReadOperations{runtime: &SyncRuntime{messages: messages, attachments: attachments}}, db
}

func TestGetThreadCanonicalizesStaleMessageReferenceAndKeepsScope(t *testing.T) {
	operations, db := setupThreadReadTest(t)
	defer db.Close()

	view, err := operations.GetThread(context.Background(), views.ThreadRef{AccountID: "acc-1", ThreadID: "msg-inbox"}, views.Scope{
		Kind: views.ScopeFolder, AccountID: "acc-1", FolderID: "inbox",
	})
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if view.Ref.ThreadID != "thread-current" {
		t.Fatalf("thread ref = %q, want canonical thread-current", view.Ref.ThreadID)
	}
	if view.MatchedCount != 1 || len(view.Messages) != 2 {
		t.Fatalf("matched count/messages = %d/%d, want 1/2", view.MatchedCount, len(view.Messages))
	}
	if !view.Messages[0].Matched || view.Messages[1].Matched {
		t.Fatalf("matched scope = [%t %t], want [true false]", view.Messages[0].Matched, view.Messages[1].Matched)
	}
}

func TestGetThreadResponseIncludesAttachmentSummaryButNoPrivateBodyOrPath(t *testing.T) {
	operations, db := setupThreadReadTest(t)
	defer db.Close()

	view, err := operations.GetThread(context.Background(), views.ThreadRef{AccountID: "acc-1", ThreadID: "thread-current"}, views.Scope{
		Kind: views.ScopeFolder, AccountID: "acc-1", FolderID: "inbox",
	})
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(view.Messages) == 0 || len(view.Messages[0].Attachments) != 1 {
		t.Fatalf("thread response missing attachment summary: %#v", view.Messages)
	}
	attachment := view.Messages[0].Attachments[0]
	if attachment.ID != "attachment-1" || attachment.Filename != "secret.txt" || attachment.Size != 7 {
		t.Fatalf("unexpected attachment summary: %#v", attachment)
	}

	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal thread response: %v", err)
	}
	for _, privateValue := range []string{"PRIVATE BODY TEXT", "PRIVATE BODY HTML", "PRIVATE CONTEXT BODY", "/private/attachment-cache/secret.txt", "BYTES!!"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("thread response leaked %q: %s", privateValue, encoded)
		}
	}
}
