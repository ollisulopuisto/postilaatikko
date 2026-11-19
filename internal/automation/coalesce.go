package automation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Read request coalescing (PERF-SC-002). The remote desktop issues the same
// read-only API calls from several components at once (folder pages, counts,
// saved-view reads, identical searches); the baseline shows each duplicate
// re-executes the full SQL aggregation. This middleware merges concurrent
// identical eligible requests into one execution and shares the response,
// with these guarantees:
//
//   - Only an allowlist of read-only routes is eligible. Mutations (send,
//     move, delete, undo, folder/filter/draft/triage mutations, ...) are
//     never coalesced, so their idempotency behavior is unchanged.
//   - Flight identity covers the method, path, raw query, request body,
//     the Authorization header, and the caller's X-Request-Id (the response
//     envelope echoes it), so results are never shared across clients with
//     different credentials or correlation IDs.
//   - Execution runs on a context detached from any single caller's
//     cancellation: a cancelled waiter never cancels work another waiter
//     still needs. When the last waiter leaves, execution is cancelled so
//     no goroutine or query outlives its callers.
//   - Each waiter keeps its own request context and deadline; a timed-out
//     waiter detaches without affecting the others.
//   - The number of in-flight flights, the request body read for keying,
//     and the shared response buffer are all bounded. Overflowing requests
//     execute directly instead of queuing unboundedly.

type readCoalescerConfig struct {
	// MaxInFlight bounds distinct flights executing at once. Requests beyond
	// the bound execute directly.
	MaxInFlight int
	// MaxRequestBody bounds the body read to key POST reads. Larger bodies
	// execute directly.
	MaxRequestBody int64
	// MaxResponseBody bounds the shared response buffer. Oversized responses
	// fail the flight with 502 instead of growing memory without bound.
	MaxResponseBody int
}

func defaultReadCoalescerConfig() readCoalescerConfig {
	return readCoalescerConfig{
		MaxInFlight:     64,
		MaxRequestBody:  1 << 20,
		MaxResponseBody: 32 << 20,
	}
}

// coalescibleRead reports whether a request is eligible for coalescing. Only
// read-only operations whose responses depend solely on the request and the
// caller's authorization qualify. Keep this list in sync with the routes in
// internal/automation/v1: adding a route here that mutates state, streams, or
// personalizes per client identity breaks those semantics. The route
// classification test in internal/alwayson fails when a mounted route has no
// explicit classification.
//
// /v1/threads/read is eligible despite its name: "read" means rendering a
// thread. ReadHandler.handleThreadRead only resolves the thread and loads
// messages/attachments (mailboxReadOperations.GetThread performs no writes
// and never marks mail read); marking read happens through /v1/actions.
func coalescibleRead(method, path string) bool {
	switch method {
	case http.MethodGet:
		switch path {
		case "/v1/accounts", "/v1/folders", "/v1/views", "/v1/views/unread-counts":
			return true
		}
		// Saved-view cache reads: GET /v1/views/{virtualFolderID}/cache.
		return strings.HasPrefix(path, "/v1/views/") && strings.HasSuffix(path, "/cache")
	case http.MethodPost:
		switch path {
		case "/v1/conversations", "/v1/conversations/count", "/v1/conversations/search",
			"/v1/search", "/v1/threads/read":
			return true
		}
	}
	return false
}

// CoalescibleRead reports whether the read coalescer merges requests for a
// method/path pair. Exported for the route classification drift test in
// internal/alwayson.
func CoalescibleRead(method, path string) bool {
	return coalescibleRead(method, path)
}

type coalesceResult struct {
	status   int
	header   http.Header
	body     []byte
	overflow bool
}

type coalesceFlight struct {
	done    chan struct{}
	result  coalesceResult
	pattern string // inner router's route pattern, propagated for timing logs
	waiters int
	// finished is set when the execution completed (normally, with an
	// overflow, or via panic recovery) and the result is final. abandoned is
	// set when the last waiter left: execution is cancelled and the flight is
	// unregistered so a retry starts fresh instead of inheriting a cancelled
	// context.
	finished   bool
	abandoned  bool
	execCancel context.CancelFunc
}

type readCoalescer struct {
	next    http.Handler
	config  readCoalescerConfig
	mu      sync.Mutex
	flights map[string]*coalesceFlight
}

func newReadCoalescer(next http.Handler, config readCoalescerConfig) *readCoalescer {
	if config.MaxInFlight <= 0 {
		config.MaxInFlight = 64
	}
	if config.MaxRequestBody <= 0 {
		config.MaxRequestBody = 1 << 20
	}
	if config.MaxResponseBody <= 0 {
		config.MaxResponseBody = 32 << 20
	}
	return &readCoalescer{
		next:    next,
		config:  config,
		flights: make(map[string]*coalesceFlight),
	}
}

