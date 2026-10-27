package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hkdb/aerion/internal/account"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/gmail"
	"github.com/hkdb/aerion/internal/message"
	syncengine "github.com/hkdb/aerion/internal/sync"
	"github.com/hkdb/aerion/internal/undo"
	"github.com/hkdb/aerion/internal/virtualfolder"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGmailSearchAcrossAccountsAndVirtualFolderFromEmptyCache(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if _, err = db.Exec(`INSERT INTO accounts(id,name,email,imap_host,smtp_host,username,auth_type,enabled) VALUES(?,?,?,'imap.gmail.com','smtp.gmail.com',?,'oauth2',1)`, id, id, id+"@example.org", id); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"1", "2"} {
			if _, err = db.Exec(`INSERT INTO folders(id,account_id,name,path,folder_type) VALUES(?,?,?,?, 'folder')`, id+n, id, "Folder "+n, "Folder "+n); err != nil {
				t.Fatal(err)
			}
		}
	}
	var bodyCalls, modifies, metadataCalls atomic.Int32
	var failB, quotaThreads atomic.Bool
	header := func(id string) map[string]any {
		return map[string]any{"id": id, "threadId": id, "internalDate": fmt.Sprint(time.Now().UnixMilli()), "labelIds": []string{"Label_1", "UNREAD"}, "payload": map[string]any{"headers": []map[string]string{{"name": "Subject", "value": "Remote only"}, {"name": "From", "value": "peer@example.org"}, {"name": "Message-ID", "value": "<" + id + "@example.org>"}}}}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failB.Load() && r.Header.Get("Authorization") == "Bearer b" {
			w.WriteHeader(401)
			return
		}
		if quotaThreads.Load() && strings.HasPrefix(r.URL.Path, "/threads/") {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"rateLimitExceeded"}],"message":"quota"}}`))
			return
		}
		switch {
		case r.URL.Path == "/labels":
			_, _ = w.Write([]byte(`{"labels":[{"id":"Label_1","name":"Folder 1"},{"id":"Label_2","name":"Folder 2"}]}`))
		case r.URL.Path == "/messages" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"copied"}`))
		case r.URL.Path == "/messages" || r.URL.Path == "/threads":
			id, next := "same", ""
			if r.Header.Get("Authorization") == "Bearer a" && r.URL.Query().Get("pageToken") == "" {
				next = "next"
			}
			if r.URL.Query().Get("pageToken") == "next" {
				id = "second"
			}
			key := "messages"
			if r.URL.Path == "/threads" {
				key = "threads"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{key: []map[string]string{{"id": id}}, "nextPageToken": next})
		case strings.HasSuffix(r.URL.Path, "/modify"):
			modifies.Add(1)
			_, _ = w.Write([]byte(`{}`))
		case strings.HasPrefix(r.URL.Path, "/messages/"):
			if r.URL.Query().Get("format") == "full" {
				bodyCalls.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"payload": map[string]any{"mimeType": "text/plain", "body": map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte("Requested content"))}}})
				return
			}
			if r.URL.Query().Get("format") == "raw" {
				bodyCalls.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]string{"raw": base64.RawURLEncoding.EncodeToString([]byte("From: peer@example.org\r\nMessage-ID: <same@example.org>\r\n\r\nContent"))})
				return
			}
			metadataCalls.Add(1)
			_ = json.NewEncoder(w).Encode(header(strings.TrimPrefix(r.URL.Path, "/messages/")))
		case strings.HasPrefix(r.URL.Path, "/threads/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": []any{header(strings.TrimPrefix(r.URL.Path, "/threads/"))}})
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	a := &App{ctx: context.Background(), accountStore: account.NewStore(db), folderStore: folder.NewStore(db), messageStore: message.NewStore(db), virtualFolderStore: virtualfolder.NewStore(db), undoStack: undo.NewStack(10, time.Minute), runtimeEventEmitter: func(string, ...interface{}) {}}
	a.syncEngine = syncengine.NewEngine(nil, a.accountStore, a.folderStore, a.messageStore, message.NewAttachmentStore(db))
	a.syncEngine.SetGmailTokenSource(func(_ context.Context, id string, _ bool) (string, error) { return id, nil })
	a.syncEngine.SetGmailClientFactory(func(src gmail.TokenSource) *gmail.Client { c := gmail.NewClient(src); c.BaseURL = server.URL; return c })
	page, err := a.SearchMail(MailSearchRequest{Query: "is:unread", Limit: 10})
	if err != nil || len(page.Issues) > 0 || len(page.Conversations) != 2 || page.Cursor == "" {
		t.Fatalf("first page %+v %v", page, err)
	}
	if page.Conversations[0].ThreadID == page.Conversations[1].ThreadID {
		t.Fatal("cross-account thread identity collision")
	}
	next, err := a.SearchMail(MailSearchRequest{Query: "is:unread", Limit: 10, Cursor: page.Cursor})
	if err != nil || len(next.Conversations) != 1 || next.Cursor != "" {
		t.Fatalf("next page %+v %v", next, err)
	}
	if _, err = a.SearchMail(MailSearchRequest{Query: "different", Cursor: page.Cursor}); err == nil {
		t.Fatal("cursor accepted under a different query")
	}
	vf := &virtualfolder.VirtualFolder{Name: "Both accounts", FolderIDs: []string{"a1", "a2", "b1"}, ReadStatus: "unread"}
	if err = a.virtualFolderStore.Create(vf); err != nil {
		t.Fatal(err)
	}
	virtual, err := a.SearchMail(MailSearchRequest{VirtualFolderID: vf.ID})
	if err != nil || len(virtual.Issues) > 0 || len(virtual.Conversations) != 2 {
		t.Fatalf("virtual page %+v %v", virtual, err)
	}
	failB.Store(true)
	partial, err := a.SearchMail(MailSearchRequest{Query: "is:unread"})
	if err != nil || len(partial.Conversations) != 1 || len(partial.Issues) != 1 || !partial.Issues[0].Reauthorize {
		t.Fatalf("one account hid another: %+v %v", partial, err)
	}
	failB.Store(false)
	if metadataCalls.Load() != 0 {
		t.Fatalf("search duplicated message and thread metadata: %d message gets", metadataCalls.Load())
	}
	if bodyCalls.Load() != 0 {
		t.Fatal("search fetched bodies")
	}
	var chosen *message.Conversation
	for _, c := range page.Conversations {
		if c.AccountID == "a" {
			chosen = c
		}
	}
	opened, err := a.messageStore.Get(chosen.MessageIDs[0])
	if err != nil || opened.BodyFetched {
		t.Fatalf("search-created message is not header-only: %+v %v", opened, err)
	}
	opened, err = a.FetchMessageBody(opened.ID)
	if err != nil || !opened.BodyFetched || opened.BodyText != "Requested content" {
		t.Fatalf("on-demand body unavailable: %+v %v", opened, err)
	}
	quotaThreads.Store(true)
	cached, err := a.GetConversation(chosen.ThreadID, chosen.FolderID)
	if err != nil || cached == nil || len(cached.Messages) == 0 || cached.Messages[0].BodyText != "Requested content" {
		t.Fatalf("quota hid cached reader: %+v %v", cached, err)
	}
	quotaThreads.Store(false)
	operation, err := a.moveToFolder(chosen.MessageIDs, "a2", true, "")
	if err != nil || operation == "" {
		t.Fatalf("native move failed %s %v", operation, err)
	}
	if _, err = a.UndoOperation(operation); err != nil {
		t.Fatal(err)
	}
	moved, err := a.messageStore.Get(chosen.MessageIDs[0])
	if err != nil || moved.FolderID != "a1" || modifies.Load() != 2 {
		t.Fatalf("native move/undo: %+v %v (%d requests)", moved, err, modifies.Load())
	}
	_, err = a.moveToFolder(chosen.MessageIDs, "b2", true, "")
	if err == nil {
		t.Fatal("unconfirmed destination copy was accepted")
	}
	source, err := a.messageStore.Get(chosen.MessageIDs[0])
	if err != nil || source == nil || source.FolderID != "a1" || modifies.Load() != 2 {
		t.Fatalf("unconfirmed copy changed the source: %+v %v", source, err)
	}

}
