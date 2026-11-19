package automation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingHandler sleeps briefly, counts executions, and echoes a fixed body.
func countingHandler(t *testing.T, executions *atomic.Int32, delay time.Duration, body string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executions.Add(1)
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
}

func runConcurrent(t *testing.T, h http.Handler, n int, makeReq func(i int) (*http.Request, context.CancelFunc)) []*httptest.ResponseRecorder {
	t.Helper()
	recorders := make([]*httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, cancel := makeReq(i)
			defer cancel()
			rec := httptest.NewRecorder()
			<-start
			h.ServeHTTP(rec, req)
			recorders[i] = rec
		}(i)
	}
	close(start)
	wg.Wait()
	return recorders
}

func postConversationReq(t *testing.T, ctx context.Context, body, auth string) (*http.Request, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	req := httptest.NewRequest(http.MethodPost, "/v1/conversations", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return req, cancel
}

func TestReadCoalescerMergesConcurrentIdenticalReads(t *testing.T) {
	var executions atomic.Int32
	h := newReadCoalescer(countingHandler(t, &executions, 30*time.Millisecond, `{"items":[]}`), readCoalescerConfig{
		MaxInFlight: 8, MaxRequestBody: 1 << 20, MaxResponseBody: 1 << 20,
	})
	recorders := runConcurrent(t, h, 8, func(i int) (*http.Request, context.CancelFunc) {
		return postConversationReq(t, context.Background(), `{"scope":{"kind":"folder"}}`, "Bearer tok")
	})
	if got := executions.Load(); got != 1 {
		t.Fatalf("executions = %d, want 1", got)
	}
	for i, rec := range recorders {
		if rec.Code != http.StatusOK || rec.Body.String() != `{"items":[]}` {
			t.Fatalf("waiter %d: code %d body %q", i, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("waiter %d: missing content type", i)
		}
	}
}

func TestReadCoalescerSeparatesKeys(t *testing.T) {
	var executions atomic.Int32
	h := newReadCoalescer(countingHandler(t, &executions, 20*time.Millisecond, `{}`), readCoalescerConfig{
		MaxInFlight: 16, MaxRequestBody: 1 << 20, MaxResponseBody: 1 << 20,
	})
	runConcurrent(t, h, 4, func(i int) (*http.Request, context.CancelFunc) {
		// Two bodies x two auth tokens = four distinct flights.
		body := fmt.Sprintf(`{"page":%d}`, i%2)
		auth := "Bearer a"
		if i >= 2 {
			auth = "Bearer b"
		}
		return postConversationReq(t, context.Background(), body, auth)
	})
	if got := executions.Load(); got != 4 {
		t.Fatalf("executions = %d, want 4 (body and authorization must partition flights)", got)
	}
}

func TestReadCoalescerCancelledLeaderDoesNotCancelSharedExecution(t *testing.T) {
	var executions atomic.Int32
	done := make(chan struct{})
	h := newReadCoalescer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executions.Add(1)
		select {
		case <-time.After(50 * time.Millisecond):
			_, _ = io.WriteString(w, "shared")
		case <-r.Context().Done():
			t.Error("execution context was cancelled while a waiter remained")
		}
		close(done)
	}), readCoalescerConfig{MaxInFlight: 4, MaxRequestBody: 1 << 20, MaxResponseBody: 1 << 20})

	leaderCtx, leaderCancel := context.WithCancel(context.Background())
	leaderReq, _ := postConversationReq(t, leaderCtx, `{"x":1}`, "Bearer tok")
	leaderRec := httptest.NewRecorder()
	go func() { h.ServeHTTP(leaderRec, leaderReq) }()

	// Waiter joins the in-flight flight, then the leader cancels.
	waiterReq, waiterCancel := postConversationReq(t, context.Background(), `{"x":1}`, "Bearer tok")
	defer waiterCancel()
	waiterRec := httptest.NewRecorder()
	waiterDone := make(chan struct{})
	go func() { h.ServeHTTP(waiterRec, waiterReq); close(waiterDone) }()

	time.Sleep(10 * time.Millisecond) // let both join the flight
	leaderCancel()

	<-waiterDone
	if waiterRec.Body.String() != "shared" {
		t.Fatalf("waiter body = %q, want shared", waiterRec.Body.String())
	}
	<-done
	if got := executions.Load(); got != 1 {
		t.Fatalf("executions = %d, want 1", got)
	}
}

