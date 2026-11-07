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
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
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
	helperHandler(nil, true).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}
