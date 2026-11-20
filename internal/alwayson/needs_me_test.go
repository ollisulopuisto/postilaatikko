package alwayson

import (
	"testing"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
)

func TestRemoteNeedsMeTriageScopeListCountAndSearch(t *testing.T) {
	db, _, _ := newCachedVirtualFolderTestRuntime(t)
	defer db.Close()
	if err := (views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: "triage"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO read_later_queue(account_id,conversation_id) VALUES('acc','thread-b')`); err != nil {
		t.Fatal(err)
	}
	o := &catalogReadOperations{accounts: account.NewStore(db), folders: folder.NewStore(db), messages: message.NewStore(db)}
	items, err := o.ListConversationsUnifiedFolder("triage", 0, 50, "newest", "needsMe")
	if err != nil {
		t.Fatal(err)
	}
	count, err := o.CountConversationsUnifiedFolder("triage", "needsMe")
	if err != nil {
		t.Fatal(err)
	}
	if count != len(items) || count != 2 {
		t.Fatalf("list=%d count=%d", len(items), count)
	}
	for _, item := range items {
		if item.AccountID != "acc" {
			t.Fatalf("account changed: %+v", item)
		}
	}
	results, total, err := o.SearchConversationsUnifiedFolder("triage", "A", 0, 50, "needsMe")
	if err != nil || total != 1 || len(results) != 1 {
		t.Fatalf("search list=%d total=%d err=%v", len(results), total, err)
	}
	if _, err := db.Exec(`UPDATE folders SET folder_type='archive' WHERE id='inbox'`); err != nil {
		t.Fatal(err)
	}
	items, err = o.ListConversationsUnifiedFolder("triage", 0, 50, "newest", "needsMe")
	if err != nil || len(items) != 0 {
		t.Fatalf("excluded list=%d %v", len(items), err)
	}
	count, err = o.CountConversationsUnifiedFolder("triage", "needsMe")
	if err != nil || count != 0 {
		t.Fatalf("excluded count=%d %v", count, err)
	}
	results, total, err = o.SearchConversationsUnifiedFolder("triage", "A", 0, 50, "needsMe")
	if err != nil || total != 0 || len(results) != 0 {
		t.Fatalf("excluded search=%d/%d %v", len(results), total, err)
	}
}
