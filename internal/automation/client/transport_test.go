package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hkdb/aerion/internal/automation/client"
)

type countingTransport struct {
	calls *atomic.Int64
	base  http.RoundTripper
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return t.base.RoundTrip(req)
}

func TestConfigTransport_OverridesDefault(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"version":"v1","capabilities":[]},"requestId":"req-1"}`))
	}))
	defer server.Close()

	api := client.New(client.Config{
		Endpoint:  server.URL,
		Token:     "test-token",
		Transport: &countingTransport{calls: &calls, base: http.DefaultTransport},
	})
	if _, err := api.GetCapabilities(context.Background()); err != nil {
		t.Fatalf("GetCapabilities: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("custom transport calls = %d, want 1", calls.Load())
	}
}
