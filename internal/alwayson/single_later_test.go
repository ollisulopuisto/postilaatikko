package alwayson

import (
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
	"testing"
)

func TestRemoteLaterCatalogUnion(t *testing.T) {
	db, _, _ := newCachedVirtualFolderTestRuntime(t)
	defer db.Close()
	for _, query := range []string{`INSERT INTO read_later_queue(account_id,conversation_id) VALUES('acc','thread-a')`, `INSERT INTO reply_later_queue(account_id,conversation_id) VALUES('acc','thread-a'),('acc','thread-b')`} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	o := &catalogReadOperations{accounts: account.NewStore(db), folders: folder.NewStore(db), messages: message.NewStore(db)}
	for _, alias := range []string{"readLater", "replyLater"} {
		if err := (views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: alias}).Validate(); err != nil {
			t.Fatal(err)
		}
		items, err := o.ListConversationsUnifiedFolder(alias, 0, 50, "newest", "")
		if err != nil {
			t.Fatal(err)
		}
		count, err := o.CountConversationsUnifiedFolder(alias, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || count != 2 {
			t.Fatalf("%s: list=%d count=%d", alias, len(items), count)
		}
		// Empty text search still exercises scope/folder resolution and union filtering.
		_, _, err = o.SearchConversationsUnifiedFolder(alias, "project", 0, 50, "")
		if err != nil {
			t.Fatal(err)
		}
	}
}
