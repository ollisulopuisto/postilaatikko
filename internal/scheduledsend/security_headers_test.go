package scheduledsend

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersAndOriginProtection(t *testing.T) {
	store, client, helper := baselineQueue(t)
	handler := store.Handler()

	// 1. Origin header rejected with 403 Forbidden
	reqOrigin := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/commands", bytes.NewReader([]byte("{}")))
	reqOrigin.Header.Set("Origin", "http://evil.com")
	respOrigin := httptest.NewRecorder()
	handler.ServeHTTP(respOrigin, reqOrigin)
	if respOrigin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for request with Origin header, got %d", respOrigin.Code)
	}
	assertHeaders(t, respOrigin.Header(), "on Origin rejection")

	// 2. Host header validation: reject non-loopback Host (DNS rebinding protection)
	for _, badHost := range []string{"evil.com", "attacker.local:8028", "rebinding.com"} {
		reqBadHost := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/commands", bytes.NewReader([]byte("{}")))
		reqBadHost.Host = badHost
		respBadHost := httptest.NewRecorder()
		handler.ServeHTTP(respBadHost, reqBadHost)
		if respBadHost.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for Host %q, got %d", badHost, respBadHost.Code)
		}
		assertHeaders(t, respBadHost.Header(), "on bad Host rejection")
	}

	// 3. 404 Not Found has security headers
	reqNotFound := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/unknown", bytes.NewReader([]byte("{}")))
	respNotFound := httptest.NewRecorder()
	handler.ServeHTTP(respNotFound, reqNotFound)
	if respNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", respNotFound.Code)
	}
	assertHeaders(t, respNotFound.Header(), "on 404")

	// 4. 405 Method Not Allowed has security headers
	reqMethod := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/v1/commands", nil)
	respMethod := httptest.NewRecorder()
	handler.ServeHTTP(respMethod, reqMethod)
	if respMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", respMethod.Code)
	}
	assertHeaders(t, respMethod.Header(), "on 405")

	// 5. Valid authenticated request has security headers
	cmd := baselineCommand()
	sealed := baselineSeal(t, cmd, client, helper)
	reqValid := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/commands", bytes.NewReader(sealed))
	respValid := httptest.NewRecorder()
	handler.ServeHTTP(respValid, reqValid)
	if respValid.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", respValid.Code)
	}
	assertHeaders(t, respValid.Header(), "on 200 OK")
}

func assertHeaders(t *testing.T, h http.Header, context string) {
	t.Helper()
	if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("[%s] expected X-Content-Type-Options: nosniff, got %q", context, got)
	}
	if got := h.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("[%s] expected X-Frame-Options: DENY, got %q", context, got)
	}
	if got := h.Get("Content-Security-Policy"); got != "default-src 'none'; frame-ancestors 'none'" {
		t.Errorf("[%s] expected Content-Security-Policy: default-src 'none'; frame-ancestors 'none', got %q", context, got)
	}
	if got := h.Get("Cache-Control"); got != "no-store" {
		t.Errorf("[%s] expected Cache-Control: no-store, got %q", context, got)
	}
}
