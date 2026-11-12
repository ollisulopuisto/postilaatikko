package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/platform"
	"github.com/hkdb/aerion/internal/smtp"
)

func TestRemotePendingQueueWithoutMailboxDatabase(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var submits int
	var uncertain bool
	var receiptSent bool
	var requestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/send" {
			submits++
			requestID = r.Header.Get("Idempotency-Key")
			var request v1.SendRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if uncertain {
				http.Error(w, "transport failure", http.StatusBadGateway)
				return
			}
			if request.AccountID != "a" || len(request.RFC822) == 0 {
				t.Error("missing prepared message")
			}
			_ = json.NewEncoder(w).Encode(v1.Response[v1.SendResponse]{Data: v1.SendResponse{Receipt: v1.OperationReceipt{State: v1.OperationSent}}})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/send/receipts/") {
			if receiptSent {
				_ = json.NewEncoder(w).Encode(v1.Response[v1.SendReceiptLookup]{Data: v1.SendReceiptLookup{Found: true, Response: &v1.SendResponse{Receipt: v1.OperationReceipt{State: v1.OperationSent}}}})
				return
			}
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		t.Errorf("unexpected request: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	a := &App{ctx: context.Background(), deviceDB: db, remoteServiceMode: true, remoteServiceEndpoint: server.URL, paths: &platform.Paths{Data: t.TempDir()}, runtimeEventEmitter: func(string, ...interface{}) {}, mailboxService: &mailboxServiceConnector{statusFunc: func(context.Context) MailboxServiceStatus {
		return MailboxServiceStatus{Connected: true, Capabilities: []string{"mail.read", "mail.body.fetch", "mail.send", "draft.read", "draft.write"}}
	}, client: automationclient.New(automationclient.Config{Endpoint: server.URL, Token: "test"})}}
	msg := smtp.ComposeMessage{From: smtp.Address{Address: "a@example.test"}, To: []smtp.Address{{Address: "b@example.test"}}, TextBody: "immutable"}
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	id, err := a.QueuePendingSend("a", "", string(payload))
	if err != nil {
		t.Fatal(err)
	}
	if a.db != nil || a.credStore != nil || a.syncEngine != nil {
		t.Fatal("opened mailbox dependencies")
	}
	list, err := a.ListPendingSends()
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	handled, err := a.DispatchRemotePendingSend(id)
	if !handled || err != nil {
		t.Fatalf("dispatch=%v %v", handled, err)
	}
	if submits != 1 || requestID != id {
		t.Fatalf("submits=%d requestID=%s", submits, requestID)
	}
	if _, err := a.DispatchRemotePendingSend(id); err == nil {
		t.Fatal("dispatched twice")
	}
	if submits != 1 {
		t.Fatal("submitted twice")
	}
	uncertain = true
	id2, err := a.QueuePendingSend("a", "", string(payload))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.DispatchRemotePendingSend(id2); err == nil {
		t.Fatal("uncertain send reported success")
	}
	if _, err := a.DispatchRemotePendingSend(id2); err == nil {
		t.Fatal("uncertain send resent")
	}
	if submits != 2 {
		t.Fatalf("submits after uncertain retry=%d", submits)
	}
	receiptSent = true
	if _, err := a.ListPendingSends(); err != nil {
		t.Fatal(err)
	}
	store, err := a.remotePendingStore()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := store.Uncertain()
	if err != nil || len(ids) != 0 {
		t.Fatalf("unreconciled=%v err=%v", ids, err)
	}
	if submits != 2 {
		t.Fatal("receipt reconciliation submitted mail")
	}

	if err := a.CancelPendingSend(id); err == nil {
		t.Fatal("cancel accepted after submission")
	}
}

func TestRemotePendingCancelAndRestartDoNotSend(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := &App{deviceDB: db, remoteServiceMode: true, runtimeEventEmitter: func(string, ...interface{}) {}}
	store, err := a.remotePendingStore()
	if err != nil {
		t.Fatal(err)
	}
	// A missing mailbox DB must produce an ordinary error, not a nil dereference.
	if _, err := a.QueuePendingSend("a", "", "invalid json"); err == nil {
		t.Fatal("malformed payload accepted")
	}
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	if err := a.ExpirePendingSends(); err != nil {
		t.Fatal(err)
	}
	items, err := a.ListPendingSends()
	if err != nil || len(items) != 0 {
		t.Fatalf("pending=%v error=%v", items, err)
	}
}
