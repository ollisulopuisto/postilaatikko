package v1

import (
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
