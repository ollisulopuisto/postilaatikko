package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
)

func TestGmailPublicationClientNeverReplaysFailedCreate(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/v1/filters/gmail" {
			t.Error("wrong route")
		}
		var req v1.GmailPublicationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.AccountID != "helper-owned" || req.ID != "plan" || req.Fingerprint != "review" {
			t.Error("lost reviewed identity", req)
		}
		w.Header().Set("Content-Type", "application/json")
		if req.Operation == "publish" {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"unknown create"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"accountId":"helper-owned","publication":{"id":"plan","state":"published","providerId":"remote"}},"requestId":"req"}`))
	}))
	defer server.Close()
	api := client.New(client.Config{Endpoint: server.URL, Token: "api-grant"})
	req := v1.GmailPublicationRequest{Operation: "publish", AccountID: "helper-owned", ID: "plan", Fingerprint: "review"}
	if _, err := api.GmailPublication(context.Background(), req); err == nil || calls != 1 {
		t.Fatal("failed create replayed", calls, err)
	}
	req.Operation = "reconcile"
	result, err := api.GmailPublication(context.Background(), req)
	if err != nil || calls != 2 || result.Publication == nil || result.Publication.ProviderID != "remote" {
		t.Fatal(result, calls, err)
	}
}
