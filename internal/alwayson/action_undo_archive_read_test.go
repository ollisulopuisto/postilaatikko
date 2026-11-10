package alwayson

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
)

func setupArchiveReadUndoRuntime(t *testing.T) (*mailboxActionOperations, *message.Store) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "archive-read-undo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES
		 ('a','A','a@example.com','imap','smtp','a',1)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES
		 ('fin','a','Inbox','INBOX','inbox'),('far','a','Archive','Archive','archive')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	messages := message.NewStore(db)
	item := message.Message{ID: "m1", AccountID: "a", FolderID: "fin", UID: 1, ThreadID: "m1", Date: time.Now(), IsRead: false}
	if err := messages.Create(&item); err != nil {
		t.Fatal(err)
	}
	operations := &mailboxActionOperations{
		runtime: &SyncRuntime{messages: messages, folders: folder.NewStore(db)},
		gmailClientFunc: func(string) (*gmail.Client, bool, error) {
			return nil, false, nil
		},
	}
	return operations, messages
}

func TestCaptureActionUndoArchiveMarkReadExpectsArchivedAndRead(t *testing.T) {
	operations, _ := setupArchiveReadUndoRuntime(t)
	payload, err := operations.CaptureActionUndo(context.Background(), v1.ActionRequest{Action: v1.ActionArchiveMarkRead}, []string{"m1"})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot mailboxActionUndoSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Action != v1.ActionArchiveMarkRead {
		t.Fatalf("snapshot action = %q, want %q", snapshot.Action, v1.ActionArchiveMarkRead)
	}
	if len(snapshot.Entries) != 1 {
		t.Fatalf("snapshot entries = %d, want 1", len(snapshot.Entries))
	}
	entry := snapshot.Entries[0]
	if entry.FolderID != "fin" {
		t.Errorf("original folder = %q, want fin", entry.FolderID)
	}
	if entry.ExpectedFolderID != "far" {
		t.Errorf("expected folder after archive_read = %q, want far (Archive)", entry.ExpectedFolderID)
	}
	if entry.IsRead {
		t.Errorf("original read state must stay unread in the snapshot")
	}
	if !entry.ExpectedIsRead {
		t.Errorf("expected read state after archive_read must be read, so Undo can tell the action completed")
	}
}

func TestPlanActionGroupsArchiveMarkReadTargetsArchive(t *testing.T) {
	operations, _ := setupArchiveReadUndoRuntime(t)
	groups, err := operations.PlanActionGroups(context.Background(), v1.ActionRequest{Action: v1.ActionArchiveMarkRead}, []string{"m1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	group := groups[0]
	if group.DestinationFolderID != "far" || group.DestinationAccountID != "a" {
		t.Errorf("archive_read must group toward the Archive folder, got destination %s/%s", group.DestinationAccountID, group.DestinationFolderID)
	}
	if group.SourceFolderID != "fin" {
		t.Errorf("source folder = %q, want fin", group.SourceFolderID)
	}
}

func TestUndoActionArchiveMarkReadRejectsChangedReadState(t *testing.T) {
	operations, messages := setupArchiveReadUndoRuntime(t)
	payload, err := operations.CaptureActionUndo(context.Background(), v1.ActionRequest{Action: v1.ActionArchiveMarkRead}, []string{"m1"})
	if err != nil {
		t.Fatal(err)
	}
	// The message never reached the archive-read end state: it is still in the
	// Inbox and still unread, so Undo must refuse instead of guessing.
	if _, err := messages.GetByIDs([]string{"m1"}); err != nil {
		t.Fatal(err)
	}
	if err := operations.UndoAction(context.Background(), v1.ActionArchiveMarkRead, payload); err == nil {
		t.Fatal("Undo succeeded although the message never reached the archived-read state")
	}
}
