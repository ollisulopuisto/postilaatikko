package scheduledsend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProtectBrowserRequestsAllowsConfiguredHosts(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := ProtectBrowserRequests(ok, "studio.example.com", "backup.example.com")

	cases := []struct {
		host string
		want int
	}{
		{"studio.example.com", http.StatusNoContent},
		{"studio.example.com:8028", http.StatusNoContent},
		{"STUDIO.EXAMPLE.COM", http.StatusNoContent},
		{"backup.example.com", http.StatusNoContent},
		{"evil.com", http.StatusBadRequest},
		{"studio.example.com.evil.com", http.StatusBadRequest},
		{"example.com", http.StatusBadRequest},
		{"127.0.0.1", http.StatusNoContent},
		{"[::1]:8028", http.StatusNoContent},
	}
	for _, tc := range cases {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
		request.Host = tc.host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("Host %q: expected %d, got %d", tc.host, tc.want, response.Code)
		}
	}
}

func TestProtectBrowserRequestsWithoutConfiguredHostsStaysLoopbackOnly(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := ProtectBrowserRequests(ok)
	for _, host := range []string{"studio.example.com", "evil.com"} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
		request.Host = host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("Host %q without allowlist: expected 400, got %d", host, response.Code)
		}
	}
}

func TestStoreHandlerForwardsAllowedHosts(t *testing.T) {
	store, _, _ := baselineQueue(t)
	handler := store.Handler("studio.example.com")
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/v1/commands", nil)
	request.Host = "studio.example.com:8028"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	// 405 proves the Host check passed; 400 would mean the allowlist was dropped.
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 once the configured Host is accepted, got %d", response.Code)
	}
}
