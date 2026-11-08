package alwayson

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/views"
)

func TestAllInboxesResolverSelectsOneEnabledInboxPerAccount(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "all-inboxes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES
		 ('enabled','Enabled','enabled@example.com','imap','smtp','enabled',1),
		 ('disabled','Disabled','disabled@example.com','imap','smtp','disabled',0),
		 ('empty','Empty','empty@example.com','imap','smtp','empty',1)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES
		 ('enabled-inbox-a','enabled','Inbox A','INBOX-A','inbox'),
		 ('enabled-inbox-b','enabled','Inbox B','INBOX-B','inbox'),
		 ('disabled-inbox','disabled','Inbox','INBOX','inbox'),
		 ('custom-folder','empty','Other','Other','folder')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	accounts := account.NewStore(db)
	folders := folder.NewStore(db)
	wantFolder, err := folders.GetByType("enabled", folder.TypeInbox)
	if err != nil || wantFolder == nil {
		t.Fatalf("resolve expected inbox: folder=%#v err=%v", wantFolder, err)
	}
	got, err := resolveSpecialFolderIDs(accounts, folders, folder.TypeInbox)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{wantFolder.ID}) {
		t.Fatalf("resolved inbox IDs = %v, want only enabled account's designated inbox %q", got, wantFolder.ID)
	}

	operations := &mailboxReadOperations{runtime: &SyncRuntime{accounts: accounts, folders: folders}}
	request, empty, err := operations.searchRequest(views.Scope{Kind: views.ScopeAllInboxes}, "", "", "", 50)
	if err != nil || empty {
		t.Fatalf("all-inboxes search request: empty=%t err=%v", empty, err)
	}
	if !reflect.DeepEqual(request.FolderIDs, got) {
		t.Fatalf("search folder IDs = %v, resolver IDs = %v", request.FolderIDs, got)
	}
}

func TestAllInboxesResolverReturnsEmptyWhenNoEnabledAccountHasInbox(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "no-inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,enabled) VALUES ('empty','Empty','empty@example.com','imap','smtp','empty',1)`); err != nil {
		t.Fatal(err)
	}

	accounts := account.NewStore(db)
	folders := folder.NewStore(db)
	got, err := resolveSpecialFolderIDs(accounts, folders, folder.TypeInbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("resolved inbox IDs = %v, want empty", got)
	}

	operations := &mailboxReadOperations{runtime: &SyncRuntime{accounts: accounts, folders: folders}}
	_, empty, err := operations.searchRequest(views.Scope{Kind: views.ScopeAllInboxes}, "", "", "", 50)
	if err != nil || !empty {
		t.Fatalf("all-inboxes search request: empty=%t err=%v, want empty without error", empty, err)
	}
}
