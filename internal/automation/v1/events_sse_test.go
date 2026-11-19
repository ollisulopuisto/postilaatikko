package v1

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func allowAll(http.ResponseWriter, *http.Request, Capability) bool { return true }

func denyAll(w http.ResponseWriter, r *http.Request, c Capability) bool {
	writeHeaderError(w, r, http.StatusForbidden, CodeCapabilityDenied, "denied", false)
	return false
}

// stubSource implements MailboxEventSource with test-controlled channels.
type stubSource struct {
	mu        sync.Mutex
	subs      []chan MailboxEvent
	replay    []MailboxEvent
	resync    bool
	err       error
	afterSeqs []uint64
}

func (s *stubSource) Subscribe(afterSeq uint64) ([]MailboxEvent, <-chan MailboxEvent, bool, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.afterSeqs = append(s.afterSeqs, afterSeq)
	if s.err != nil {
		return nil, nil, false, nil, s.err
	}
	ch := make(chan MailboxEvent, 8)
	s.subs = append(s.subs, ch)
	cancel := func() {}
	return s.replay, ch, s.resync, cancel, nil
}

func (s *stubSource) broadcast(ev MailboxEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		ch <- ev
	}
}

func (s *stubSource) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		close(ch)
	}
}

// readSSEEvent reads one event block (id/event/data lines) from the stream.
func readSSEEvent(t *testing.T, scanner *bufio.Scanner) (id string, ev MailboxEvent) {
	t.Helper()
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				t.Fatalf("bad data line: %v", err)
			}
		case line == "":
			if ev.Type != "" {
				return id, ev
			}
		}
	}
	t.Fatalf("stream ended before an event arrived; scanner err %v", scanner.Err())
	return "", MailboxEvent{}
}

