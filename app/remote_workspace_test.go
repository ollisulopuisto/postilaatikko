package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/smtp"
	"github.com/hkdb/aerion/internal/views"
	"github.com/hkdb/aerion/internal/virtualfolder"
)

func TestRemoteWorkspaceCoreReadsWithoutMailboxStores(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/accounts":
			_ = json.NewEncoder(w).Encode(v1.Response[[]v1.AccountSummary]{Data: []v1.AccountSummary{{ID: "account-a", Name: "A", Email: "a@example.test", Enabled: true}}})
		case "/v1/folders":
			_ = json.NewEncoder(w).Encode(v1.Response[[]v1.FolderSummary]{Data: []v1.FolderSummary{{ID: "folder-a", AccountID: "account-a", Name: "Inbox", Path: "INBOX", Type: "inbox"}}})
		case "/v1/views":
			_ = json.NewEncoder(w).Encode(v1.Response[[]*virtualfolder.VirtualFolder]{Data: []*virtualfolder.VirtualFolder{{ID: "view-a", Name: "Saved"}}})
		case "/v1/views/unread-counts":
			_ = json.NewEncoder(w).Encode(v1.Response[map[string]int]{Data: map[string]int{"view-a": 2}})
		case "/v1/views/view-a/cache":
			_ = json.NewEncoder(w).Encode(v1.Response[v1.CachedVirtualFolderView]{Data: v1.CachedVirtualFolderView{VirtualFolderID: "view-a", Conversations: []*message.Conversation{{ThreadID: "thread-a"}}, HasCache: true}})
		case "/v1/conversations":
			_ = json.NewEncoder(w).Encode(v1.Response[v1.ConversationListPage]{Data: v1.ConversationListPage{Items: []*message.Conversation{{ThreadID: "thread-a", AccountID: "account-a", FolderID: "folder-a", Subject: "Hello"}}, Complete: true}})
		case "/v1/conversations/count":
			_ = json.NewEncoder(w).Encode(v1.Response[int]{Data: 9})
		case "/v1/messages/message-a/header":
			_ = json.NewEncoder(w).Encode(v1.Response[message.Message]{Data: message.Message{ID: "message-a", AccountID: "account-a", Subject: "Hello"}})
		case "/v1/messages/body":
			_ = json.NewEncoder(w).Encode(v1.Response[v1.MessageBody]{Data: v1.MessageBody{Ref: v1.MessageRef{AccountID: "account-a", MessageID: "message-a"}, ContentState: views.ContentCached, Original: &v1.OriginalBody{Text: "Remote body"}}})
		case "/v1/threads/read":
			_ = json.NewEncoder(w).Encode(v1.Response[views.ThreadView]{Data: views.ThreadView{Ref: views.ThreadRef{AccountID: "account-a", ThreadID: "thread-a"}, Subject: "Hello", MessageCount: 1, Messages: []views.ThreadMessage{}}})
		default:
			t.Errorf("unexpected remote request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		ctx:               context.Background(),
		remoteServiceMode: true,
		mailboxService: &mailboxServiceConnector{
			client:     automationclient.New(automationclient.Config{Endpoint: server.URL, Token: "device-token"}),
			statusFunc: func(context.Context) MailboxServiceStatus { return MailboxServiceStatus{Connected: true} },
		},
	}
	accounts, err := app.GetAccounts()
	if err != nil || len(accounts) != 1 || accounts[0].ID != "account-a" {
		t.Fatalf("GetAccounts = %#v, %v", accounts, err)
	}
	folders, err := app.GetFolders("account-a")
	if err != nil || len(folders) != 1 || folders[0].ID != "folder-a" {
		t.Fatalf("GetFolders = %#v, %v", folders, err)
	}
	views, err := app.GetVirtualFolders()
	if err != nil || len(views) != 1 || views[0].ID != "view-a" {
		t.Fatalf("GetVirtualFolders = %#v, %v", views, err)
	}
	counts, err := app.GetVirtualFolderUnreadCounts()
	if err != nil || counts["view-a"] != 2 {
		t.Fatalf("GetVirtualFolderUnreadCounts = %#v, %v", counts, err)
	}
	cached, err := app.GetVirtualFolderCachedView("view-a")
	if err != nil || cached == nil || !cached.HasCache || len(cached.Conversations) != 1 {
		t.Fatalf("GetVirtualFolderCachedView = %#v, %v", cached, err)
	}
	conversations, err := app.GetConversations("account-a", "folder-a", 0, 25, "newest", "")
	if err != nil || len(conversations) != 1 || conversations[0].ThreadID != "thread-a" {
		t.Fatalf("GetConversations = %#v, %v", conversations, err)
	}
	count, err := app.GetConversationCount("account-a", "folder-a", "")
	if err != nil || count != 9 {
		t.Fatalf("GetConversationCount = %d, %v", count, err)
	}
	viewCount, err := app.GetVirtualFolderCount("view-a")
	if err != nil || viewCount != 9 {
		t.Fatalf("GetVirtualFolderCount = %d, %v", viewCount, err)
	}
	thread, err := app.GetConversation("thread-a", "folder-a")
	if err != nil || thread.ThreadID != "thread-a" || thread.AccountID != "account-a" {
		t.Fatalf("GetConversation = %#v, %v", thread, err)
	}
	body, err := app.FetchMessageBodyForAccount("account-a", "message-a")
	if err != nil || !body.BodyFetched || body.BodyText != "Remote body" {
		t.Fatalf("FetchMessageBodyForAccount = %#v, %v", body, err)
	}
}

