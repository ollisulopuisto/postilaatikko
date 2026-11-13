package client

import (
	"context"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCalendarClientTypedReadRoutes(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/calendar/events/one" {
			_, _ = w.Write([]byte(`{"data":{"id":"one"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	c := New(Config{Endpoint: server.URL, Token: "test"})
	ctx := context.Background()
	if _, err := c.CalendarSources(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CalendarCalendars(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if ev, err := c.CalendarEvent(ctx, "one"); err != nil || ev.ID != "one" {
		t.Fatalf("event=%v err=%v", ev, err)
	}
	if _, err := c.CalendarEvents(ctx, v1.CalendarRangeRequest{CalendarIDs: []string{"one"}, FromUnix: 1, ToUnix: 2}); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /v1/calendar/sources", "GET /v1/calendar/calendars?sourceId=source", "GET /v1/calendar/events/one", "POST /v1/calendar/events/query"}
	for i, p := range want {
		if paths[i] != p {
			t.Fatalf("path[%d]=%s", i, paths[i])
		}
	}
}
func TestCalendarClientInvalidRangeDoesNotRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("invalid query reached helper") }))
	defer server.Close()
	c := New(Config{Endpoint: server.URL})
	if _, err := c.CalendarEvents(context.Background(), v1.CalendarRangeRequest{}); err == nil {
		t.Fatal("invalid range accepted")
	}
}
