package app

import (
	"context"
	"encoding/json"
	"github.com/hkdb/aerion/internal/appstate"
	"github.com/hkdb/aerion/internal/database"
	"path/filepath"
	"testing"

	"github.com/hkdb/aerion/internal/folder"
	"github.com/hkdb/aerion/internal/ipc"
)

func TestMailWindowAllowlist(t *testing.T) {
	methods := (&App{}).mailWindowMethods()
	for _, name := range []string{"GetConversations", "GetConversation", "GetFolderTree", "SearchMail", "GetVirtualFolders", "RefreshViewHeaders", "OpenComposerWindow", "OpenReaderWindow", "OpenMailWindow", "MarkAsRead", "ArchiveWithUndo"} {
		if _, ok := methods[name]; !ok {
			t.Errorf("mail operation unavailable: %s", name)
		}
	}
	for _, name := range []string{"SaveUIState", "SetMessageListSortOrder", "RemoveAccount", "UpdateAccount", "QuitApp", "InitializeSendLaterClient", "SendMessageWithDraftGuard"} {
		if _, ok := methods[name]; ok {
			t.Errorf("child may change shared state through %s", name)
		}
	}
}

func TestMailWindowFolderAccountIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, accountID string
		folder          *folder.Folder
		wantErr         bool
	}{
		{"correct account", "a", &folder.Folder{ID: "inbox", AccountID: "a"}, false},
		{"other account", "b", &folder.Folder{ID: "inbox", AccountID: "a"}, true},
		{"missing folder", "a", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMailWindowFolder(tc.accountID, tc.folder)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMailWindowCallUsesSeparateAuthenticatedRoute(t *testing.T) {
	client := &readerTestClient{response: ipc.Message{Payload: json.RawMessage(`[]`)}}
	child := NewReaderApp(ReaderConfig{MailWindow: true, AccountID: "a", FolderID: "inbox"})
	child.ctx = context.Background()
	child.client = client
	close(child.ready)
	if _, err := child.Call("GetConversations", json.RawMessage(`["inbox",0,50,"newest"]`)); err != nil {
		t.Fatal(err)
	}
	if client.request.Type != mailWindowCallType {
		t.Fatalf("route = %s", client.request.Type)
	}
	var request struct {
		Operation string
		Args      json.RawMessage
	}
	if err := client.request.ParsePayload(&request); err != nil {
		t.Fatal(err)
	}
	if request.Operation != "GetConversations" || string(request.Args) != `["inbox",0,50,"newest"]` {
		t.Fatalf("request = %+v", request)
	}
}

func TestMailWindowEventBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []any
		wantErr bool
	}{
		{"normal", []any{map[string]any{"messageIds": []string{"m1"}, "isRead": true}}, false},
		{"oversize", []any{string(make([]byte, 1<<20))}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newMailWindowEvent("messages:readChanged", tc.data)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if _, err := newMailWindowEvent("oauth:token", nil); err == nil {
		t.Fatal("unexpected event forwarded")
	}
}

type mailWindowTestServer struct {
	ipc.Server
	reply ipc.Message
}

func (s *mailWindowTestServer) Send(_ string, msg ipc.Message) error { s.reply = msg; return nil }

func TestMailWindowParentRejectsPrimaryStateWrites(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	store := appstate.NewStore(db)
	if err := store.SaveUIState(&appstate.UIState{SelectedFolderID: "primary-inbox", ListWidth: 420}); err != nil {
		t.Fatal(err)
	}
	server := &mailWindowTestServer{}
	parent := &App{appStateStore: store, ipcServer: server}
	for _, tc := range []struct {
		operation string
		args      string
		wantErr   bool
	}{
		{"SaveUIState", `[{"selectedFolderId":"child-sent","listWidth":600}]`, true},
		{"SetMessageListSortOrder", `["oldest"]`, true},
		{"GetUIState", `["unexpected"]`, true},
		{"GetUIState", `[]`, false},
	} {
		t.Run(tc.operation+tc.args, func(t *testing.T) {
			request, err := ipc.NewMessage(mailWindowCallType, map[string]any{"operation": tc.operation, "args": json.RawMessage(tc.args)})
			if err != nil {
				t.Fatal(err)
			}
			parent.handleReaderCall("child", request)
			if (server.reply.Error != "") != tc.wantErr {
				t.Fatalf("response = %+v", server.reply)
			}
			if server.reply.ReplyTo != request.ID || server.reply.Type != mailWindowCallType {
				t.Fatal("lost request identity")
			}
			if !tc.wantErr {
				var got appstate.UIState
				if err := json.Unmarshal(server.reply.Payload, &got); err != nil {
					t.Fatal(err)
				}
				if got.SelectedFolderID != "primary-inbox" {
					t.Fatalf("state = %+v", got)
				}
			}
		})
	}
	got, err := store.GetUIState()
	if err != nil || got.SelectedFolderID != "primary-inbox" || got.ListWidth != 420 {
		t.Fatalf("primary state changed: %+v, %v", got, err)
	}
}
