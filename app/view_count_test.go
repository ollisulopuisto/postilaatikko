package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
	syncre "github.com/hkdb/aerion/internal/sync"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

func TestOnlineCountJobScopeCompletionAndCancellation(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type,enabled) VALUES('g','Gmail','a@b.c','imap.gmail.com','smtp.gmail.com','a','oauth2',1)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('f','g','Family','Family','folder')`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{ctx: context.Background(), accountStore: account.NewStore(db), folderStore: folder.NewStore(db), virtualFolderStore: virtualfolder.NewStore(db), messageStore: message.NewStore(db), runtimeEventEmitter: func(string, ...interface{}) {}}
	if err = a.virtualFolderStore.Create(&virtualfolder.VirtualFolder{ID: "v", Name: "Family", FolderIDs: []string{"f"}, FolderMode: "include", SearchQuery: "from:friend@example.org"}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/threads" {
			t.Errorf("unexpected content fetch: %s", r.URL)
			w.WriteHeader(500)
			return
		}
		if r.URL.Query().Get("q") == "is:unread" {
			_, _ = w.Write([]byte(`{"threads":[{"id":"one"}]}`))
			return
		}
		if r.URL.Query().Get("q") != `(from:friend@example.org) (label:"Family")` {
			t.Errorf("scope %q", r.URL.Query().Get("q"))
		}
		_, _ = w.Write([]byte(`{"threads":[{"id":"one"},{"id":"two"}],"resultSizeEstimate":500}`))
	}))
	defer srv.Close()
	a.syncEngine = syncre.NewEngine(nil, a.accountStore, a.folderStore, message.NewStore(db), message.NewAttachmentStore(db))
	a.syncEngine.SetGmailTokenSource(func(context.Context, string, bool) (string, error) { return "token", nil })
	a.syncEngine.SetGmailClientFactory(func(source gmail.TokenSource) *gmail.Client {
		c := gmail.NewClient(source)
		c.BaseURL = srv.URL
		return c
	})
	if err = a.StartVirtualFolderOnlineCount("window", "v"); err != nil {
		t.Fatal(err)
	}
	defer a.CancelVirtualFolderOnlineCount("window")
	deadline := time.Now().Add(3 * time.Second)
	for {
		state, err := a.GetVirtualFolderOnlineCount("window")
		if err != nil {
			t.Fatal(err)
		}
		if !state.Running {
			if !state.Complete || state.Total != 2 || state.Unread != 1 || len(state.Issues) != 0 {
				t.Fatalf("count %+v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("count did not complete")
		}
		time.Sleep(time.Millisecond)
	}

	// Completed exact results survive window closure and are shared, not rescanned.
	before := calls.Load()
	if err = a.StartVirtualFolderOnlineCount("second", "v"); err != nil {
		t.Fatal(err)
	}
	state, err := a.GetVirtualFolderOnlineCount("second")
	if err != nil || !state.Complete || state.Total != 2 || state.SessionID != "second" || calls.Load() != before {
		t.Fatal("completed scan not reused", state, err)
	}
	a.CancelVirtualFolderOnlineCount("window")
	state, err = a.GetVirtualFolderOnlineCount("second")
	if err != nil || !state.Complete {
		t.Fatal("closing first subscriber lost result")
	}
	a.CancelVirtualFolderOnlineCount("second")
	// Manual refresh has to bypass a completed shared result.
	if err = a.RefreshVirtualFolderCache("v"); err != nil {
		t.Fatal(err)
	}
	if err = a.StartVirtualFolderOnlineCount("window", "v"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		state, _ = a.GetVirtualFolderOnlineCount("window")
		if !state.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fresh scan timed out")
		}
		time.Sleep(time.Millisecond)
	}
	if !state.Complete || calls.Load() <= before {
		t.Fatal("manual refresh reused stale result")
	}

	// Adding an unsupported provider must preserve Gmail's result while reporting
	// the whole view incomplete. A disabled account must not create an issue.
	for _, stmt := range []string{
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type,enabled) VALUES('other','Other','o@example.org','example.org','example.org','o','password',1)`,
		`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type,enabled) VALUES('disabled','Disabled','d@example.org','example.org','example.org','d','password',0)`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('other-folder','other','Inbox','INBOX','inbox')`,
		`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES('disabled-folder','disabled','Inbox','INBOX','inbox')`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err = a.virtualFolderStore.Update(&virtualfolder.VirtualFolder{ID: "v", Name: "Mixed", FolderIDs: []string{"f", "other-folder", "disabled-folder"}, FolderMode: "include", SearchQuery: "from:friend@example.org"}); err != nil {
		t.Fatal(err)
	}
	if err = a.StartVirtualFolderOnlineCount("mixed", "v"); err != nil {
		t.Fatal(err)
	}
	defer a.CancelVirtualFolderOnlineCount("mixed")
	deadline = time.Now().Add(3 * time.Second)
	for {
		state, err := a.GetVirtualFolderOnlineCount("mixed")
		if err != nil {
			t.Fatal(err)
		}
		if !state.Running {
			if state.Complete || state.Total != 2 || state.Unread != 1 || len(state.Issues) != 1 || state.Issues[0].AccountID != "other" {
				t.Fatalf("unsupported account claimed complete or lost successful counts: %+v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mixed count did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	a.CancelVirtualFolderOnlineCount("window")
	if _, err = a.GetVirtualFolderOnlineCount("window"); err == nil {
		t.Fatal("cancelled state retained")
	}
	if err = a.StartVirtualFolderOnlineCount("window", "missing"); err == nil {
		t.Fatal("missing virtual folder accepted")
	}
}
