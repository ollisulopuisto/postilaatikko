package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	automationclient "github.com/ollisulopuisto/postilaatikko/internal/automation/client"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
	"github.com/ollisulopuisto/postilaatikko/internal/operations"
)

// A mutation must reach the endpoint the desktop is connected to. It used to be
// sent to the default loopback helper, so remote archive and other actions never
// arrived while remote reads worked.
func TestMutateActionUsesConfiguredEndpoint(t *testing.T) {
	var mu sync.Mutex
	var gotPath, gotAuth, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath, gotAuth, gotKey = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{},"requestId":"req-test"}`))
	}))
	defer server.Close()

	connector := &mailboxServiceConnector{
		endpoint:   server.URL,
		client:     automationclient.New(automationclient.Config{Endpoint: mailboxServiceEndpoint, Token: "remote-token"}),
		credential: &desktopMailboxCredential{Token: "remote-token"},
	}
	_, err := connector.mutateAction(context.Background(), v1.ActionRequest{
		Action: v1.ActionArchive,
		Target: operations.Target{Kind: operations.TargetMatchedMessages, Messages: []operations.MessageRef{{AccountID: "a", MessageID: "m"}}},
	})
	if err != nil {
		t.Fatalf("mutateAction() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/v1/actions" {
		t.Fatalf("remote endpoint got path %q, want /v1/actions", gotPath)
	}
	if gotAuth != "Bearer remote-token" {
		t.Fatalf("remote endpoint got Authorization %q, want the remote token", gotAuth)
	}
	if gotKey == "" {
		t.Fatal("remote endpoint got no Idempotency-Key")
	}
}
