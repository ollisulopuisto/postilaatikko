package app

import (
	"context"
	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"path/filepath"
	"testing"
	"time"
)

func TestAutomaticRefreshDoesNotCancelRunningSync(t *testing.T) {
	cancelled := false
	a := &App{ctx: context.Background(), syncContexts: map[string]context.CancelFunc{"a:f": func() { cancelled = true }}, syncLastRequest: map[string]time.Time{}}
	if err := a.syncFolder("a", "f", false); err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("automatic refresh cancelled a manual sync")
	}
}

func TestTriageEntryRefreshesOnlyEligibleSources(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "triage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES('a','A','a@example.test','imap','smtp','a',1),('off','Off','off@example.test','imap','smtp','off',0)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type,hidden) VALUES('inbox','a','Inbox','INBOX','inbox',0),('work','a','Work','Work','folder',0),('spam','a','Spam','Spam','spam',0),('hidden','a','Hidden','Hidden','folder',1),('disabled','off','Inbox','INBOX','inbox',0)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{accountStore: account.NewStore(db), folderStore: folder.NewStore(db)}
	got := map[string]bool{}
	if err := a.refreshTriageViewHeaders(func(accountID, folderID string) error { got[accountID+":"+folderID] = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["a:inbox"] || !got["a:work"] {
		t.Fatalf("scheduled sources=%v", got)
	}
}