func TestReadCoalescerCancelsExecutionWhenAllWaitersLeave(t *testing.T) {
	sawCancel := make(chan struct{})
	h := newReadCoalescer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(sawCancel)
	}), readCoalescerConfig{MaxInFlight: 4, MaxRequestBody: 1 << 20, MaxResponseBody: 1 << 20})

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := postConversationReq(t, ctx, `{"x":2}`, "Bearer tok")
	rec := httptest.NewRecorder()
	go func() { h.ServeHTTP(rec, req) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-sawCancel:
	case <-time.After(2 * time.Second):
		t.Fatal("execution context was not cancelled after the only waiter left")
	}
}

func TestReadCoalescerSkipsMutations(t *testing.T) {
	var executions atomic.Int32
	h := newReadCoalescer(countingHandler(t, &executions, 10*time.Millisecond, `{}`), readCoalescerConfig{
		MaxInFlight: 4, MaxRequestBody: 1 << 20, MaxResponseBody: 1 << 20,
	})
	runConcurrent(t, h, 3, func(i int) (*http.Request, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest(http.MethodPost, "/v1/actions", strings.NewReader(`{"action":"archive"}`)).WithContext(ctx)
		return req, cancel
	})
	if got := executions.Load(); got != 3 {
		t.Fatalf("executions = %d, want 3 (mutations must never be coalesced)", got)
	}
}

func TestReadCoalescerRespectsInFlightBound(t *testing.T) {
	var executions atomic.Int32
	release := make(chan struct{})
	h := newReadCoalescer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executions.Add(1)
		<-release
		_, _ = io.WriteString(w, "ok")
	}), readCoalescerConfig{MaxInFlight: 1, MaxRequestBody: 1 << 20, MaxResponseBody: 1 << 20})

	req1, cancel1 := postConversationReq(t, context.Background(), `{"k":1}`, "Bearer tok")
	defer cancel1()
	rec1 := httptest.NewRecorder()
	go func() { h.ServeHTTP(rec1, req1) }()
	time.Sleep(10 * time.Millisecond) // flight occupies the single slot

	// A different key beyond the bound executes directly instead of blocking.
	req2, cancel2 := postConversationReq(t, context.Background(), `{"k":2}`, "Bearer tok")
	defer cancel2()
	rec2 := httptest.NewRecorder()
	go func() { h.ServeHTTP(rec2, req2) }()
	time.Sleep(10 * time.Millisecond)
	close(release)
	time.Sleep(20 * time.Millisecond)
	if got := executions.Load(); got != 2 {
		t.Fatalf("executions = %d, want 2 (overflow must pass through)", got)
	}
}

func TestReadCoalescerPreservesIndependentDeadlines(t *testing.T) {
	h := newReadCoalescer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(100 * time.Millisecond):
			_, _ = io.WriteString(w, "late")
		case <-r.Context().Done():
		}
	}), readCoalescerConfig{MaxInFlight: 4, MaxRequestBody: 1 << 20, MaxResponseBody: 1 << 20})

	leaderReq, leaderCancel := postConversationReq(t, context.Background(), `{"d":1}`, "Bearer tok")
	defer leaderCancel()
	leaderRec := httptest.NewRecorder()
	leaderDone := make(chan struct{})
	go func() { h.ServeHTTP(leaderRec, leaderReq); close(leaderDone) }()
	time.Sleep(10 * time.Millisecond)

	// Impatient waiter times out; the leader must still receive the result.
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer shortCancel()
	shortReq, _ := postConversationReq(t, shortCtx, `{"d":1}`, "Bearer tok")
	shortRec := httptest.NewRecorder()
	h.ServeHTTP(shortRec, shortReq)
	if shortRec.Body.Len() != 0 {
		t.Fatalf("timed-out waiter received a body: %q", shortRec.Body.String())
	}

	<-leaderDone
	if leaderRec.Body.String() != "late" {
		t.Fatalf("leader body = %q, want late", leaderRec.Body.String())
	}
}

func TestReadCoalescerResponseOverflow(t *testing.T) {
	h := newReadCoalescer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 64))
	}), readCoalescerConfig{MaxInFlight: 4, MaxRequestBody: 1 << 20, MaxResponseBody: 16})
	recorders := runConcurrent(t, h, 2, func(i int) (*http.Request, context.CancelFunc) {
		return postConversationReq(t, context.Background(), `{"o":1}`, "Bearer tok")
	})
	for i, rec := range recorders {
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("waiter %d: code %d, want 502 on unshareable oversized response", i, rec.Code)
		}
	}
}
