package app

import (
	"context"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/virtualfolder"
	"testing"
	"time"
)

func TestCountSubscribersReuseAndCancelIndependently(t *testing.T) {
	a := &App{ctx: context.Background()}
	first, ctx, fresh, err := a.acquireViewCount("one", "v1", "key")
	if err != nil || !fresh {
		t.Fatal(err)
	}
	second, _, fresh, err := a.acquireViewCount("two", "v2", "key")
	if err != nil || fresh || first != second {
		t.Fatal("scan not coalesced", err)
	}
	a.CancelVirtualFolderOnlineCount("one")
	if ctx.Err() != nil {
		t.Fatal("one subscriber cancelled shared work")
	}
	state, err := a.GetVirtualFolderOnlineCount("two")
	if err != nil || state.SessionID != "two" || state.VirtualFolderID != "v2" {
		t.Fatal(state, err)
	}
	a.CancelVirtualFolderOnlineCount("two")
	if ctx.Err() == nil {
		t.Fatal("last subscriber did not cancel")
	}
}

func TestCountOnlyFreshExactResultsAreReusable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		complete  bool
		age       time.Duration
		wantReuse bool
	}{
		{"exact", true, 0, true}, {"partial", false, 0, false}, {"expired", true, 2 * time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{ctx: context.Background()}
			old, _, _, _ := a.acquireViewCount("one", "v", "key")
			old.state.Running = false
			old.state.Complete = tc.complete
			old.created = time.Now().Add(-tc.age)
			a.CancelVirtualFolderOnlineCount("one")
			next, _, fresh, err := a.acquireViewCount("two", "v", "key")
			defer a.CancelVirtualFolderOnlineCount("two")
			if err != nil || (next == old) != tc.wantReuse || fresh == tc.wantReuse {
				t.Fatal("reuse mismatch", err)
			}
		})
	}
}

func TestCountMutationAndExplicitRefreshInvalidateActiveAndCompleted(t *testing.T) {
	for _, running := range []bool{true, false} {
		a := &App{ctx: context.Background(), runtimeEventEmitter: func(string, ...interface{}) {}}
		old, ctx, _, _ := a.acquireViewCount("one", "v", "key")
		old.state.Running = running
		old.state.Complete = !running
		a.emitRuntimeEvent("messages:readChanged")
		state, _ := a.GetVirtualFolderOnlineCount("one")
		if state.Complete || !old.invalidated || ctx.Err() == nil {
			t.Fatal("mutation retained exact result")
		}
		next, _, fresh, err := a.acquireViewCount("two", "v", "key")
		if err != nil || !fresh || next == old {
			t.Fatal("invalidated scan reused")
		}
		a.invalidateViewCounts() // same hook used by manual RefreshVirtualFolderCache
		a.CancelVirtualFolderOnlineCount("one")
		a.CancelVirtualFolderOnlineCount("two")
	}
}

func TestCountKeyIncludesCriteriaAccountsAndFolderPaths(t *testing.T) {
	vf := &virtualfolder.VirtualFolder{FolderMode: "include", SearchQuery: "from:a", DateWindow: "7d"}
	accounts := []*account.Account{{ID: "a", Email: "a@b", Enabled: true}}
	scope := map[string][]*folder.Folder{"a": {{ID: "f", AccountID: "a", Path: "Inbox"}}}
	base := onlineCountKey(vf, accounts, scope)
	vf.SearchQuery = "from:b"
	if onlineCountKey(vf, accounts, scope) == base {
		t.Fatal("query omitted")
	}
	vf.SearchQuery = "from:a"
	vf.DateWindow = "30d"
	if onlineCountKey(vf, accounts, scope) == base {
		t.Fatal("window omitted")
	}
	vf.DateWindow = "7d"
	vf.ReadStatus = "unread"
	if onlineCountKey(vf, accounts, scope) == base {
		t.Fatal("flags omitted")
	}
	vf.ReadStatus = ""
	accounts[0].Email = "changed"
	if onlineCountKey(vf, accounts, scope) == base {
		t.Fatal("identity omitted")
	}
	accounts[0].Email = "a@b"
	scope["a"][0].Path = "Renamed"
	if onlineCountKey(vf, accounts, scope) == base {
		t.Fatal("folder path omitted")
	}
}

func TestCountMutationEventsAllInvalidate(t *testing.T) {
	for _, event := range []string{"messages:readChanged", "messages:starredChanged", "messages:moved", "messages:deleted", "folders:countsChanged", "folder:synced", "undo:completed"} {
		t.Run(event, func(t *testing.T) {
			a := &App{ctx: context.Background(), runtimeEventEmitter: func(string, ...interface{}) {}}
			old, _, _, _ := a.acquireViewCount("one", "v", "key")
			a.emitRuntimeEvent(event)
			if !old.invalidated {
				t.Fatal("missing invalidation")
			}
			a.CancelVirtualFolderOnlineCount("one")
		})
	}
}

func TestMutationInvalidatesSubscribedEvictedResults(t *testing.T) {
	a := &App{ctx: context.Background()}
	old, _, _, _ := a.acquireViewCount("old", "v", "key")
	old.state.Running = false
	old.state.Complete = true
	old.created = time.Now().Add(-2 * time.Minute)
	_, _, _, _ = a.acquireViewCount("new", "v", "key")
	a.invalidateViewCounts()
	state, _ := a.GetVirtualFolderOnlineCount("old")
	if state.Complete {
		t.Fatal("evicted subscribed result stayed exact after mutation")
	}
	a.CancelVirtualFolderOnlineCount("old")
	a.CancelVirtualFolderOnlineCount("new")
}
