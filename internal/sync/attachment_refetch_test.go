package sync

import (
	"github.com/rs/zerolog"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/message"
)

// Reparse the real MIME fixture twice, as background and on-open fetches do.
// Stable parser IDs must not turn two legitimate same-name parts into four.
func TestReparsedMIMEKeepsAttachmentMultiplicity(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "attachments.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES('acc','Test','test@example.test','imap','smtp','test')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('folder','acc','Inbox','Inbox','inbox')`); err != nil {
		t.Fatal(err)
	}
	if err = message.NewStore(db).Create(&message.Message{ID: "msg-1", AccountID: "acc", FolderID: "folder", UID: 1, ThreadID: "thread", Date: time.Now(), ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	store := message.NewAttachmentStore(db)
	raw := []byte("MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain\r\n\r\nHello\r\n--b\r\nContent-Type: image/png\r\nContent-Disposition: inline; filename=image.png\r\nContent-ID: <img>\r\n\r\none\r\n--b\r\nContent-Type: image/png\r\nContent-Disposition: inline; filename=image.png\r\nContent-ID: <img>\r\n\r\ntwo\r\n--b--\r\n")
	engine := &Engine{log: zerolog.Nop()}
	first := engine.parseMessageBodyInternal(raw, "msg-1")
	if len(first.Attachments) != 2 {
		t.Fatalf("fixture parts=%d", len(first.Attachments))
	}
	if err = store.CreateBatch(first.Attachments); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE attachments SET local_path='/cache/photo.png' WHERE id=?`, first.Attachments[0].ID); err != nil {
		t.Fatal(err)
	}
	second := engine.parseMessageBodyInternal(raw, "msg-1")
	if second.Attachments[0].ID != first.Attachments[0].ID {
		t.Fatal("reparse must preserve cache identity")
	}
	if err = store.CreateBatch(second.Attachments); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByMessage("msg-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("same two MIME parts became %d entries after refetch", len(got))
	}
	var cachedPath string
	if err = db.QueryRow(`SELECT local_path FROM attachments WHERE id=?`, first.Attachments[0].ID).Scan(&cachedPath); err != nil || cachedPath != "/cache/photo.png" {
		t.Fatalf("cache path lost: %q %v", cachedPath, err)
	}
	var one, two int
	if err = db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE message_id='msg-1' AND content=?`, []byte("one")).Scan(&one); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE message_id='msg-1' AND content=?`, []byte("two")).Scan(&two); err != nil {
		t.Fatal(err)
	}
	if one != 1 || two != 1 {
		t.Fatalf("distinct same-name same-size payloads not preserved: %d %d", one, two)
	}
}
