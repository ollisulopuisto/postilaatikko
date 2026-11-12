package v1

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/message"
)

type cachedVirtualFolderTestOperations struct {
	view *CachedVirtualFolderView
	err  error
}

func TestMountVirtualFolderCacheRouteReturnsNotFoundForUnknownView(t *testing.T) {
	mux := http.NewServeMux()
	MountVirtualFolderCacheRoute(mux, cachedVirtualFolderTestOperations{err: ErrCachedVirtualFolderNotFound}, func(w http.ResponseWriter, _ *http.Request, _ Capability) bool {
		return true
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/views/missing/cache", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if response.Body.Len() == 0 {
		t.Fatal("not-found response omitted its error body")
	}
}

func (o cachedVirtualFolderTestOperations) GetCachedVirtualFolderView(context.Context, string) (*CachedVirtualFolderView, error) {
	return o.view, o.err
}

func TestMountVirtualFolderCacheRouteReturnsHydratedCacheAndFreshness(t *testing.T) {
	refreshedAt := time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC)
	view := &CachedVirtualFolderView{
		VirtualFolderID: "view-1",
		Conversations:   []*message.Conversation{{ThreadID: "thread-1", Subject: "Cached subject", UnreadCount: 1}},
		RefreshedAt:     refreshedAt,
		HasCache:        true,
	}
	mux := http.NewServeMux()
	MountVirtualFolderCacheRoute(mux, cachedVirtualFolderTestOperations{view: view}, func(w http.ResponseWriter, _ *http.Request, capability Capability) bool {
		if capability != CapabilityMailRead {
			t.Errorf("required capability = %q, want %q", capability, CapabilityMailRead)
		}
		return true
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/views/view-1/cache", nil)
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"virtualFolderId":"view-1"`) || !strings.Contains(response.Body.String(), `"refreshedAt":"2026-11-08T12:00:00Z"`) || !strings.Contains(response.Body.String(), `"subject":"Cached subject"`) {
		t.Fatalf("response = %s, want cached view, timestamp, and hydrated conversation", response.Body.String())
	}
}

type slowCachedVirtualFolderOperations struct {
	delay time.Duration
}

func (o slowCachedVirtualFolderOperations) GetCachedVirtualFolderView(context.Context, string) (*CachedVirtualFolderView, error) {
	time.Sleep(o.delay)
	return &CachedVirtualFolderView{VirtualFolderID: "view-slow", HasCache: true}, nil
}

type busyCachedVirtualFolderOperations struct {
	attempts chan struct{}
}

func (o busyCachedVirtualFolderOperations) GetCachedVirtualFolderView(context.Context, string) (*CachedVirtualFolderView, error) {
	return nil, errors.New("failed to begin transaction: database is locked (5) (SQLITE_BUSY)")
}

// Lock contention must surface as a distinct retryable database_busy code, not
// a generic internal_error the desktop cannot tell apart from real failures.
func TestMountVirtualFolderCacheRouteClassifiesBusy(t *testing.T) {
	mux := http.NewServeMux()
	MountVirtualFolderCacheRoute(mux, busyCachedVirtualFolderOperations{}, func(w http.ResponseWriter, _ *http.Request, _ Capability) bool {
		return true
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/views/view-1/cache", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"code":"database_busy"`) {
		t.Fatalf("response = %s, want database_busy code", body)
	}
	if !strings.Contains(body, `"retryable":true`) {
		t.Fatalf("response = %s, want retryable true", body)
	}
	if strings.Contains(body, "SQLITE_BUSY") || strings.Contains(body, "database is locked") {
		t.Fatalf("response = %s, must not leak raw database errors", body)
	}
}

// The desktop's request budget is two seconds; the route answers within its
// bounded deadline instead of holding the request through a long lock wait.
// Deadline expiry is labelled cache_timeout: a slow-but-healthy hydration is
// not SQLite lock contention, and database_busy stays reserved for real lock
// errors so clients are not told to expect contention that never happened.
func TestMountVirtualFolderCacheRouteHonoursBoundedDeadline(t *testing.T) {
	previous := cachedViewRouteDeadline
	cachedViewRouteDeadline = 50 * time.Millisecond
	defer func() { cachedViewRouteDeadline = previous }()

	mux := http.NewServeMux()
	MountVirtualFolderCacheRoute(mux, slowCachedVirtualFolderOperations{delay: 2 * time.Second}, func(w http.ResponseWriter, _ *http.Request, _ Capability) bool {
		return true
	})
	response := httptest.NewRecorder()
	start := time.Now()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/views/view-slow/cache", nil))
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("route took %s, want it to give up within the bounded deadline", elapsed)
	}
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"code":"cache_timeout"`) {
		t.Fatalf("response = %s, want cache_timeout code for deadline expiry", body)
	}
	if strings.Contains(body, "database_busy") {
		t.Fatalf("response = %s, a route timeout must not be labelled database_busy", body)
	}
	if !strings.Contains(body, `"retryable":true`) {
		t.Fatalf("response = %s, want retryable true", body)
	}
}

// Concurrent cache requests for one view share a single refresh instead of
// each starting its own detached goroutine: timed-out requests would
// otherwise pile abandoned cache writes onto the helper's bounded database
// pool until unrelated operations starve.
func TestMountVirtualFolderCacheRouteCoalescesConcurrentRefreshes(t *testing.T) {
	ops := &countingSlowCachedViewOperations{delay: 300 * time.Millisecond}
	mux := http.NewServeMux()
	MountVirtualFolderCacheRoute(mux, ops, func(w http.ResponseWriter, _ *http.Request, _ Capability) bool {
		return true
	})
	const requests = 8
	var wg sync.WaitGroup
	codes := make(chan int, requests)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/views/view-1/cache", nil))
			codes <- response.Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Fatalf("status = %d, want every joined request served by the shared refresh", code)
		}
	}
	if got := ops.calls.Load(); got != 1 {
		t.Fatalf("GetCachedVirtualFolderView ran %d times for %d concurrent requests, want 1 coalesced refresh", got, requests)
	}
}

type countingSlowCachedViewOperations struct {
	delay time.Duration
	calls atomic.Int32
}

func (o *countingSlowCachedViewOperations) GetCachedVirtualFolderView(context.Context, string) (*CachedVirtualFolderView, error) {
	o.calls.Add(1)
	time.Sleep(o.delay)
	return &CachedVirtualFolderView{VirtualFolderID: "view-1", HasCache: true}, nil
}

// The detached refresh that outlives its request is bounded: once its work
// budget expires the context is cancelled, so an abandoned cache write
// cannot hold a pool connection through the full lock-retry budget.
func TestMountVirtualFolderCacheRouteCancelsDetachedWork(t *testing.T) {
	prevDeadline := cachedViewRouteDeadline
	cachedViewRouteDeadline = 20 * time.Millisecond
	defer func() { cachedViewRouteDeadline = prevDeadline }()
	prevBudget := detachedCacheWorkBudget
	detachedCacheWorkBudget = 100 * time.Millisecond
	defer func() { detachedCacheWorkBudget = prevBudget }()

	ops := &ctxAwareCachedViewOperations{cancelled: make(chan struct{})}
	mux := http.NewServeMux()
	MountVirtualFolderCacheRoute(mux, ops, func(w http.ResponseWriter, _ *http.Request, _ Capability) bool {
		return true
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/views/view-1/cache", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"cache_timeout"`) {
		t.Fatalf("response = %d %s, want 503 cache_timeout while the refresh is still running", response.Code, response.Body.String())
	}
	select {
	case <-ops.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("detached cache work was not cancelled after its work budget expired")
	}
}

type ctxAwareCachedViewOperations struct {
	cancelled chan struct{}
}

func (o *ctxAwareCachedViewOperations) GetCachedVirtualFolderView(ctx context.Context, _ string) (*CachedVirtualFolderView, error) {
	<-ctx.Done()
	close(o.cancelled)
	return nil, ctx.Err()
}
