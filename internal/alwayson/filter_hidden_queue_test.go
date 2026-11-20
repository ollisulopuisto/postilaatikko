package alwayson

import (
	"context"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/mailboxevents"
	"github.com/ollisulopuisto/postilaatikko/internal/mailfilter"
	"github.com/ollisulopuisto/postilaatikko/internal/mailmove"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"path/filepath"
	"testing"
	"time"
)

func TestHiddenDestinationPausesQueuedJobAndShowResumes(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "hidden-queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username) VALUES ('a','A','a@example.com','imap','smtp','a'),('b','B','b@example.com','imap','smtp','b')`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES ('source','a','Inbox','INBOX','inbox'),('destination','b','Archive','Archive','archive')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	messages := message.NewStore(db)
	folders := folder.NewStore(db)
	rules := mailfilter.NewStore(db)
	jobs := mailfilter.NewJobStore(db)
	m := &message.Message{ID: "m", AccountID: "a", FolderID: "source", UID: 10, FromEmail: "sender@example.com", Date: time.Now()}
	if err := messages.Create(m); err != nil {
		t.Fatal(err)
	}
	rule := &mailfilter.Rule{ID: "rule", Name: "Route", Enabled: true, FromContains: []string{"sender@example.com"}, DestFolderID: "destination"}
	if err := rules.Create(rule); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Enqueue(rule, rule.DestFolderID, m); err != nil {
		t.Fatal(err)
	}
	if err := folders.SetHiddenBatch("b", []string{"destination"}, true); err != nil {
		t.Fatal(err)
	}
	moves := 0
	r := &SyncRuntime{messages: messages, rules: rules, folders: folders, filterJobs: jobs, filterOp: mailmove.FilterOperation{Messages: messages, Move: func(context.Context, string, *message.Message, *folder.Folder) error { moves++; return nil }}}
	r.events = mailboxevents.NewBroker(mailboxevents.Options{})
	defer r.CloseEventStreams()
	_, events, _, cancel, subscribeErr := r.events.Subscribe(0)
	if subscribeErr != nil {
		t.Fatal(subscribeErr)
	}
	defer cancel()
	job, err := jobs.ClaimNext()
	if err != nil || job == nil {
		t.Fatalf("claim %+v %v", job, err)
	}
	r.runFilterJob(context.Background(), job)
	if moves != 0 {
		t.Fatal("provider called after destination hidden")
	}
	if next, err := jobs.ClaimNext(); next != nil || err != nil {
		t.Fatalf("paused job busy-reclaimed %+v %v", next, err)
	}
	if unknown, err := jobs.ListOutcomeUnknown(0, 10); err != nil || len(unknown) != 0 {
		t.Fatalf("unattempted work reported uncertain %v %v", unknown, err)
	}
	if err := folders.SetHiddenBatch("b", []string{"destination"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE automatic_filter_jobs SET updated_at=datetime('now','-1 second') WHERE id=?`, job.ID); err != nil {
		t.Fatal(err)
	}
	resumed, err := jobs.ClaimNext()
	if err != nil || resumed == nil || resumed.ID != job.ID || resumed.SourceUID != 10 {
		t.Fatalf("resume lost snapshot %+v %v", resumed, err)
	}
	r.runFilterJob(context.Background(), resumed)
	if moves != 1 {
		t.Fatalf("provider calls after Show %d", moves)
	}
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case e := <-events:
			got[e.AccountID+":"+e.FolderID] = true
		case <-time.After(time.Second):
			t.Fatalf("filter missing invalidation: %v", got)
		}
	}
	if !got["a:source"] || !got["b:destination"] {
		t.Fatal(got)
	}
	stored, err := rules.Get(rule.ID)
	if err != nil || !stored.Enabled {
		t.Fatal("pausing changed the enabled flag")
	}
	if next, err := jobs.ClaimNext(); next != nil || err != nil {
		t.Fatalf("completed job remained pending %+v %v", next, err)
	}
}
