package triage

import (
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"path/filepath"
	"testing"
)

func TestDefaultTriageScopeExcludesSystemMappingsHiddenAndDisabled(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled,archive_folder_path) VALUES('a','A','a@example.test','imap','smtp','a',1,'Old'),('b','B','b@example.test','imap','smtp','b',1,''),('off','Off','off@example.test','imap','smtp','off',0,'')`,
		`INSERT INTO folders(id,account_id,name,path,folder_type,hidden,parent_id) VALUES('inbox','a','Inbox','INBOX','inbox',0,NULL),('work','b','Work','Work','folder',0,NULL),('mapped','a','Old','Old','folder',0,NULL),('archive','a','Archive','Archive','archive',0,NULL),('spam','a','Spam','Spam','spam',0,NULL),('sent','a','Sent','Sent','sent',0,NULL),('drafts','a','Drafts','Drafts','drafts',0,NULL),('trash','a','Trash','Trash','trash',0,NULL),('all','a','All Mail','All Mail','all',0,NULL),('hidden','a','Hidden','Hidden','folder',1,NULL),('child','a','Child','Hidden/Child','folder',0,'hidden'),('disabled','off','Inbox','INBOX','inbox',0,NULL)`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	items, err := DefaultFolderScope(account.NewStore(db), folder.NewStore(db))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, item := range items {
		got[item.ID] = true
	}
	if len(got) != 2 || !got["inbox"] || !got["work"] {
		t.Fatalf("scope=%v", got)
	}
}
