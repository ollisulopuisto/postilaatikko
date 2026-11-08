package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type virtualFolderUnreadCountOperationsFunc func() (map[string]int, error)

func (f virtualFolderUnreadCountOperationsFunc) GetVirtualFolderUnreadCounts() (map[string]int, error) {
	return f()
}

func TestMountVirtualFolderUnreadCountRouteReturnsServiceCounts(t *testing.T) {
	var requested Capability
	mux := http.NewServeMux()
	MountVirtualFolderUnreadCountRoute(mux, virtualFolderUnreadCountOperationsFunc(func() (map[string]int, error) {
		return map[string]int{"view-a": 3, "view-b": 0}, nil
	}), func(_ http.ResponseWriter, _ *http.Request, capability Capability) bool {
		requested = capability
		return true
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/views/unread-counts", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if requested != CapabilityMailRead {
		t.Fatalf("required capability = %q, want %q", requested, CapabilityMailRead)
	}
	var result Response[map[string]int]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data["view-a"] != 3 || result.Data["view-b"] != 0 || len(result.Data) != 2 {
		t.Fatalf("counts = %#v, want view-a=3 and view-b=0", result.Data)
	}
}

func TestMountVirtualFolderUnreadCountRouteRequiresCapability(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	MountVirtualFolderUnreadCountRoute(mux, virtualFolderUnreadCountOperationsFunc(func() (map[string]int, error) {
		called = true
		return nil, nil
	}), func(w http.ResponseWriter, _ *http.Request, _ Capability) bool {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/views/unread-counts", nil))
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d called=%v, want forbidden without invoking operation", response.Code, called)
	}
}
