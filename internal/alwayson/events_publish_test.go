package alwayson

import (
	"context"
	"encoding/json"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/mailboxevents"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/replylater"
	"github.com/ollisulopuisto/postilaatikko/internal/triage"
	"testing"
	"time"
)

func TestMoveInvalidatesEverySourceAndDestination(t *testing.T) {
	ops, _, db := setupArchiveReadUndoRuntime(t)
	ops.runtime.events = mailboxevents.NewBroker(mailboxevents.Options{})
	t.Cleanup(func() { _ = ops.runtime.events.Close() })
	_, events, _, cancel, err := ops.runtime.events.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	ops.moveToFolderFunc = moveToFolderInDB(db)
	msgs := []*message.Message{{ID: "m1", AccountID: "a", FolderID: "fin"}}
	if err := ops.moveToFolder(msgs, &folder.Folder{ID: "far", AccountID: "a"}); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case e := <-events:
			if e.Type != v1.EventMailboxChanged {
				t.Fatalf("unexpected %v", e)
			}
			got[e.AccountID+":"+e.FolderID] = true
		case <-time.After(time.Second):
			t.Fatalf("missing source/destination events: %v", got)
		}
	}
	if !got["a:fin"] || !got["a:far"] {
		t.Fatal(got)
	}
}

func TestSnoozeExpiryInvalidatesOnlyChanges(t *testing.T) {
	r := replyModeRuntime(t)
	r.triage = triage.NewStore(r.db)
	r.events = mailboxevents.NewBroker(mailboxevents.Options{})
	defer r.CloseEventStreams()
	_, events, _, cancel, err := r.events.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if err := r.expireSnoozes(time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-events:
		t.Fatalf("empty expiry emitted %v", e)
	default:
	}
	if err := r.triage.Snooze(triage.Ref{AccountID: "a", ConversationID: "m"}, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := r.expireSnoozes(time.Now().Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-events:
		if e.Type != v1.EventTriageChanged {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("missing expiry event")
	}
}
func TestTriageUndoPublishesInvalidation(t *testing.T) {
	r := replyModeRuntime(t)
	r.triage = triage.NewStore(r.db)
	r.replyLater = replylater.NewStore(r.db)
	r.events = mailboxevents.NewBroker(mailboxevents.Options{})
	defer r.CloseEventStreams()
	_, events, _, cancel, err := r.events.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, err := r.triage.SetReadLater(triage.Ref{AccountID: "a", ConversationID: "m"}, true); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(triageActionUndoSnapshot{Version: 1, Action: v1.ActionReadLaterAdd, Entries: []triageActionUndoSnapshotEntry{{AccountID: "a", ConversationID: "m", PriorInQueue: false, ExpectedInQueue: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&mailboxActionOperations{runtime: r}).undoTriageAction(context.Background(), v1.ActionReadLaterAdd, raw); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-events:
		if e.Type != v1.EventTriageChanged {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("missing undo event")
	}
}
func TestCloseEventStreamsUnblocksBeforeDatabaseClose(t *testing.T) {
	r := replyModeRuntime(t)
	r.events = mailboxevents.NewBroker(mailboxevents.Options{})
	_, events, _, cancel, err := r.events.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	r.CloseEventStreams()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("stream still open")
		}
	case <-time.After(time.Second):
		t.Fatal("stream remained blocked")
	}
	if _, err := r.db.Exec("SELECT 1"); err != nil {
		t.Fatalf("database closed too early: %v", err)
	}
}
