package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ollisulopuisto/postilaatikko/internal/automation/client"
)

// TestClientSendsNoRequestID guards PERF-SC-002 coalescing effectiveness:
// the server's read coalescer partitions flights by the caller's
// X-Request-Id, so if this client started sending a unique ID per request,
// concurrent identical desktop reads would stop sharing executions.
func TestClientSendsNoRequestID(t *testing.T) {
	var gotRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"requestId":"req-1"}`))
	}))
	defer server.Close()

	api := client.New(client.Config{Endpoint: server.URL, Token: "test-token"})
	if _, err := api.GetAccounts(context.Background()); err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	if gotRequestID != "" {
		t.Fatalf("client sent X-Request-Id %q; unique per-request IDs defeat read coalescing on the helper", gotRequestID)
	}
}
