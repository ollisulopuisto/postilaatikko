package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
)

func TestBootstrapWorkspaceParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspace/bootstrap" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"accounts":[{"id":"acc1"}],"folders":[],"views":[],"unreadCounts":{"vf1":2},"initialPage":{"items":[],"complete":true}},"requestId":"req-1"}`))
	}))
	defer server.Close()

	api := client.New(client.Config{Endpoint: server.URL, Token: "test-token"})
	resp, err := api.BootstrapWorkspace(context.Background(), v1.WorkspaceBootstrapRequest{
		InitialPage: &v1.ConversationListRequest{Page: v1.PageRequest{Limit: 10}},
	})
	if err != nil {
		t.Fatalf("BootstrapWorkspace: %v", err)
	}
	if len(resp.Accounts) != 1 || resp.Accounts[0].ID != "acc1" {
		t.Fatalf("accounts = %+v", resp.Accounts)
	}
	if resp.UnreadCounts["vf1"] != 2 {
		t.Fatalf("unreadCounts = %+v", resp.UnreadCounts)
	}
	if resp.InitialPage == nil || !resp.InitialPage.Complete {
		t.Fatalf("initialPage = %+v", resp.InitialPage)
	}
}

func TestBootstrapWorkspaceMissingRouteIsDetectable(t *testing.T) {
	// An older helper without the bootstrap route answers with a bare 404.
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	api := client.New(client.Config{Endpoint: server.URL, Token: "test-token"})
	_, err := api.BootstrapWorkspace(context.Background(), v1.WorkspaceBootstrapRequest{})
	if err == nil {
		t.Fatal("expected an error for a missing route")
	}
	if !client.IsHTTPNotFound(err) {
		t.Fatalf("IsHTTPNotFound(%v) = false, want true so callers can fall back to separate reads", err)
	}
}
