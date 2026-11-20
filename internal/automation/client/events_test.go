package client_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ollisulopuisto/postilaatikko/internal/automation/client"
	v1 "github.com/ollisulopuisto/postilaatikko/internal/automation/v1"
)

func TestSubscribeEventsParsesStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Last-Event-ID"); got != "41" {
			t.Errorf("Last-Event-ID = %q, want 41", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, ": heartbeat\n\n")
		fmt.Fprint(w, "id: 42\nevent: mailbox.changed\ndata: {\"sequence\":42,\"type\":\"mailbox.changed\",\"accountId\":\"acc1\",\"folderId\":\"inbox\",\"at\":\"2026-11-19T13:00:00Z\"}\n\n")
		fmt.Fprint(w, "event: mailbox.resync\ndata: {\"sequence\":0,\"type\":\"mailbox.resync\",\"at\":\"2026-11-19T13:00:01Z\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	api := client.New(client.Config{Endpoint: server.URL, Token: "test-token"})
	var got []v1.MailboxEvent
	err := api.SubscribeEvents(context.Background(), 41, func(ev v1.MailboxEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeEvents: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].Sequence != 42 || got[0].AccountID != "acc1" || got[0].FolderID != "inbox" {
		t.Fatalf("first event = %+v", got[0])
	}
	if got[1].Type != v1.EventResync {
		t.Fatalf("second event = %+v, want resync", got[1])
	}
}

func TestSubscribeEventsSurfacesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	api := client.New(client.Config{Endpoint: server.URL, Token: "test-token"})
	err := api.SubscribeEvents(context.Background(), 0, func(v1.MailboxEvent) error { return nil })
	if err == nil {
		t.Fatal("expected an error on 503")
	}
}

func TestSubscribeEventsStopsOnContextCancel(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-release
	}))
	defer server.Close()
	defer close(release)

	api := client.New(client.Config{Endpoint: server.URL, Token: "test-token"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- api.SubscribeEvents(ctx, 0, func(v1.MailboxEvent) error { return nil })
	}()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected context error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubscribeEvents did not stop on context cancel")
	}
}

func TestSubscriptionOutlivesOrdinaryRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 10; i++ {
			fmt.Fprint(w, ": heartbeat\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
		fmt.Fprint(w, "data: {\"sequence\":1,\"type\":\"mailbox.changed\"}\n\n")
	}))
	defer srv.Close()
	api := client.New(client.Config{Endpoint: srv.URL, Timeout: 20 * time.Millisecond})
	count := 0
	err := api.SubscribeEvents(context.Background(), 0, func(v1.MailboxEvent) error { count++; return nil })
	if err != nil || count != 1 {
		t.Fatalf("count=%d error=%v", count, err)
	}
}