func (c *readCoalescer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !coalescibleRead(r.Method, r.URL.Path) {
		c.next.ServeHTTP(w, r)
		return
	}

	var body []byte
	if r.Body != nil && r.Method == http.MethodPost {
		read, err := io.ReadAll(io.LimitReader(r.Body, c.config.MaxRequestBody+1))
		_ = r.Body.Close()
		if err != nil || int64(len(read)) > c.config.MaxRequestBody {
			// Unreadable or oversized body: execute directly. Restoring what
			// was read is best-effort; on error the handler will fail parsing
			// exactly as it would for a truncated upload.
			r.Body = io.NopCloser(bytes.NewReader(read))
			c.next.ServeHTTP(w, r)
			return
		}
		body = read
	}

	key := coalesceKey(r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("X-Request-Id"), body)

	c.mu.Lock()
	flight := c.flights[key]
	if flight != nil && flight.abandoned {
		// Defensive: abandoned flights are unregistered when the last waiter
		// leaves, so this only guards a future regression. A cancelled
		// execution must never gain new waiters.
		flight = nil
	}
	if flight == nil {
		if len(c.flights) >= c.config.MaxInFlight {
			c.mu.Unlock()
			if body != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			c.next.ServeHTTP(w, r)
			return
		}
		flight = &coalesceFlight{done: make(chan struct{}), waiters: 1}
		c.flights[key] = flight
		execCtx, execCancel := context.WithCancel(context.WithoutCancel(r.Context()))
		flight.execCancel = execCancel
		go c.execute(key, flight, execCtx, r, body)
	} else {
		flight.waiters++
	}
	c.mu.Unlock()

	select {
	case <-flight.done:
		c.mu.Lock()
		flight.waiters--
		pattern := flight.pattern
		c.mu.Unlock()
		if pattern != "" {
			// Propagate the inner router's pattern so the timing middleware
			// (which logs this request after ServeHTTP returns) sees the real
			// endpoint instead of the "/v1/" mount. Safe: the worker finished
			// before done closed, and each waiter mutates only its own request.
			r.Pattern = pattern
		}
		writeCoalesceResult(w, flight.result)
	case <-r.Context().Done():
		c.mu.Lock()
		flight.waiters--
		if flight.waiters == 0 && !flight.finished {
			// Last waiter gone: cancel execution and unregister atomically so
			// a retry of the same request starts a fresh flight instead of
			// joining a cancelled one.
			flight.abandoned = true
			if c.flights[key] == flight {
				delete(c.flights, key)
			}
			flight.execCancel()
		}
		c.mu.Unlock()
		// The caller is gone; there is nothing to write.
	}
}

// execute runs the underlying handler once and publishes the result. A
// panicking handler is recovered here: without coalescing the HTTP server
// recovers panics in the request goroutine, but the worker goroutine has no
// such protection and a panic would terminate the whole helper. A panic
// becomes a 500 for every waiter, exactly like any other failed execution.
func (c *readCoalescer) execute(key string, flight *coalesceFlight, execCtx context.Context, r *http.Request, body []byte) {
	execReq := r.Clone(execCtx)
	if body != nil {
		execReq.Body = io.NopCloser(bytes.NewReader(body))
	}
	recorder := &boundedRecorder{
		header: make(http.Header),
		status: http.StatusOK,
		limit:  c.config.MaxResponseBody,
	}
	defer func() {
		if p := recover(); p != nil {
			// http.ErrAbortHandler is also absorbed: it is a per-connection
			// abort signal that has no meaning for the other waiters sharing
			// this execution, and re-panicking would kill the process.
			c.finish(key, flight, coalesceResult{
				status: http.StatusInternalServerError,
				header: make(http.Header),
				body:   []byte(`{"error":"read execution failed"}`),
			}, execReq.Pattern)
		}
	}()
	c.next.ServeHTTP(recorder, execReq)
	c.finish(key, flight, coalesceResult{
		status:   recorder.status,
		header:   recorder.header,
		body:     recorder.buf.Bytes(),
		overflow: recorder.overflow,
	}, execReq.Pattern)
}

// finish publishes the execution result and releases waiters. The flight is
// removed from the map before waiters are released so no new request can
// join a completed execution; the removal only happens when the map still
// references this flight, so an abandoned flight cannot delete its
// replacement.
func (c *readCoalescer) finish(key string, flight *coalesceFlight, result coalesceResult, pattern string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if flight.finished {
		return
	}
	flight.result = result
	flight.pattern = pattern
	flight.finished = true
	if c.flights[key] == flight {
		delete(c.flights, key)
	}
	flight.execCancel()
	close(flight.done)
}

func coalesceKey(method, path, rawQuery, authorization, requestID string, body []byte) string {
	sum := sha256.New()
	sum.Write([]byte(method))
	sum.Write([]byte{0})
	sum.Write([]byte(path))
	sum.Write([]byte{0})
	sum.Write([]byte(rawQuery))
	sum.Write([]byte{0})
	sum.Write([]byte(authorization))
	sum.Write([]byte{0})
	// The API envelope echoes the caller's X-Request-Id; requests with
	// different correlation IDs must not share a serialized response.
	sum.Write([]byte(requestID))
	sum.Write([]byte{0})
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(body)))
	sum.Write(length[:])
	sum.Write(body)
	return string(sum.Sum(nil))
}

func writeCoalesceResult(w http.ResponseWriter, result coalesceResult) {
	if result.overflow {
		// The response exceeded the shared buffer bound and cannot be
		// replayed reliably; fail explicitly instead of truncating.
		http.Error(w, `{"error":"coalesced response exceeded the shared buffer limit"}`, http.StatusBadGateway)
		return
	}
	for name, values := range result.header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(result.status)
	_, _ = w.Write(result.body)
}

// boundedRecorder captures a handler response up to a byte limit. Writes past
// the limit are discarded and recorded as an overflow.
type boundedRecorder struct {
	header   http.Header
	status   int
	buf      bytes.Buffer
	limit    int
	overflow bool
}

func (r *boundedRecorder) Header() http.Header { return r.header }

func (r *boundedRecorder) WriteHeader(status int) { r.status = status }

func (r *boundedRecorder) Write(p []byte) (int, error) {
	if r.buf.Len()+len(p) > r.limit {
		r.overflow = true
		return len(p), nil
	}
	return r.buf.Write(p)
}
