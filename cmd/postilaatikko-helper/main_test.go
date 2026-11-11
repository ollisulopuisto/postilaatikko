package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHelperHealthEndpoint(t *testing.T) {
	handler := helperHandler(nil, false)
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
	helperHandler(nil, true).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/healthz", nil))
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
				helperHandler(nil, false).ServeHTTP(response, request)
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
			helperHandler(nil, false).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Errorf("Host %q %s status=%d, want %d", badHost, route, response.Code, http.StatusBadRequest)
			}
			if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("Host %q %s X-Content-Type-Options=%q", badHost, route, got)
			}
		}
	}
}