func newEventsServer(t *testing.T, src MailboxEventSource, checker GrantRevocationChecker, grantID func(*http.Request) string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	MountEventsRoute(mux, src, checker, grantID, allowAll)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestEventsStreamDeliversLiveEvents(t *testing.T) {
	src := &stubSource{}
	srv := newEventsServer(t, src, nil, nil)

	resp, err := srv.Client().Get(srv.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}

	src.broadcast(MailboxEvent{Sequence: 7, Type: EventMailboxChanged, AccountID: "acc1", FolderID: "inbox", At: time.Now().UTC()})
	scanner := bufio.NewScanner(resp.Body)
	id, ev := readSSEEvent(t, scanner)
	if ev.Type != EventMailboxChanged || ev.AccountID != "acc1" || ev.FolderID != "inbox" {
		t.Fatalf("event = %+v", ev)
	}
	if id != "7" || ev.Sequence != 7 {
		t.Fatalf("id = %q sequence = %d, want 7", id, ev.Sequence)
	}
}

func TestEventsStreamTwoSubscribersConverge(t *testing.T) {
	src := &stubSource{}
	srv := newEventsServer(t, src, nil, nil)

	connect := func() *bufio.Scanner {
		resp, err := srv.Client().Get(srv.URL + "/v1/events")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return bufio.NewScanner(resp.Body)
	}
	s1, s2 := connect(), connect()
	src.broadcast(MailboxEvent{Sequence: 3, Type: EventMailboxChanged, AccountID: "acc1", FolderID: "inbox", At: time.Now().UTC()})
	_, ev1 := readSSEEvent(t, s1)
	_, ev2 := readSSEEvent(t, s2)
	if ev1.Sequence != ev2.Sequence || ev1.Type != EventMailboxChanged {
		t.Fatalf("subscribers diverged: %+v vs %+v", ev1, ev2)
	}
}

func TestEventsStreamResyncPrecedesReplay(t *testing.T) {
	src := &stubSource{
		resync: true,
		replay: []MailboxEvent{{Sequence: 5, Type: EventMailboxChanged, AccountID: "acc1", At: time.Now().UTC()}},
	}
	srv := newEventsServer(t, src, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/events", nil)
	req.Header.Set("Last-Event-ID", "2")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	_, first := readSSEEvent(t, scanner)
	if first.Type != EventResync {
		t.Fatalf("first event after gap = %q, want %q", first.Type, EventResync)
	}
	id, second := readSSEEvent(t, scanner)
	if second.Sequence != 5 || id != "5" {
		t.Fatalf("replay after resync = %+v id %q, want sequence 5", second, id)
	}
	if src.afterSeqs[0] != 2 {
		t.Fatalf("source got afterSeq %d, want 2 from Last-Event-ID", src.afterSeqs[0])
	}
}

func TestEventsStreamRejectsMalformedLastEventID(t *testing.T) {
	srv := newEventsServer(t, &stubSource{}, nil, nil)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/events", nil)
	req.Header.Set("Last-Event-ID", "abc")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestEventsStreamSaturatedSourceAnswers503(t *testing.T) {
	srv := newEventsServer(t, &stubSource{err: ErrEventSourceSaturated}, nil, nil)
	resp, err := srv.Client().Get(srv.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

func TestEventsStreamEndsOnRevocation(t *testing.T) {
	old := eventsRevocationInterval
	eventsRevocationInterval = 20 * time.Millisecond
	defer func() { eventsRevocationInterval = old }()

	var revoked atomic.Bool
	checker := grantRevocationFunc(func(string) (bool, error) { return revoked.Load(), nil })
	srv := newEventsServer(t, &stubSource{}, checker, func(*http.Request) string { return "client-1" })

	resp, err := srv.Client().Get(srv.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	revoked.Store(true)
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end after grant revocation")
	}
}

func TestEventsStreamRequiresCapability(t *testing.T) {
	mux := http.NewServeMux()
	MountEventsRoute(mux, &stubSource{}, nil, nil, denyAll)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestEventsStreamEndsOnSourceClose(t *testing.T) {
	src := &stubSource{}
	srv := newEventsServer(t, src, nil, nil)
	resp, err := srv.Client().Get(srv.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	src.closeAll()
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end on source close (helper shutdown)")
	}
}

type grantRevocationFunc func(string) (bool, error)

func (f grantRevocationFunc) GrantRevoked(clientID string) (bool, error) { return f(clientID) }

type unwrapEventWriter struct{ http.ResponseWriter }

func (w unwrapEventWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func TestEventsWithUnwrapWriterAndServerDeadline(t *testing.T) {
	old := eventsHeartbeatInterval
	eventsHeartbeatInterval = 5 * time.Millisecond
	defer func() { eventsHeartbeatInterval = old }()
	src := &stubSource{}
	mux := http.NewServeMux()
	MountEventsRoute(mux, src, nil, nil, allowAll)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mux.ServeHTTP(unwrapEventWriter{w}, r) }))
	srv.Config.WriteTimeout = 15 * time.Millisecond
	srv.Start()
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	time.Sleep(40 * time.Millisecond)
	src.broadcast(MailboxEvent{Sequence: 1, Type: EventMailboxChanged})
	_, ev := readSSEEvent(t, bufio.NewScanner(resp.Body))
	if ev.Sequence != 1 {
		t.Fatalf("event=%+v", ev)
	}
}

func TestEventsRevalidationFailsClosed(t *testing.T) {
	old := eventsRevocationInterval
	eventsRevocationInterval = 5 * time.Millisecond
	defer func() { eventsRevocationInterval = old }()
	for _, id := range []string{"client-1", ""} {
		checker := grantRevocationFunc(func(string) (bool, error) { return false, io.ErrUnexpectedEOF })
		srv := newEventsServer(t, &stubSource{}, checker, func(*http.Request) string { return id })
		resp, err := srv.Client().Get(srv.URL + "/v1/events")
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(io.Discard, resp.Body); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("stream remained open without verifiable grant")
		}
		resp.Body.Close()
	}
}
