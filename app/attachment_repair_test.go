package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/email"
	"github.com/ollisulopuisto/postilaatikko/internal/gmail"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/platform"
	"github.com/ollisulopuisto/postilaatikko/internal/smtp"
	"github.com/rs/zerolog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestDesktopForwardUsesManagedDownloadedFileOffline(t *testing.T) {
	f := newFilterFixture(t, false)
	a := f.app
	a.syncEngine = nil
	a.paths = &platform.Paths{Data: t.TempDir()}
	a.attachmentStore = message.NewAttachmentStore(f.db)
	if err := os.MkdirAll(a.paths.AttachmentsPath(), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.paths.AttachmentsPath(), "original.pdf")
	if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	att := &message.Attachment{ID: "downloaded", MessageID: f.message.ID, Filename: "original.pdf", ContentType: "application/pdf", Size: 4, LocalPath: path}
	if err := a.attachmentStore.Create(att); err != nil {
		t.Fatal(err)
	}
	if err := a.attachmentStore.SetVerifiedDownload(att.ID, path, []byte("file")); err != nil {
		t.Fatal(err)
	}
	msg, err := a.messageStore.Get(f.message.ID)
	if err != nil {
		t.Fatal(err)
	}
	var attachments []smtp.Attachment
	if err := a.fetchForwardAttachments(zerolog.Nop(), msg, &attachments); err != nil || len(attachments) != 1 || string(attachments[0].Content) != "file" {
		t.Fatal("downloaded file did not forward offline", attachments, err)
	}
	for name, mutate := range map[string]func(){
		"wrong-size":     func() { mustWriteFixture(t, path, []byte("bad")) },
		"same-size-edit": func() { mustWriteFixture(t, path, []byte("edit")) },
		"outside": func() {
			att.LocalPath = filepath.Join(t.TempDir(), "outside")
			mustWriteFixture(t, att.LocalPath, []byte("file"))
		},
		"symlink": func() {
			outside := filepath.Join(t.TempDir(), "secret")
			mustWriteFixture(t, outside, []byte("file"))
			att.LocalPath = filepath.Join(a.paths.AttachmentsPath(), "link")
			if err := os.Symlink(outside, att.LocalPath); err != nil {
				t.Fatal(err)
			}
		},
		"missing": func() { att.LocalPath = filepath.Join(a.paths.AttachmentsPath(), "missing") },
	} {
		t.Run(name, func(t *testing.T) {
			att.LocalPath = path
			att.Size = 4
			mustWriteFixture(t, path, []byte("file"))
			if err := a.attachmentStore.SetVerifiedDownload(att.ID, path, []byte("file")); err != nil {
				t.Fatal(err)
			}
			mutate()
			if name == "outside" || name == "symlink" {
				if err := a.attachmentStore.SetVerifiedDownload(att.ID, att.LocalPath, []byte("file")); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.attachmentStore.UpdateLocalPath(att.ID, att.LocalPath); err != nil {
				t.Fatal(err)
			}
			if got, ok := a.attachmentStore.ReadVerifiedDownload(a.paths.AttachmentsPath(), att.ID); ok {
				t.Fatal("unsafe cache accepted", string(got))
			}
		})
	}
}

