package scheduledsend

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSecurityLostAcknowledgementAndReceiptReplay(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	var savedReceipt []byte
	var requests [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		requests = append(requests, body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if len(requests) == 1 {
			response := httptest.NewRecorder()
			store.Handler().ServeHTTP(response, r)
			savedReceipt = bytes.Clone(response.Body.Bytes())
			w.WriteHeader(http.StatusServiceUnavailable) // Queue committed; receipt lost.
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(savedReceipt)
	}))
	defer server.Close()
	local := securityClientStore(t, server.URL, helper)
	job, err := local.Schedule(ctx, "local-account", "draft", "fingerprint", *baselineCommand().Delivery, client)
	if err == nil || job.ID == "" || job.State != "awaiting_ack" {
		t.Fatalf("unconfirmed acceptance: %+v %v", job, err)
	}
	accepted, err := store.GetStatus(ctx, job.ID)
	if err != nil || accepted.State != Queued {
		t.Fatalf("durable helper acceptance: %+v %v", accepted, err)
	}
	job, err = local.Submit(ctx, job.ID, client)
	if err != nil || job.State != "queued" {
		t.Fatalf("exact retry receipt: %+v %v", job, err)
	}
	if len(requests) != 2 || !bytes.Equal(requests[0], requests[1]) {
		t.Fatal("retry changed encrypted request")
	}
	if _, err := local.Refresh(ctx, job.ID, client); err == nil {
		t.Fatal("old encrypted receipt confirmed a fresh query")
	}
	if bytes.Equal(requests[1], requests[2]) {
		t.Fatal("status query reused encrypted schedule")
	}
	current, _, _, _, err := local.request(ctx, job.ID, false)
	if err != nil || current.State != "queued" || current.Revision != 1 {
		t.Fatalf("replay changed client state: %+v %v", current, err)
	}
}

func TestSecurityStatusCannotOverwriteConfirmedCancellation(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	statusReady, releaseStatus := make(chan struct{}), make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(releaseStatus) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		command, _, err := OpenCommand(body, helper)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		if command.Delivery != nil {
			defer clear(command.Delivery.RFC822)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		response := httptest.NewRecorder()
		store.Handler().ServeHTTP(response, r)
		if command.Action == "status" {
			close(statusReady)
			<-releaseStatus // Deliver this stale queued receipt after cancellation.
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	}))
	defer server.Close()
	local := securityClientStore(t, server.URL, helper)
	job, err := local.Schedule(ctx, "local-account", "draft", "fingerprint", *baselineCommand().Delivery, client)
	if err != nil {
		t.Fatal(err)
	}
	refreshDone := make(chan error, 1)
	go func() { _, err := local.Refresh(ctx, job.ID, client); refreshDone <- err }()
	select {
	case <-statusReady:
	case <-time.After(5 * time.Second):
		t.Fatal("status query did not start")
	}
	cancelled, err := local.Cancel(ctx, job.ID, client)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	release.Do(func() { close(releaseStatus) })
	select {
	case err := <-refreshDone:
		if !errors.Is(err, ErrStateChanged) {
			t.Fatalf("stale status: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("status query did not finish")
	}
	current, _, _, payload, err := local.request(ctx, job.ID, true)
	if err != nil || current.State != "cancelled" || current.Revision != 2 || len(payload) != 0 {
		t.Fatalf("cancellation overwritten: %+v %v", current, err)
	}
}

func TestSecurityCrossProcessSendNowArbitration(t *testing.T) {
	ctx := context.Background()
	_, client, helper := baselineQueue(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	local := securityClientStore(t, server.URL, helper)
	var index int
	var name, path string
	if err := local.db.QueryRow("PRAGMA database_list").Scan(&index, &name, &path); err != nil {
		t.Fatal(err)
	}
	other, err := OpenClientStore(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := other.Close(); err != nil {
			t.Error(err)
		}
	})
	for i := 0; i < 12; i++ {
		draft := baselineCommand().JobID
		start := make(chan struct{})
		type scheduleResult struct {
			job ClientJob
			err error
		}
		scheduled := make(chan scheduleResult, 1)
		immediate := make(chan error, 1)
		go func() {
			<-start
			job, err := local.Schedule(ctx, "local-account", draft, "fingerprint", *baselineCommand().Delivery, client)
			scheduled <- scheduleResult{job, err}
		}()
		go func() { <-start; immediate <- other.BeginImmediateSend(ctx, "local-account", draft) }()
		close(start)
		result, immediateErr := <-scheduled, <-immediate
		if (result.job.ID != "") == (immediateErr == nil) {
			t.Fatalf("expected one winner; scheduled=%+v %v, immediate=%v", result.job, result.err, immediateErr)
		}
		if result.job.ID != "" {
			if result.err == nil || result.job.State != "awaiting_ack" {
				t.Fatalf("offline scheduled winner: %+v %v", result.job, result.err)
			}
			continue
		}
		if result.err == nil {
			t.Fatal("losing schedule lacked error")
		}
	}
}

func TestSecurityOtherPairedClientCannotOperateJob(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	other, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PairClient(ctx, other.PublicKey(), Permissions{Senders: []SenderPermission{{AccountID: "test-account", From: "sender@example.com"}}}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"status", "cancel", "schedule"} {
		change := command
		change.Action, change.Revision = action, 2
		if action != "schedule" {
			change.Delivery = nil
		}
		if _, err := store.Accept(ctx, baselineSeal(t, change, other, helper)); err == nil {
			t.Fatalf("other owner authorized for %s", action)
		}
	}
	status, err := store.GetStatus(ctx, command.JobID)
	if err != nil || status.State != Queued || status.Revision != 1 {
		t.Fatalf("owner isolation: %+v %v", status, err)
	}
}

func securityClientStore(t *testing.T, endpoint string, helper *Identity) *ClientStore {
	t.Helper()
	store, err := OpenClientStore(filepath.Join(t.TempDir(), "client"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := store.SaveConfig(context.Background(), ClientConfig{AccountID: "local-account", Endpoint: endpoint, HelperKey: helper.PublicKey().String(), HelperAccountID: "test-account"}); err != nil {
		t.Fatal(err)
	}
	return store
}
