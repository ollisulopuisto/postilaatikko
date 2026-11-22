package alwayson

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/mailservice"
	mailSync "github.com/ollisulopuisto/postilaatikko/internal/sync"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/views"
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

func TestGetThreadCountsAndCompletenessUseRetainedCopies(t *testing.T) {
	operations, db := setupThreadReadTest(t)
	defer db.Close()
	// The service path must prefer the same in-scope cached copy as the viewer.
	if _, err := db.Exec(`UPDATE messages SET message_id='<same@example.org>' WHERE id='msg-inbox'`); err != nil {
		t.Fatal(err)
	}
	for _, copy := range []message.Message{
		{ID: "duplicate-inbox-empty", AccountID: "acc-1", FolderID: "inbox", UID: 3, ThreadID: "thread-current", MessageID: "same@example.org", Date: time.Now()},
		{ID: "duplicate-context-empty", AccountID: "acc-1", FolderID: "sent", UID: 4, ThreadID: "thread-current", MessageID: "<same@example.org>", Date: time.Now()},
	} {
		if err := operations.runtime.messages.Create(&copy); err != nil {
			t.Fatal(err)
		}
	}
	view, err := operations.GetThread(context.Background(), views.ThreadRef{AccountID: "acc-1", ThreadID: "thread-current"}, views.Scope{
		Kind: views.ScopeFolder, AccountID: "acc-1", FolderID: "inbox",
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.MessageCount != 2 || len(view.Messages) != 2 || view.MatchedCount != 1 {
		t.Fatalf("hidden copies inflated counts: count=%d messages=%d matched=%d", view.MessageCount, len(view.Messages), view.MatchedCount)
	}
	if view.Messages[0].Ref.MessageID != "msg-inbox" || view.Messages[0].ContentState != views.ContentCached {
		t.Fatalf("lost selected cached copy: %+v", view.Messages[0])
	}
	if view.Completeness != views.CompletenessOf(view.Messages) {
		t.Fatalf("completeness does not describe retained messages: %+v", view.Completeness)
	}
}

func TestThreadThenCachedBodyReadsSnapshotWithoutProvider(t *testing.T) {
	ops, db := setupThreadReadTest(t)
	defer db.Close()
	parts := []message.RepairPart{
		{Metadata: message.Attachment{ID: "exact-one", MessageID: "msg-inbox", Filename: "same.txt", ContentType: "text/plain", Size: 3}, Payload: []byte("one")},
		{Metadata: message.Attachment{ID: "exact-two", MessageID: "msg-inbox", Filename: "same.txt", ContentType: "text/plain", Size: 3}, Payload: []byte("two")},
	}
	if _, err := ops.runtime.attachments.RepairFromMIME("acc-1", "msg-inbox", "fixture", parts, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE messages SET message_id='same@test',uid=-1 WHERE id='msg-inbox'`); err != nil {
		t.Fatal(err)
	}
	if err := ops.runtime.messages.Create(&message.Message{ID: "live", AccountID: "acc-1", FolderID: "inbox", UID: 8, MessageID: "same@test", ThreadID: "thread-current", Date: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ops.runtime.mailService = mailservice.New(mailservice.Options{Messages: ops.runtime.messages}) // No provider exists.
	view, err := ops.GetThread(context.Background(), views.ThreadRef{AccountID: "acc-1", ThreadID: "thread-current"}, views.Scope{Kind: views.ScopeFolder, AccountID: "acc-1", FolderID: "inbox"})
	if err != nil {
		t.Fatal(err)
	}
	var selected *views.ThreadMessage
	for i := range view.Messages {
		if view.Messages[i].Ref.MessageID == "live" {
			selected = &view.Messages[i]
		}
	}
	if selected == nil || selected.ContentState != views.ContentCached || len(selected.Attachments) != 2 {
		t.Fatal("thread lost cached source", selected)
	}
	mux := http.NewServeMux()
	v1.MountThreadBodyRoutes(mux, ops, func(http.ResponseWriter, *http.Request, v1.Capability) bool { return true })
	v1.MountAttachmentRoutes(mux, ops, ops, func(http.ResponseWriter, *http.Request, v1.Capability) bool { return true })
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/messages/body", strings.NewReader(`{"ref":{"accountId":"acc-1","messageId":"live"},"mode":"cached","includeOriginal":true}`)))
	var body v1.Response[v1.MessageBody]
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || body.Data.ContentState != views.ContentCached || body.Data.Original == nil || body.Data.Original.Text != "PRIVATE BODY TEXT" || body.Data.Ref.MessageID != "live" {
		t.Fatal("cached-only body disagrees with thread", rec.Code, rec.Body.String())
	}
	for _, part := range parts {
		rec := httptest.NewRecorder()
		body := `{"ref":{"accountId":"acc-1","messageId":"live"},"attachmentId":"` + part.Metadata.ID + `"}`
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/messages/attachments/read", strings.NewReader(body)))
		if rec.Code != 200 || rec.Body.String() != string(part.Payload) {
			t.Fatal("snapshot byte route lost exact occurrence", rec.Code, rec.Body.String())
		}
		if _, _, err := ops.ReadAttachmentBytes("wrong", "msg-inbox", part.Metadata.ID); err == nil {
			t.Fatal("cross-account attachment read allowed")
		}
		if _, _, err := ops.ReadAttachmentBytes("acc-1", "live", part.Metadata.ID); err == nil {
			t.Fatal("cross-message attachment read allowed")
		}
	}
	ops.runtime.accounts = account.NewStore(db)
	forward, err := ops.runtime.PrepareReply(v1.ReplyPreparationRequest{AccountID: "acc-1", MessageID: "live", Mode: "forward"})
	if err != nil || len(forward.Attachments) != 2 || string(forward.Attachments[0].Content) != "one" || string(forward.Attachments[1].Content) != "two" {
		t.Fatal("helper forward lost cached exact parts", forward, err)
	}
	production := http.NewServeMux()
	mountMailboxV1Routes(production, &mailboxRouteDeps{messageRead: ops, attachmentRead: ops})
	denied := httptest.NewRecorder()
	production.ServeHTTP(denied, httptest.NewRequest("POST", "/v1/messages/attachments/read", strings.NewReader(`{}`)))
	if denied.Code != http.StatusUnauthorized {
		t.Fatal("production attachment route missing or unguarded", denied.Code)
	}
	raw, err := ops.runtime.messages.Get("live")
	if err != nil || raw.BodyFetched || raw.BodyText != "" {
		t.Fatal("reader persisted overlay", raw, err)
	}
}

func TestHelperGmailRawFallbackKeepsProviderIdentityAndRejectsAmbiguity(t *testing.T) {
	ops, db := setupThreadReadTest(t)
	defer db.Close()
	if _, err := db.Exec(`UPDATE accounts SET imap_host='imap.gmail.com',auth_type='oauth2' WHERE id='acc-1'`); err != nil {
		t.Fatal(err)
	}
	msg := &message.Message{AccountID: "acc-1", FolderID: "inbox", Date: time.Now()}
	if err := ops.runtime.messages.UpsertGmail(msg, "provider"); err != nil {
		t.Fatal(err)
	}
	att := &message.Attachment{ID: "raw-part", MessageID: msg.ID, Filename: "same.txt"}
	if err := ops.runtime.attachments.Create(att); err != nil {
		t.Fatal(err)
	}
	raw := "MIME-Version: 1.0\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=same.txt\r\n\r\none"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages/provider" || r.URL.Query().Get("format") != "raw" {
			t.Error("wrong raw fallback", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"raw": base64.RawURLEncoding.EncodeToString([]byte(raw))})
	}))
	defer server.Close()
	e := mailSync.NewEngine(nil, account.NewStore(db), folder.NewStore(db), ops.runtime.messages, ops.runtime.attachments)
	e.SetGmailTokenSource(func(context.Context, string, bool) (string, error) { return "fixture", nil })
	e.SetGmailClientFactory(func(src gmail.TokenSource) *gmail.Client { c := gmail.NewClient(src); c.BaseURL = server.URL; return c })
	ops.runtime.syncEngine = e
	got, _, err := ops.ReadAttachmentBytes("acc-1", msg.ID, att.ID)
	if err != nil || string(got) != "one" {
		t.Fatal("helper rejected Gmail raw cache key", string(got), err)
	}
	raw = "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=same.txt\r\n\r\none\r\n--b\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=same.txt\r\n\r\ntwo\r\n--b--\r\n"
	if got, _, err := ops.ReadAttachmentBytes("acc-1", msg.ID, att.ID); err == nil {
		t.Fatal("helper returned first of ambiguous payloads", string(got))
	}
	if _, err := db.Exec(`DELETE FROM gmail_message_refs WHERE message_id=?`, msg.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ops.ReadAttachmentBytes("acc-1", msg.ID, att.ID); err == nil {
		t.Fatal("unbound snapshot reached provider")
	}
}

func TestDeletedGmailAttachmentIsPermanentNotFound(t *testing.T) {
	ops, db := setupThreadReadTest(t)
	defer db.Close()
	if _, err := db.Exec(`UPDATE accounts SET imap_host='imap.gmail.com',auth_type='oauth2' WHERE id='acc-1'`); err != nil {
		t.Fatal(err)
	}
	if err := ops.runtime.attachments.Create(&message.Attachment{ID: "gone", MessageID: "msg-inbox", Filename: "file.pdf", Size: 4}); err != nil {
		t.Fatal(err)
	}
	if err := ops.runtime.attachments.SetGmailReference("gone", "provider", "gone-file"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"not found"}}`))
	}))
	defer server.Close()
	e := mailSync.NewEngine(nil, account.NewStore(db), folder.NewStore(db), ops.runtime.messages, ops.runtime.attachments)
	e.SetGmailTokenSource(func(context.Context, string, bool) (string, error) { return "fixture", nil })
	e.SetGmailClientFactory(func(src gmail.TokenSource) *gmail.Client { c := gmail.NewClient(src); c.BaseURL = server.URL; return c })
	ops.runtime.syncEngine = e
	_, _, err := ops.ReadAttachmentBytes("acc-1", "msg-inbox", "gone")
	var fault *v1.Fault
	if !errors.As(err, &fault) || fault.Code != v1.CodeNotFound || fault.Retryable {
		t.Fatal("deleted file treated as outage", err)
	}
}

func TestHelperDownloadCacheRequiresOriginalIntegrity(t *testing.T) {
	ops, db := setupThreadReadTest(t)
	defer db.Close()
	root := t.TempDir()
	ops.runtime.attachmentCacheRoot = root
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	att := &message.Attachment{ID: "downloaded", MessageID: "msg-inbox", Filename: "file.txt", ContentType: "text/plain", Size: 8, LocalPath: path}
	if err := ops.runtime.attachments.Create(att); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ops.ReadAttachmentBytes("acc-1", "msg-inbox", att.ID); err == nil {
		t.Fatal("legacy unverified download accepted")
	}
	if err := ops.runtime.attachments.SetVerifiedDownload(att.ID, path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	got, _, err := ops.ReadAttachmentBytes("acc-1", "msg-inbox", att.ID)
	if err != nil || string(got) != "original" {
		t.Fatal(string(got), err)
	}
	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, _, err := ops.ReadAttachmentBytes("acc-1", "msg-inbox", att.ID); err == nil {
		t.Fatal("edited helper download accepted", string(got))
	}
}
