package app

import (
	"context"
	"testing"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/message"
	"github.com/hkdb/aerion/internal/views"
)

func TestSearchMailViaServiceUsesVirtualFolderScope(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantKind   views.ScopeKind
		wantWithin bool
	}{
		{name: "list saved view", wantKind: views.ScopeVirtualFolder},
		{name: "search inside saved view", query: "from:alice@example.com", wantKind: views.ScopeSearch, wantWithin: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request v1.SearchRequest
			connector := &mailboxServiceConnector{
				statusFunc: func(context.Context) MailboxServiceStatus {
					return MailboxServiceStatus{Connected: true}
				},
				searchFunc: func(_ context.Context, _ string, req v1.SearchRequest) (*views.ThreadPage, error) {
					request = req
					return &views.ThreadPage{Items: []views.ThreadSummary{}, Complete: true}, nil
				},
			}
			app := &App{ctx: context.Background(), mailboxService: connector}
			page, handled, err := app.searchMailViaService(MailSearchRequest{
				SessionID: "session-1", Query: test.query, VirtualFolderID: "view-1",
				Cursor: "cursor-1", Limit: 25, Filter: "unread",
			})
			if err != nil {
				t.Fatalf("searchMailViaService: %v", err)
			}
			if !handled {
				t.Fatal("virtual-folder search was not handled by the service")
			}
			if page == nil || len(page.Conversations) != 0 {
				t.Fatalf("page = %#v, want an empty page", page)
			}
			if request.Scope.Kind != test.wantKind {
				t.Fatalf("scope kind = %q, want %q", request.Scope.Kind, test.wantKind)
			}
			if test.wantWithin {
				if request.Scope.Query != test.query || request.Scope.Within == nil || request.Scope.Within.Kind != views.ScopeVirtualFolder || request.Scope.Within.VirtualFolderID != "view-1" {
					t.Fatalf("search scope = %#v, want query within virtual folder view-1", request.Scope)
				}
			} else if request.Scope.VirtualFolderID != "view-1" {
				t.Fatalf("virtual-folder scope = %#v, want view-1", request.Scope)
			}
			if request.Filter != "unread" || request.Page.Cursor != "cursor-1" || request.Page.Limit != 25 {
				t.Fatalf("request paging/filter = %#v, want filter unread, cursor cursor-1, limit 25", request)
			}
		})
	}
}

func TestGetVirtualFolderCachedViewReadsConnectedHelper(t *testing.T) {
	called := false
	connector := &mailboxServiceConnector{
		statusFunc: func(context.Context) MailboxServiceStatus {
			return MailboxServiceStatus{Connected: true}
		},
		cachedViewFunc: func(_ context.Context, viewID string) (*v1.CachedVirtualFolderView, error) {
			called = true
			if viewID != "view-remote" {
				t.Fatalf("view ID = %q, want view-remote", viewID)
			}
			return &v1.CachedVirtualFolderView{
				VirtualFolderID: viewID,
				Conversations:   []*message.Conversation{{ThreadID: "thread-remote", Subject: "Cached remotely"}},
				HasCache:        true,
			}, nil
		},
	}
	app := &App{ctx: context.Background(), mailboxService: connector}
	view, err := app.GetVirtualFolderCachedView("view-remote")
	if err != nil {
		t.Fatalf("GetVirtualFolderCachedView: %v", err)
	}
	if !called {
		t.Fatal("connected helper cache was not read")
	}
	if view == nil || !view.HasCache || len(view.Conversations) != 1 || view.Conversations[0].Subject != "Cached remotely" {
		t.Fatalf("view = %#v, want the hydrated helper cache", view)
	}
}
