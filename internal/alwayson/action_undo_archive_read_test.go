package alwayson

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
)

func setupArchiveReadUndoRuntime(t *testing.T) (*mailboxActionOperations, *message.Store, *database.DB) {
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
	return operations, messages, db
}

// moveToFolderInDB simulates a provider move by updating the local store, the
// same way a confirmed provider move would be mirrored locally.
func moveToFolderInDB(db *database.DB) func([]*message.Message, *folder.Folder) error {
	return func(messages []*message.Message, destination *folder.Folder) error {
		for _, msg := range messages {
			if _, err := db.Exec(`UPDATE messages SET folder_id = ? WHERE id = ?`, destination.ID, msg.ID); err != nil {
				return err
			}
		}
		return nil
	}
}

func archiveReadGroup() v1.ActionProviderGroup {
	return v1.ActionProviderGroup{
		ID:                   "group-1",
		AccountID:            "a",
		SourceFolderID:       "fin",
		DestinationAccountID: "a",
		DestinationFolderID:  "far",
		MessageIDs:           []string{"m1"},
	}
}

func TestExecuteActionGroupArchiveMarkReadCompensatesMoveOnFlagFailure(t *testing.T) {
	operations, messages, db := setupArchiveReadUndoRuntime(t)
	operations.moveToFolderFunc = moveToFolderInDB(db)
	operations.setIMAPFlagsFunc = func(accountID, folderID string, _ []*message.Message, kind string, enabled bool) error {
		return errors.New("provider rejected the flag update")
	}
	err := operations.ExecuteActionGroup(context.Background(), v1.ActionRequest{Action: v1.ActionArchiveMarkRead}, archiveReadGroup())
	if err == nil {
		t.Fatal("ExecuteActionGroup succeeded although the flag update failed")
	}
	restored, err := messages.GetByIDs([]string{"m1"})
	if err != nil {
		t.Fatal(err)
	}
	if restored[0].FolderID != "fin" {
		t.Errorf("message folder after compensation = %q, want fin (the move must be reversed)", restored[0].FolderID)
	}
	if restored[0].IsRead {
		t.Errorf("message must stay unread when the read update failed")
	}
}

func TestExecuteActionGroupArchiveMarkReadConfirmsOnlyAfterBothEffects(t *testing.T) {
	operations, messages, db := setupArchiveReadUndoRuntime(t)
	operations.moveToFolderFunc = moveToFolderInDB(db)
	operations.setIMAPFlagsFunc = func(accountID, folderID string, _ []*message.Message, kind string, enabled bool) error {
		return nil
	}
	if err := operations.ExecuteActionGroup(context.Background(), v1.ActionRequest{Action: v1.ActionArchiveMarkRead}, archiveReadGroup()); err != nil {
		t.Fatal(err)
	}
	updated, err := messages.GetByIDs([]string{"m1"})
	if err != nil {
		t.Fatal(err)
	}
	if updated[0].FolderID != "far" {
		t.Errorf("message folder = %q, want far", updated[0].FolderID)
	}
	if !updated[0].IsRead {
		t.Errorf("message must be read after a confirmed archive_read")
	}
}

func TestCaptureActionUndoArchiveMarkReadExpectsArchivedAndRead(t *testing.T) {
	operations, _, _ := setupArchiveReadUndoRuntime(t)
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
	operations, _, _ := setupArchiveReadUndoRuntime(t)
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
	operations, messages, _ := setupArchiveReadUndoRuntime(t)
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

// The provider accepts the read flag but the local persist fails: compensation
// must restore both the folder and the original unread state.
func TestExecuteActionGroupArchiveMarkReadRestoresUnreadWhenPersistFails(t *testing.T) {
	operations, messages, db := setupArchiveReadUndoRuntime(t)
	operations.moveToFolderFunc = moveToFolderInDB(db)
	// The provider applies flag changes; a sync would eventually surface them
	// locally, so the stub writes them through.
	operations.setIMAPFlagsFunc = func(accountID, folderID string, msgs []*message.Message, kind string, enabled bool) error {
		if kind != "read" {
			return nil
		}
		for _, msg := range msgs {
			if _, err := db.Exec(`UPDATE messages SET is_read = ? WHERE id = ?`, enabled, msg.ID); err != nil {
				return err
			}
		}
		return nil
	}
	operations.persistFlagGroupFunc = func(msgs []*message.Message, kind string, enabled bool) error {
		if kind == "read" && enabled {
			return errors.New("local flag persistence failed")
		}
		return nil
	}
	err := operations.ExecuteActionGroup(context.Background(), v1.ActionRequest{Action: v1.ActionArchiveMarkRead}, archiveReadGroup())
	if err == nil {
		t.Fatal("ExecuteActionGroup succeeded although the flag persistence failed")
	}
	restored, err := messages.GetByIDs([]string{"m1"})
	if err != nil {
		t.Fatal(err)
	}
	if restored[0].FolderID != "fin" {
		t.Errorf("message folder after compensation = %q, want fin", restored[0].FolderID)
	}
	if restored[0].IsRead {
		t.Errorf("message must be unread again after compensation; provider flags changed before the failure must be reversed")
	}
}

// Only messages that were unread before the action may be restored to unread;
// a message that was already read must stay read through compensation.
func TestExecuteActionGroupArchiveMarkReadRestoresOnlyOriginallyUnread(t *testing.T) {
	operations, messages, db := setupArchiveReadUndoRuntime(t)
	second := message.Message{ID: "m2", AccountID: "a", FolderID: "fin", UID: 2, ThreadID: "m2", Date: time.Now(), IsRead: true}
	if err := messages.Create(&second); err != nil {
		t.Fatal(err)
	}
	operations.moveToFolderFunc = moveToFolderInDB(db)
	operations.setIMAPFlagsFunc = func(accountID, folderID string, msgs []*message.Message, kind string, enabled bool) error {
		if kind != "read" {
			return nil
		}
		for _, msg := range msgs {
			if _, err := db.Exec(`UPDATE messages SET is_read = ? WHERE id = ?`, enabled, msg.ID); err != nil {
				return err
			}
		}
		return nil
	}
	operations.persistFlagGroupFunc = func(msgs []*message.Message, kind string, enabled bool) error {
		if kind == "read" && enabled {
			return errors.New("local flag persistence failed")
		}
		return nil
	}
	group := archiveReadGroup()
	group.MessageIDs = []string{"m1", "m2"}
	err := operations.ExecuteActionGroup(context.Background(), v1.ActionRequest{Action: v1.ActionArchiveMarkRead}, group)
	if err == nil {
		t.Fatal("ExecuteActionGroup succeeded although the flag persistence failed")
	}
	restored, err := messages.GetByIDs([]string{"m1", "m2"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]bool{}
	folderByID := map[string]string{}
	for _, msg := range restored {
		byID[msg.ID] = msg.IsRead
		folderByID[msg.ID] = msg.FolderID
	}
	if byID["m1"] {
		t.Errorf("originally unread message m1 must be unread again after compensation")
	}
	if !byID["m2"] {
		t.Errorf("originally read message m2 must stay read; compensation must not flip its state")
	}
	if folderByID["m1"] != "fin" || folderByID["m2"] != "fin" {
		t.Errorf("both messages must be back in fin, got m1=%q m2=%q", folderByID["m1"], folderByID["m2"])
	}
}
