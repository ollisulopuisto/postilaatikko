package message

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttachmentRepairDuplicatedCacheExactPayloadRestoreAndRefetch(t *testing.T) {
	messages, db := setupThreadTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	if err := messages.Create(&Message{ID: "mail", AccountID: "acc-1", FolderID: "fld-1", UID: 1, ThreadID: "mail", Date: time.Now(), ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	store := NewAttachmentStore(db)
	root := t.TempDir()
	path := filepath.Join(root, "saved.png")
	if err := os.WriteFile(path, []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old-1", "old-2", "old-3", "old-4"} {
		a := &Attachment{ID: id, MessageID: "mail", Filename: "same.png", ContentType: "image/png", Size: 3, IsInline: true, ContentID: "cid", Content: []byte("one"), LocalPath: path}
		if err := store.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	parts := []RepairPart{
		{Metadata: Attachment{ID: "canonical-1", MessageID: "mail", Filename: "same.png", ContentType: "image/png", Size: 3, IsInline: true, ContentID: "cid"}, Payload: []byte("one")},
		{Metadata: Attachment{ID: "canonical-2", MessageID: "mail", Filename: "same.png", ContentType: "image/png", Size: 3}, Payload: []byte("two")},
	}
	if _, err := store.RepairFromMIME("wrong", "mail", "digest", parts, root); err == nil {
		t.Fatal("cross-account repair accepted")
	}
	backup, err := store.RepairFromMIME("acc-1", "mail", "digest", parts, root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByMessage("mail")
	if err != nil || len(got) != 2 {
		t.Fatalf("duplicates not repaired: %d %v", len(got), err)
	}
	for _, a := range got {
		payload, ok, err := store.RepairedContent(a.ID)
		if err != nil || !ok {
			t.Fatalf("missing exact bytes %s: %v", a.ID, err)
		}
		if a.ID == "canonical-1" && (string(payload) != "one" || a.LocalPath != "") {
			t.Fatal("filename-only path mapping")
		}
		if a.ID == "canonical-2" && (string(payload) != "two" || a.LocalPath != path) {
			t.Fatal("matching cached file not preserved")
		}
	}
	if err := store.Create(&Attachment{ID: "late-single", MessageID: "mail", Filename: "same.png", Size: 3}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateBatch([]*Attachment{{ID: "late-batch", MessageID: "mail", Filename: "same.png", Size: 3}}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetByMessage("mail")
	if err != nil || len(got) != 2 {
		t.Fatal("late refetch reintroduced duplicates")
	}
	again, err := store.RepairFromMIME("acc-1", "mail", "digest", parts, root)
	if err != nil || again != backup {
		t.Fatal("repair not idempotent")
	}
	if err = store.RestoreRepair("acc-1", "mail", "stale"); err == nil {
		t.Fatal("stale undo accepted")
	}
	if err = store.RestoreRepair("acc-1", "mail", backup); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetByMessage("mail")
	if err != nil || len(got) != 4 {
		t.Fatalf("restore count %d %v", len(got), err)
	}
	var inline []byte
	if err = db.QueryRow(`SELECT content FROM attachments WHERE id='old-1'`).Scan(&inline); err != nil || string(inline) != "one" {
		t.Fatalf("backup lost inline bytes: %q %v", inline, err)
	}
	if bytes, err := os.ReadFile(path); err != nil || string(bytes) != "two" {
		t.Fatal("repair/restore changed cached file")
	}
	if _, ok, err := store.RepairedContent("canonical-1"); err != nil || ok {
		t.Fatal("restore left canonical payload")
	}
	// Undo is itself reversible, with a new generation; old token is stale.
	next, err := store.RepairBackupID("mail")
	if err != nil || next == "" || next == backup {
		t.Fatal("restore has no new undo generation")
	}
	if err = store.RestoreRepair("acc-1", "mail", backup); err == nil {
		t.Fatal("old generation reusable")
	}
	if err = store.RestoreRepair("acc-1", "mail", next); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetByMessage("mail")
	if err != nil || len(got) != 2 {
		t.Fatal("undo restore failed")
	}
	if content, ok, err := store.RepairedContent("canonical-2"); err != nil || !ok || string(content) != "two" {
		t.Fatal("undo restore lost exact payload")
	}

}

func TestAttachmentRepairRollbackLeavesOriginalAndNoBackupMarker(t *testing.T) {
	messages, db := setupThreadTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	if err := messages.Create(&Message{ID: "mail", AccountID: "acc-1", FolderID: "fld-1", UID: 1, ThreadID: "mail", Date: time.Now(), ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	store := NewAttachmentStore(db)
	if err := store.Create(&Attachment{ID: "old", MessageID: "mail", Filename: "x", Size: 1}); err != nil {
		t.Fatal(err)
	}
	bad := []RepairPart{{Metadata: Attachment{ID: "same-id", MessageID: "mail", Filename: "x", Size: 1}, Payload: []byte("1")}, {Metadata: Attachment{ID: "same-id", MessageID: "mail", Filename: "y", Size: 1}, Payload: []byte("2")}}
	if _, err := store.RepairFromMIME("acc-1", "mail", "digest", bad, ""); err == nil {
		t.Fatal("duplicate canonical IDs accepted")
	}
	got, err := store.GetByMessage("mail")
	if err != nil || len(got) != 1 || got[0].ID != "old" {
		t.Fatal("failed transaction lost original cache")
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM attachment_repair_backups`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed transaction left backup")
	}
	if id, err := store.RepairBackupID("mail"); err != nil || id != "" {
		t.Fatal("failed transaction marked repaired")
	}
}
