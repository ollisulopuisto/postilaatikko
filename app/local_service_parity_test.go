package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/account"
	"github.com/ollisulopuisto/postilaatikko/internal/alwayson"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/database"
	"github.com/ollisulopuisto/postilaatikko/internal/folder"
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"github.com/ollisulopuisto/postilaatikko/internal/views"
	"github.com/ollisulopuisto/postilaatikko/internal/virtualfolder"
)

// TestLocalModeMatchesHelperConversationRoutes guards against the desktop's
// local mode and the helper's HTTP routes drifting apart. The same fixture is
// read through the desktop App (no mailbox service) and through the real
// /v1/conversations and /v1/conversations/count routes over the same stores;
// every window, scope, filter and order must produce identical results.
func TestLocalModeMatchesHelperConversationRoutes(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "parity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"INSERT INTO accounts (id,name,email,imap_host,smtp_host,username) VALUES ('a','a','a@test','imap','smtp','a'),('b','b','b@test','imap','smtp','b')",
		"INSERT INTO folders (id,account_id,name,path,folder_type) VALUES ('a-in','a','INBOX','INBOX','inbox'),('a-sent','a','Sent','Sent','sent'),('a-all','a','All Mail','[Gmail]/All Mail','all'),('b-in','b','INBOX','INBOX','inbox'),('b-arch','b','Archive','Archive','archive')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	messages := message.NewStore(db)
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	folders := []struct{ id, account string }{{"a-in", "a"}, {"b-in", "b"}, {"a-sent", "a"}, {"a-all", "a"}, {"b-arch", "b"}}
	n := 0
	for i := 0; i < 180; i++ {
		f := folders[i%len(folders)]
		// Pairs share a timestamp, and account a's Gmail All Mail (virtual
		// archive) ties with account b's physical Archive, to exercise
		// cross-source tie-breaking in unified scopes.
		date := base.Add(time.Duration(i/2) * time.Hour)
		if f.id == "a-all" || f.id == "b-arch" {
			date = base.Add(time.Duration(i/5) * 5 * time.Hour)
		}
		m := &message.Message{
			ID: fmt.Sprintf("m%d", i), AccountID: f.account, FolderID: f.id, UID: uint32(i + 1),
			MessageID: fmt.Sprintf("<m%d@test>", i), ThreadID: fmt.Sprintf("<t%d@test>", i/3),
			Subject: fmt.Sprintf("s%d", i/3), FromEmail: "x@test", Date: date,
			IsRead: i%4 != 0, IsStarred: i%7 == 0,
		}
		if err := messages.Create(m); err != nil {
			t.Fatal(err)
		}
		n++
	}
	accounts := account.NewStore(db)
	folderStore := folder.NewStore(db)
	vfs := virtualfolder.NewStore(db)
	app := &App{accountStore: accounts, folderStore: folderStore, messageStore: messages, virtualFolderStore: vfs}

	ops := alwayson.NewConversationReadOperations(accounts, folderStore, vfs, messages)
	mux := http.NewServeMux()
	allow := func(http.ResponseWriter, *http.Request, v1.Capability) bool { return true }
	v1.MountConversationListRoute(mux, ops, allow)
	v1.MountConversationCountRoute(mux, ops, allow)
	server := httptest.NewServer(mux)
	defer server.Close()
	post := func(path string, body any, out any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		resp, err := http.Post(server.URL+path, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: HTTP %d", path, resp.StatusCode)
		}
		envelope := struct {
			Data json.RawMessage `json:"data"`
		}{}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			t.Fatal(err)
		}
	}
	helperWindow := func(scope views.Scope, offset, limit int, sortOrder, filter string) []*message.Conversation {
		items, err := pageConversationWindow(func(req v1.ConversationListRequest) (*v1.ConversationListPage, error) {
			var page v1.ConversationListPage
			post("/v1/conversations", req, &page)
			return &page, nil
		}, scope, offset, limit, sortOrder, filter)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	key := func(items []*message.Conversation) []string {
		out := make([]string, 0, len(items))
		for _, c := range items {
			out = append(out, fmt.Sprintf("%s|%s|%d|%d|%s", c.AccountID, c.ThreadID, c.MessageCount, c.UnreadCount, c.LatestDate.UTC().Format(time.RFC3339)))
		}
		return out
	}
	rowsCompared := 0
	same := func(label string, desktop, helper []*message.Conversation) {
		t.Helper()
		d, h := key(desktop), key(helper)
		if fmt.Sprint(d) != fmt.Sprint(h) {
			t.Fatalf("%s drifted:\n desktop %v\n helper  %v", label, d, h)
		}
		rowsCompared += len(d)
	}

	for _, window := range [][2]int{{0, 7}, {0, 40}, {5, 9}, {20, 30}} {
		for _, order := range []string{"newest", "oldest"} {
			for _, filter := range []string{"", "unread", "starred"} {
				label := fmt.Sprintf("window=%v order=%s filter=%q", window, order, filter)
				got, err := app.GetConversations("a", "a-in", window[0], window[1], order, filter)
				if err != nil {
					t.Fatal(err)
				}
				same("folder "+label, got, helperWindow(views.Scope{Kind: views.ScopeFolder, AccountID: "a", FolderID: "a-in"}, window[0], window[1], order, filter))
				got, err = app.GetUnifiedFolderConversations("inbox", window[0], window[1], order, filter)
				if err != nil {
					t.Fatal(err)
				}
				same("all inboxes "+label, got, helperWindow(views.Scope{Kind: views.ScopeAllInboxes}, window[0], window[1], order, filter))
				for _, unified := range []string{"sent", "archive"} {
					got, err = app.GetUnifiedFolderConversations(unified, window[0], window[1], order, filter)
					if err != nil {
						t.Fatal(err)
					}
					same(unified+" "+label, got, helperWindow(views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: unified}, window[0], window[1], order, filter))
				}
			}
		}
	}

	if rowsCompared < 500 {
		t.Fatalf("fixture compared only %d rows", rowsCompared)
	}

	for _, filter := range []string{"", "unread"} {
		var helperCount int
		post("/v1/conversations/count", v1.ConversationCountRequest{Scope: views.Scope{Kind: views.ScopeFolder, AccountID: "a", FolderID: "a-in"}, Filter: filter}, &helperCount)
		if got, err := app.GetConversationCount("a", "a-in", filter); err != nil || got != helperCount || got == 0 {
			t.Fatalf("folder count filter=%q: desktop %d (%v), helper %d", filter, got, err, helperCount)
		}
		post("/v1/conversations/count", v1.ConversationCountRequest{Scope: views.Scope{Kind: views.ScopeAllInboxes}, Filter: filter}, &helperCount)
		if got, err := app.GetUnifiedFolderCount("inbox", filter); err != nil || got != helperCount || got == 0 {
			t.Fatalf("all-inboxes count filter=%q: desktop %d (%v), helper %d", filter, got, err, helperCount)
		}
		if got, err := app.GetUnifiedInboxCount(filter); err != nil || got != helperCount {
			t.Fatalf("unified inbox count filter=%q: desktop %d (%v), helper %d", filter, got, err, helperCount)
		}
		post("/v1/conversations/count", v1.ConversationCountRequest{Scope: views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: "archive"}, Filter: filter}, &helperCount)
		if got, err := app.GetUnifiedFolderCount("archive", filter); err != nil || got != helperCount || got == 0 {
			t.Fatalf("archive count filter=%q: desktop %d (%v), helper %d", filter, got, err, helperCount)
		}
	}
}
