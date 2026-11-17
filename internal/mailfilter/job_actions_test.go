package mailfilter

import (
	"github.com/hkdb/aerion/internal/message"
	"testing"
	"time"
)

func TestExplicitJobsAreAtomicAndSnapshotActions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	jobs := NewJobStore(db)
	rule := &Rule{ID: "r", Name: "Read list", AccountIDs: []string{"account1", "account2"}, ListIDContains: []string{"news"}, ActionMarkRead: true, ActionTrash: true}
	if err := NewStore(db).Create(rule); err != nil {
		t.Fatal(err)
	}
	restored, err := NewStore(db).Get(rule.ID)
	if err != nil || len(restored.AccountIDs) != 2 || len(restored.ListIDContains) != 1 || !restored.ActionTrash || !restored.ActionMarkRead {
		t.Fatalf("actions/scope lost: %+v %v", restored, err)
	}
	good := &message.Message{ID: "m", AccountID: "account1", FolderID: "folder1", UID: 1, Date: time.Now()}
	bad := &message.Message{ID: "unstable", AccountID: "account1", FolderID: "folder1"}
	if err := jobs.EnqueueBatch(rule, "", []*message.Message{good, bad}, true); err == nil {
		t.Fatal("unstable existing selection accepted")
	}
	if job, err := jobs.ClaimNext(); job != nil || err != nil {
		t.Fatalf("rejected batch queued partial mail: %+v %v", job, err)
	}
	if err := jobs.EnqueueBatch(rule, "", []*message.Message{good}, true); err != nil {
		t.Fatal(err)
	}
	// A duplicate HTTP submission cannot duplicate pending provider work.
	if err := jobs.EnqueueBatch(rule, "", []*message.Message{good}, true); err != nil {
		t.Fatal(err)
	}
	job, err := jobs.ClaimNext()
	if err != nil || job == nil || !job.Actions.ApplyExisting || !job.Actions.MarkRead || !job.Actions.Trash || job.DestinationFolderID != "" {
		t.Fatalf("action-only snapshot lost: %+v %v", job, err)
	}
	if next, err := jobs.ClaimNext(); next != nil || err != nil {
		t.Fatalf("duplicate work: %+v %v", next, err)
	}
}
