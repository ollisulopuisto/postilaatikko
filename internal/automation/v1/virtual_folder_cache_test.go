package v1

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (o cachedVirtualFolderTestOperations) GetCachedVirtualFolderView(string) (*CachedVirtualFolderView, error) {
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

func (o slowCachedVirtualFolderOperations) GetCachedVirtualFolderView(string) (*CachedVirtualFolderView, error) {
	time.Sleep(o.delay)
	return &CachedVirtualFolderView{VirtualFolderID: "view-slow", HasCache: true}, nil
}

type busyCachedVirtualFolderOperations struct {
	attempts chan struct{}
}

func (o busyCachedVirtualFolderOperations) GetCachedVirtualFolderView(string) (*CachedVirtualFolderView, error) {
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
	if !strings.Contains(response.Body.String(), `"code":"database_busy"`) {
		t.Fatalf("response = %s, want database_busy code", response.Body.String())
	}
}