func TestRemotePrepareComposerSendDoesNotNeedLocalComposeStores(t *testing.T) {
	app := &App{remoteServiceMode: true}
	request, err := app.PrepareComposerSend("account-a", smtp.ComposeMessage{
		From:     smtp.Address{Address: "sender@example.test"},
		To:       []smtp.Address{{Address: "recipient@example.test"}},
		Subject:  "Remote compose",
		TextBody: "Hello",
	}, "", "")
	if err != nil {
		t.Fatalf("PrepareComposerSend: %v", err)
	}
	if request.AccountID != "account-a" || request.From != "sender@example.test" || len(request.RFC822) == 0 {
		t.Fatalf("unexpected prepared request: %#v", request)
	}
	if app.db != nil || app.credStore != nil || app.composeOps.accountStore != nil {
		t.Fatal("remote compose initialized local mailbox or credential stores")
	}
}

func TestRemotePrepareComposerSendRejectsDeviceLocalCrypto(t *testing.T) {
	app := &App{remoteServiceMode: true}
	_, err := app.PrepareComposerSend("account-a", smtp.ComposeMessage{
		From:        smtp.Address{Address: "sender@example.test"},
		To:          []smtp.Address{{Address: "recipient@example.test"}},
		SignMessage: true,
	}, "", "")
	if err == nil {
		t.Fatal("expected local-only crypto request to fail explicitly")
	}
}

func TestRemoteAttachmentReadsUseAccountScopedHelper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/messages/attachments/list":
			_ = json.NewEncoder(w).Encode(v1.Response[v1.Page[v1.AttachmentSummary]]{Data: v1.Page[v1.AttachmentSummary]{Items: []v1.AttachmentSummary{
				{ID: "inline-a", Filename: "logo.png", ContentType: "image/png", Size: 4, ContentID: "cid-a", IsInline: true},
				{ID: "file-a", Filename: "report.pdf", ContentType: "application/pdf", Size: 16},
			}, Complete: true}})
		case "/v1/messages/attachments/read":
			var request v1.AttachmentReadRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Ref.AccountID != "account-a" || request.Ref.MessageID != "message-a" || request.AttachmentID != "inline-a" {
				t.Errorf("attachment read ref = %#v", request)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0, 1, 2, 255})
		default:
			t.Errorf("unexpected remote request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		ctx:               context.Background(),
		remoteServiceMode: true,
		mailboxService: &mailboxServiceConnector{
			client: automationclient.New(automationclient.Config{Endpoint: server.URL, Token: "device-token"}),
			statusFunc: func(context.Context) MailboxServiceStatus {
				return MailboxServiceStatus{Connected: true, Capabilities: []string{string(v1.CapabilityAttachmentRead)}}
			},
		},
	}

	attachments, err := app.GetMessageAttachments("account-a", "message-a")
	if err != nil || len(attachments) != 2 || attachments[1].ID != "file-a" {
		t.Fatalf("GetMessageAttachments = %#v, %v", attachments, err)
	}
	inline, err := app.GetInlineAttachmentsForAccount("account-a", "message-a")
	if err != nil || inline["cid-a"] != "data:image/png;base64,AAEC/w==" || len(inline) != 1 {
		t.Fatalf("GetInlineAttachmentsForAccount = %#v, %v", inline, err)
	}
	if app.db != nil || app.attachmentStore != nil || app.messageStore != nil {
		t.Fatal("remote attachment reads initialized local mailbox stores")
	}
}
