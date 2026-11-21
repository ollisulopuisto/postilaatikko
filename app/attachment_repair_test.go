package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/email"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/platform"
	"net/http"
	"net/http/httptest"
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

func TestGmailCacheKeyRawFallbackAndBatchSaveResult(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.app.paths = &platform.Paths{Data: t.TempDir()}
	f.app.attachmentStore = message.NewAttachmentStore(f.db)
	if _, err := f.db.Exec(`UPDATE accounts SET imap_host='imap.gmail.com',auth_type='oauth2' WHERE id='acct-a'`); err != nil {
		t.Fatal(err)
	}
	msg := &message.Message{AccountID: "acct-a", FolderID: "a-inbox", MessageID: "raw@test", Date: time.Now()}
	if err := f.app.messageStore.UpsertGmail(msg, "provider"); err != nil {
		t.Fatal(err)
	}
	att := &message.Attachment{ID: "no-part-reference", MessageID: msg.ID, Filename: "notes.txt"}
	if err := f.app.attachmentStore.Create(att); err != nil {
		t.Fatal(err)
	}
	raw := "MIME-Version: 1.0\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=notes.txt\r\n\r\nGmail raw bytes"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/messages/provider" || r.URL.Query().Get("format") != "raw" {
			t.Error("wrong Gmail fallback", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"raw": base64.RawURLEncoding.EncodeToString([]byte(raw))})
	}))
	defer server.Close()
	f.app.syncEngine.SetGmailTokenSource(func(context.Context, string, bool) (string, error) { return "fixture", nil })
	f.app.syncEngine.SetGmailClientFactory(func(src gmail.TokenSource) *gmail.Client { c := gmail.NewClient(src); c.BaseURL = server.URL; return c })
	var cached []byte
	got, err := f.app.loadAttachmentContent(msg, att, email.NewAttachmentDownloader(""), &cached)
	if err != nil || string(got) != "Gmail raw bytes" || calls != 1 {
		t.Fatal("live Gmail raw fallback rejected", string(got), calls, err)
	}
	engine := f.app.syncEngine
	// Both native and portal adapters share this batch result boundary.
	for _, portal := range []bool{false, true} {
		dir := t.TempDir()
		var paths []string
		if portal {
			paths = []string{filepath.Join(dir, "notes.txt"), filepath.Join(dir, "missing.txt")}
		}
		missing := &message.Attachment{ID: "missing", MessageID: msg.ID, Filename: "missing.txt"}
		folder, err := f.app.saveAttachmentBatch(msg, []*message.Attachment{att, missing}, dir, paths)
		if err == nil || folder != "" {
			t.Fatal("partial Save All reported success", portal, folder, err)
		}
		if bytes, err := os.ReadFile(filepath.Join(dir, "notes.txt")); err != nil || string(bytes) != "Gmail raw bytes" {
			t.Fatal("partial save discarded completed file", string(bytes), err)
		}
		f.app.syncEngine = nil
		folder, err = f.app.saveAttachmentBatch(msg, []*message.Attachment{missing}, t.TempDir(), nil)
		if err == nil || folder != "" {
			t.Fatal("zero Save All reported success", folder, err)
		}
		// Restore the provider for the portal iteration.
		f.app.syncEngine = engine
	}
}

func TestIMAPAttachmentFallbackRejectsSameNameParts(t *testing.T) {
	f := newFilterFixture(t, false)
	f.app.ctx = context.Background()
	f.app.attachmentStore = message.NewAttachmentStore(f.db)
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=same.txt\r\n\r\none\r\n--b\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=same.txt\r\n\r\ntwo\r\n--b--\r\n"
	f.serverA.sess.messages[f.message.UID] = []byte(raw)
	if _, err := f.db.Exec(`UPDATE folders SET uid_validity=1 WHERE id='a-inbox'`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		att := &message.Attachment{ID: id, MessageID: f.message.ID, Filename: "same.txt"}
		if err := f.app.attachmentStore.Create(att); err != nil {
			t.Fatal(err)
		}
		var cached []byte
		if got, err := f.app.loadAttachmentContent(f.message, att, email.NewAttachmentDownloader(""), &cached); err == nil {
			t.Fatal("IMAP ambiguity returned wrong occurrence", id, string(got))
		}
	}
}
