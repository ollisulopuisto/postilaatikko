package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/views"
)

type conversationCountOperationsFunc func(ConversationCountRequest) (int, error)

func (f conversationCountOperationsFunc) CountConversationsByFolder(accountID, folderID, filter string) (int, error) {
	return f(ConversationCountRequest{Scope: views.Scope{Kind: views.ScopeFolder, AccountID: accountID, FolderID: folderID}, Filter: filter})
}

func (f conversationCountOperationsFunc) CountConversationsUnifiedInbox(filter string) (int, error) {
	return f(ConversationCountRequest{Scope: views.Scope{Kind: views.ScopeAllInboxes}, Filter: filter})
}

func (f conversationCountOperationsFunc) CountConversationsUnifiedFolder(folderType, filter string) (int, error) {
	return f(ConversationCountRequest{Scope: views.Scope{Kind: views.ScopeUnifiedFolder, FolderType: folderType}, Filter: filter})
}

func (f conversationCountOperationsFunc) CountConversationsVirtualFolder(virtualFolderID string) (int, error) {
	return f(ConversationCountRequest{Scope: views.Scope{Kind: views.ScopeVirtualFolder, VirtualFolderID: virtualFolderID}})
}

func TestMountConversationCountRouteReturnsFilteredServiceCount(t *testing.T) {
	var requested Capability
	mux := http.NewServeMux()
	MountConversationCountRoute(mux, conversationCountOperationsFunc(func(req ConversationCountRequest) (int, error) {
		if req.Scope.Kind != views.ScopeFolder || req.Scope.AccountID != "account-a" || req.Scope.FolderID != "folder-a" || req.Filter != "unread" {
			t.Fatalf("count request = %+v", req)
		}
		return 7, nil
	}), func(_ http.ResponseWriter, _ *http.Request, capability Capability) bool {
		requested = capability
		return true
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/conversations/count", strings.NewReader(`{"scope":{"kind":"folder","accountId":"account-a","folderId":"folder-a"},"filter":"unread"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || requested != CapabilityMailRead {
		t.Fatalf("status=%d capability=%q", response.Code, requested)
	}
	var result Response[int]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data != 7 {
		t.Fatalf("count = %d, want 7", result.Data)
	}
}

func TestMountConversationCountRouteRejectsInvalidScope(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	MountConversationCountRoute(mux, conversationCountOperationsFunc(func(ConversationCountRequest) (int, error) {
		called = true
		return 0, nil
	}), func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	request := httptest.NewRequest(http.MethodPost, "/v1/conversations/count", strings.NewReader(`{"scope":{"kind":"all_accounts"}}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("status=%d called=%v, want bad request without operation", response.Code, called)
	}
}