func TestEditedDownloadCannotReplaceOriginalOnSaveOrForward(t *testing.T) {
	f := newFilterFixture(t, false)
	a := f.app
	a.ctx = context.Background()
	a.paths = &platform.Paths{Data: t.TempDir()}
	a.attachmentStore = message.NewAttachmentStore(f.db)
	if _, err := f.db.Exec(`UPDATE accounts SET imap_host='imap.gmail.com',auth_type='oauth2' WHERE id='acct-a'`); err != nil {
		t.Fatal(err)
	}
	msg := &message.Message{AccountID: "acct-a", FolderID: "a-inbox", MessageID: "edited@test", Date: time.Now(), BodyFetched: true, BodyText: "original"}
	if err := a.messageStore.UpsertGmail(msg, "provider"); err != nil {
		t.Fatal(err)
	}
	att := &message.Attachment{ID: "editable", MessageID: msg.ID, Filename: "terms.txt", ContentType: "text/plain", Size: 11}
	if err := a.attachmentStore.Create(att); err != nil {
		t.Fatal(err)
	}
	if err := a.attachmentStore.SetGmailReference(att.ID, "provider", "part"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]string{"data": base64.RawURLEncoding.EncodeToString([]byte("approved=no"))})
	}))
	defer server.Close()
	a.syncEngine.SetGmailTokenSource(func(context.Context, string, bool) (string, error) { return "fixture", nil })
	a.syncEngine.SetGmailClientFactory(func(src gmail.TokenSource) *gmail.Client { c := gmail.NewClient(src); c.BaseURL = server.URL; return c })
	path, err := a.DownloadAttachment(att.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("initial verified download", calls)
	}
	if err := os.WriteFile(path, []byte("approved=ok"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "save.txt")
	if _, err := a.DownloadAttachment(att.ID, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "approved=no" || calls != 2 {
		t.Fatal("Save As trusted edited bytes", string(got), calls, err)
	}
	var files []smtp.Attachment
	if err := a.fetchForwardAttachments(zerolog.Nop(), msg, &files); err != nil || len(files) != 1 || string(files[0].Content) != "approved=no" || calls != 3 {
		t.Fatal("forward trusted edited bytes", files, calls, err)
	}
	a.syncEngine = nil
	files = nil
	if err := a.fetchForwardAttachments(zerolog.Nop(), msg, &files); err == nil {
		t.Fatal("offline edited bytes accepted as original")
	}
	// Returning to the verified original bytes restores offline cache use.
	if err := os.WriteFile(path, []byte("approved=no"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.fetchForwardAttachments(zerolog.Nop(), msg, &files); err != nil || len(files) != 1 || string(files[0].Content) != "approved=no" {
		t.Fatal("verified original unavailable offline", files, err)
	}
	if _, err := f.db.Exec(`DELETE FROM attachment_download_integrity WHERE attachment_id=?`, att.ID); err != nil {
		t.Fatal(err)
	}
	files = nil
	if err := a.fetchForwardAttachments(zerolog.Nop(), msg, &files); err == nil {
		t.Fatal("legacy download without integrity accepted")
	}
}

func mustWriteFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLargeVerifiedDownloadIgnoresCappedMetadataSize(t *testing.T) {
	f := newFilterFixture(t, false)
	a := f.app
	a.syncEngine = nil
	a.paths = &platform.Paths{Data: t.TempDir()}
	a.attachmentStore = message.NewAttachmentStore(f.db)
	if err := os.MkdirAll(a.paths.AttachmentsPath(), 0700); err != nil {
		t.Fatal(err)
	}
	payload := []byte(strings.Repeat("x", 11*1024*1024))
	path := filepath.Join(a.paths.AttachmentsPath(), "large.bin")
	mustWriteFixture(t, path, payload)
	att := &message.Attachment{ID: "large", MessageID: f.message.ID, Filename: "large.bin", Size: 10 * 1024 * 1024, LocalPath: path}
	if err := a.attachmentStore.Create(att); err != nil {
		t.Fatal(err)
	}
	if err := a.attachmentStore.SetVerifiedDownload(att.ID, path, payload); err != nil {
		t.Fatal(err)
	}
	var files []smtp.Attachment
	if err := a.fetchForwardAttachments(zerolog.Nop(), f.message, &files); err != nil || len(files) != 1 || len(files[0].Content) != len(payload) {
		t.Fatal("complete large file rejected", len(files), err)
	}
}

func TestForwardSharesGmailDataPartFetch(t *testing.T) {
	f := newFilterFixture(t, false)
	a := f.app
	a.ctx = context.Background()
	a.paths = &platform.Paths{Data: t.TempDir()}
	a.attachmentStore = message.NewAttachmentStore(f.db)
	if _, err := f.db.Exec(`UPDATE accounts SET imap_host='imap.gmail.com',auth_type='oauth2' WHERE id='acct-a'`); err != nil {
		t.Fatal(err)
	}
	msg := &message.Message{AccountID: "acct-a", FolderID: "a-inbox", Date: time.Now(), BodyFetched: true, BodyText: "original"}
	if err := a.messageStore.UpsertGmail(msg, "provider"); err != nil {
		t.Fatal(err)
	}
	var parts []any
	for _, id := range []string{"1", "2"} {
		att := &message.Attachment{ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(msg.ID+":"+id+":")).String(), MessageID: msg.ID, Filename: "attachment", Size: 3}
		if err := a.attachmentStore.Create(att); err != nil {
			t.Fatal(err)
		}
		if err := a.attachmentStore.SetGmailReference(att.ID, "provider", ""); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, map[string]any{"partId": id, "mimeType": "application/octet-stream", "body": map[string]any{"size": 3, "data": base64.RawURLEncoding.EncodeToString([]byte("abc"))}})
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("format") != "full" {
			t.Error(r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": map[string]any{"mimeType": "multipart/mixed", "parts": parts}})
	}))
	defer server.Close()
	a.syncEngine.SetGmailTokenSource(func(context.Context, string, bool) (string, error) { return "fixture", nil })
	a.syncEngine.SetGmailClientFactory(func(src gmail.TokenSource) *gmail.Client { c := gmail.NewClient(src); c.BaseURL = server.URL; return c })
	var files []smtp.Attachment
	if err := a.fetchForwardAttachments(zerolog.Nop(), msg, &files); err != nil || len(files) != 2 || calls != 1 {
		t.Fatal("forward refetched full message", len(files), calls, err)
	}
}
