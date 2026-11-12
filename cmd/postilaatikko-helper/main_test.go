package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHelperHealthEndpoint(t *testing.T) {
	handler := helperHandler(nil, false, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"status":"ok"`) || !strings.Contains(body, `"service":"send-later"`) {
		t.Fatalf("unexpected health response: %s", body)
	}
}

func TestHelperHealthRejectsOtherMethods(t *testing.T) {
	recorder := httptest.NewRecorder()
	helperHandler(nil, true, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/healthz", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestHelperBrowserProtectionCoversAllRoutes(t *testing.T) {
	for _, origin := range []string{"https://untrusted.invalid", "null", ""} {
		for _, route := range []string{"/healthz", "/v1/commands", "/missing", "/healthz/"} {
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
				request := httptest.NewRequest(method, "http://127.0.0.1"+route, nil)
				request.Header.Set("Origin", origin)
				response := httptest.NewRecorder()
				helperHandler(nil, false, nil).ServeHTTP(response, request)
				if response.Code != http.StatusForbidden {
					t.Errorf("%s %s status=%d", method, route, response.Code)
				}
				for name, expected := range map[string]string{
					"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
					"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'", "Cache-Control": "no-store",
				} {
					if got := response.Header().Get(name); got != expected {
						t.Errorf("%s %s %s=%q", method, route, name, got)
					}
				}
			}
		}
	}
}

func TestHelperHostValidationRejectsNonLoopback(t *testing.T) {
	for _, badHost := range []string{"evil.com", "attacker.local:8028", "rebinding.com"} {
		for _, route := range []string{"/healthz", "/v1/commands"} {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+route, nil)
			request.Host = badHost
			response := httptest.NewRecorder()
			helperHandler(nil, false, nil).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Errorf("Host %q %s status=%d, want %d", badHost, route, response.Code, http.StatusBadRequest)
			}
			if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("Host %q %s X-Content-Type-Options=%q", badHost, route, got)
			}
		}
	}
}

func TestValidateListenerAllowsRemoteTLSWithAllowedHosts(t *testing.T) {
	// Non-loopback without an allowlist fails before any certificate work.
	if err := validateListener("0.0.0.0:8028", "missing-cert.pem", "missing-key.pem", nil); err == nil ||
		!strings.Contains(err.Error(), "--allowed-host") {
		t.Fatalf("expected --allowed-host requirement error, got %v", err)
	}
	// TLS is still mandatory off-loopback even with an allowlist.
	if err := validateListener("0.0.0.0:8028", "", "", []string{"studio.example.com"}); err == nil ||
		!strings.Contains(err.Error(), "--tls-cert") {
		t.Fatalf("expected TLS requirement error, got %v", err)
	}
	// Wildcard or malformed entries are rejected.
	if err := validateListener("0.0.0.0:8028", "missing-cert.pem", "missing-key.pem", []string{"*.example.com"}); err == nil ||
		!strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid allowlist entry error, got %v", err)
	}
	// Loopback stays unchanged: no TLS, no allowlist needed.
	if err := validateListener("127.0.0.1:8028", "", "", nil); err != nil {
		t.Fatalf("loopback listener rejected: %v", err)
	}
}

func TestHelperHandlerAllowsConfiguredRemoteHost(t *testing.T) {
	handler := helperHandler(nil, false, []string{"studio.example.com"})

	remote := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
	remote.Host = "studio.example.com:8028"
	remoteResponse := httptest.NewRecorder()
	handler.ServeHTTP(remoteResponse, remote)
	if remoteResponse.Code != http.StatusOK {
		t.Fatalf("configured remote Host rejected: got %d", remoteResponse.Code)
	}

	unlisted := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
	unlisted.Host = "evil.com"
	unlistedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unlistedResponse, unlisted)
	if unlistedResponse.Code != http.StatusBadRequest {
		t.Fatalf("unlisted Host accepted: got %d", unlistedResponse.Code)
	}

	loopback := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/healthz", nil)
	loopbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(loopbackResponse, loopback)
	if loopbackResponse.Code != http.StatusOK {
		t.Fatalf("loopback rejected: got %d", loopbackResponse.Code)
	}
}
