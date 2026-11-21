package alwayson

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/draft"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/smtp"
)

func replyModeRuntime(t *testing.T) *SyncRuntime {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "reply-mode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES ('a','A','me@example.com','imap','smtp','a')`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES ('inbox','a','Inbox','INBOX','inbox')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return &SyncRuntime{db: db, accounts: account.NewStore(db), messages: message.NewStore(db), drafts: draft.NewStore(db)}
}

// A Reply All draft whose recipients were edited down to one person must keep
// its mode through the helper create, update, read and list paths, so the
// composer and undo bar still call it Reply All.
func TestHelperDraftKeepsReplyModeWhenRecipientsShrink(t *testing.T) {
	r := replyModeRuntime(t)
	from := smtp.Address{Address: "me@example.com"}
	created, err := r.SaveDraft("", v1.DraftMutation{AccountID: "a", Message: smtp.ComposeMessage{
		From: from, ReplyMode: "reply-all", Subject: "Re: x",
		To: []smtp.Address{{Address: "one@example.com"}, {Address: "two@example.com"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Message.ReplyMode != "reply-all" {
		t.Fatalf("create lost reply mode: %q", created.Message.ReplyMode)
	}
	updated, err := r.SaveDraft(created.ID, v1.DraftMutation{AccountID: "a", Revision: created.Revision, Message: smtp.ComposeMessage{
		From: from, ReplyMode: "reply-all", Subject: "Re: x", To: []smtp.Address{{Address: "one@example.com"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Message.ReplyMode != "reply-all" || len(updated.Message.To) != 1 {
		t.Fatalf("update: mode %q to %d", updated.Message.ReplyMode, len(updated.Message.To))
	}
	got, err := r.GetDraft(created.ID, "a")
	if err != nil || got == nil || got.Message.ReplyMode != "reply-all" {
		t.Fatalf("read path lost reply mode: %+v %v", got, err)
	}
	list, err := r.ListDrafts("a")
	if err != nil || len(list) != 1 || list[0].Message.ReplyMode != "reply-all" {
		t.Fatalf("list path lost reply mode: %+v %v", list, err)
	}
}

func TestHelperPrepareReplyReportsMode(t *testing.T) {
	r := replyModeRuntime(t)
	if err := r.messages.Create(&message.Message{ID: "m1", AccountID: "a", FolderID: "inbox", UID: 1, FromEmail: "s@example.com",
		ToList: `[{"name":"","email":"me@example.com"},{"name":"","email":"x@example.com"}]`, CcList: "[]",
		Subject: "hi", BodyText: "hello", BodyFetched: true, Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"reply", "reply-all", "forward"} {
		got, err := r.PrepareReply(v1.ReplyPreparationRequest{AccountID: "a", MessageID: "m1", Mode: mode})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if got.ReplyMode != mode {
			t.Fatalf("PrepareReply(%s) ReplyMode = %q", mode, got.ReplyMode)
		}
	}
}

func TestHelperPrepareReplyUsesOfflineSnapshot(t *testing.T) {
	r := replyModeRuntime(t)
	var negative int32 = -1
	for _, msg := range []*message.Message{
		{ID: "cached", AccountID: "a", FolderID: "inbox", UID: uint32(negative), MessageID: "same@test", BodyFetched: true, BodyText: "offline quote", Date: time.Now()},
		{ID: "live", AccountID: "a", FolderID: "inbox", UID: 8, MessageID: "same@test", ToList: "[]", CcList: "[]", Date: time.Now()},
	} {
		if err := r.messages.Create(msg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.Exec(`UPDATE messages SET uid=-1 WHERE id='cached'`); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"reply", "reply-all", "forward"} {
		got, err := r.PrepareReply(v1.ReplyPreparationRequest{AccountID: "a", MessageID: "live", Mode: mode})
		if err != nil || !strings.Contains(got.TextBody, "offline quote") {
			t.Fatal("helper lost offline quote", mode, got, err)
		}
	}
}

func TestHelperForwardRejectsMissingSnapshotFiles(t *testing.T) {
	r := replyModeRuntime(t)
	r.attachments = message.NewAttachmentStore(r.db)
	for _, msg := range []*message.Message{{ID: "cached", AccountID: "a", FolderID: "inbox", UID: 7, MessageID: "same@test", BodyFetched: true, BodyText: "offline quote", Date: time.Now()}, {ID: "live", AccountID: "a", FolderID: "inbox", UID: 8, MessageID: "same@test", ToList: "[]", CcList: "[]", Date: time.Now()}} {
		if err := r.messages.Create(msg); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.Exec(`UPDATE messages SET uid=-1 WHERE id='cached'`); err != nil {
		t.Fatal(err)
	}
	if err := r.attachments.Create(&message.Attachment{ID: "missing", MessageID: "cached", Filename: "file.txt"}); err != nil {
		t.Fatal(err)
	}
	if got, err := r.PrepareReply(v1.ReplyPreparationRequest{AccountID: "a", MessageID: "live", Mode: "forward"}); err == nil || got != nil {
		t.Fatal("forward silently omitted original file", got, err)
	}
}
