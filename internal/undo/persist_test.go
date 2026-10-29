package undo

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type ephemeralCommand struct{ BaseCommand }

func (ephemeralCommand) Execute() error { return nil }
func (ephemeralCommand) Undo() error    { return nil }

type fakeUndoContext struct {
	UndoContext // nil embedded: rehydrated commands never execute in these tests
}

func openStoreDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "undo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		CREATE TABLE mailbox_undo_commands (
		  seq INTEGER PRIMARY KEY AUTOINCREMENT,
		  operation_id TEXT NOT NULL,
		  action TEXT NOT NULL,
		  message_ids TEXT NOT NULL DEFAULT '[]',
		  type TEXT NOT NULL,
		  payload TEXT NOT NULL,
		  created_at TEXT NOT NULL
		);`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStoreRoundTripRestoresOperationsAcrossStacks(t *testing.T) {
	db := openStoreDB(t)
	ctx := context.Background()
	uctx := &fakeUndoContext{}

	first := NewStack(500, 24*time.Hour)
	first.AttachStore(NewOperationStore(db), uctx)
	first.PushOperation("", NewFlagChangeCommand(ctx, uctx, "acc1", "INBOX", []string{"m1", "m2"}, []uint32{11, 12}, "read", false, "Marked 2 as read"), OperationMetadata{Action: "mark-read", MessageIDs: []string{"m1", "m2"}})
	first.PushOperation("", NewMoveCommand(uctx, "acc1", []MoveMessageIdentity{{LocalID: "m3", MessageID: "<a@b>"}}, "f-in", "f-arc", "Archived 1 conversation"), OperationMetadata{Action: "archive", MessageIDs: []string{"m3"}})

	second := NewStack(500, 24*time.Hour)
	second.AttachStore(NewOperationStore(db), uctx)
	if got := second.Size(); got != 2 {
		t.Fatalf("restored size = %d, want 2", got)
	}
	claim := second.PopLatestOperation()
	if claim == nil {
		t.Fatal("no claim from restored stack")
	}
	if claim.Action != "archive" {
		t.Fatalf("latest restored action = %q, want archive (LIFO)", claim.Action)
	}
	cmds := claim.Commands()
	move, ok := cmds[0].(*MoveCommand)
	if !ok {
		t.Fatalf("restored command type = %T, want *MoveCommand", cmds[0])
	}
	if move.sourceFolderID != "f-in" || move.destFolderID != "f-arc" || move.identities[0].LocalID != "m3" {
		t.Fatalf("restored move fields wrong: %+v", move)
	}

	// The flag op survives as the next LIFO item, with its fields intact.
	claim2 := second.PopLatestOperation()
	if claim2 == nil || claim2.Action != "mark-read" {
		t.Fatalf("second claim = %+v, want mark-read", claim2)
	}
	flag, ok := claim2.Commands()[0].(*FlagChangeCommand)
	if !ok {
		t.Fatalf("restored flag type = %T", claim2.Commands()[0])
	}
	if flag.flagType != "read" || flag.previousState != false || len(flag.uids) != 2 || flag.uids[1] != 12 {
		t.Fatalf("restored flag fields wrong: %+v", flag)
	}
}

func TestStoreExpiryAndSweep(t *testing.T) {
	db := openStoreDB(t)
	uctx := &fakeUndoContext{}
	store := NewOperationStore(db)

	// A command pushed 25h ago is outside the 24h window.
	old := NewFlagChangeCommand(context.Background(), uctx, "acc1", "INBOX", []string{"m9"}, []uint32{9}, "read", true, "old")
	old.createdAt = time.Now().Add(-25 * time.Hour)
	store.persistCommand("op-old", "mark-read", []string{"m9"}, old)

	fresh := NewFlagChangeCommand(context.Background(), uctx, "acc1", "INBOX", []string{"m10"}, []uint32{10}, "starred", false, "fresh")
	store.persistCommand("op-fresh", "star", []string{"m10"}, fresh)

	if got := store.Count(); got != 2 {
		t.Fatalf("count before sweep = %d, want 2", got)
	}
	if swept := store.Sweep(time.Now(), 24*time.Hour); swept != 1 {
		t.Fatalf("swept = %d, want 1", swept)
	}

	stack := NewStack(500, 24*time.Hour)
	stack.AttachStore(store, uctx)
	if got := stack.Size(); got != 1 {
		t.Fatalf("restored size = %d, want 1 (expired row not restored)", got)
	}
	claim := stack.PopLatestOperation()
	if claim == nil || claim.OperationID != "op-fresh" {
		t.Fatalf("claim = %+v, want op-fresh only", claim)
	}
}

func TestClaimDiscardAndClearRemovePersistedRows(t *testing.T) {
	db := openStoreDB(t)
	ctx := context.Background()
	uctx := &fakeUndoContext{}
	stack := NewStack(500, 24*time.Hour)
	store := NewOperationStore(db)
	stack.AttachStore(store, uctx)

	op1 := stack.PushOperation("", NewFlagChangeCommand(ctx, uctx, "a", "INBOX", []string{"m1"}, []uint32{1}, "read", false, "one"))
	op2 := stack.PushOperation("", NewFlagChangeCommand(ctx, uctx, "a", "INBOX", []string{"m2"}, []uint32{2}, "read", false, "two"))
	if got := store.Count(); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}

	// Claiming the newest removes only its rows.
	if claim := stack.PopLatestOperation(); claim == nil || claim.OperationID != op2 {
		t.Fatalf("claim = %+v, want %s", claim, op2)
	}
	if got := store.Count(); got != 1 {
		t.Fatalf("count after claim = %d, want 1", got)
	}

	// Discarding the failed remote action removes its row.
	if removed := stack.DiscardOperation(op1); removed != 1 {
		t.Fatalf("discarded = %d, want 1", removed)
	}
	if got := store.Count(); got != 0 {
		t.Fatalf("count after discard = %d, want 0", got)
	}

	stack.Push(NewFlagChangeCommand(ctx, uctx, "a", "INBOX", []string{"m3"}, []uint32{3}, "read", false, "three"))
	stack.Clear()
	if got := store.Count(); got != 0 {
		t.Fatalf("count after clear = %d, want 0", got)
	}
}

func TestRestoreOperationRePersistsFailedClaim(t *testing.T) {
	db := openStoreDB(t)
	ctx := context.Background()
	uctx := &fakeUndoContext{}
	stack := NewStack(500, 24*time.Hour)
	store := NewOperationStore(db)
	stack.AttachStore(store, uctx)

	stack.PushOperation("", NewFlagChangeCommand(ctx, uctx, "a", "INBOX", []string{"m1"}, []uint32{1}, "read", false, "one"))
	claim := stack.PopLatestOperation()
	if claim == nil {
		t.Fatal("no claim")
	}
	if got := store.Count(); got != 0 {
		t.Fatalf("count after claim = %d, want 0", got)
	}
	// Undo of command 0 failed: the whole claim is restored and re-persisted.
	stack.RestoreOperation(claim, 0)
	if got := store.Count(); got != 1 {
		t.Fatalf("count after restore = %d, want 1", got)
	}
	if !stack.CanUndo() {
		t.Fatal("restored operation not undoable")
	}
}

func TestNonDurableCommandStaysMemoryOnly(t *testing.T) {
	db := openStoreDB(t)
	store := NewOperationStore(db)
	stack := NewStack(500, 24*time.Hour)
	stack.AttachStore(store, &fakeUndoContext{})
	stack.Push(&ephemeralCommand{NewBaseCommand("ephemeral")})
	if got := store.Count(); got != 0 {
		t.Fatalf("count = %d, want 0 for non-durable command", got)
	}
	if got := stack.Size(); got != 1 {
		t.Fatalf("stack size = %d, want 1 (memory-only still undoable)", got)
	}
}
