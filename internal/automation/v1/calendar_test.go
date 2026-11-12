package v1

import (
	"context"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type calendarReadFake struct{ calls int }

func (f *calendarReadFake) CalendarSources(context.Context) ([]calendar.Source, error) {
	f.calls++
	return []calendar.Source{}, nil
}
func (f *calendarReadFake) CalendarCalendars(context.Context, string) ([]calendar.Calendar, error) {
	f.calls++
	return []calendar.Calendar{}, nil
}
func (f *calendarReadFake) CalendarEvent(context.Context, string) (*calendar.Event, error) {
	f.calls++
	return &calendar.Event{ID: "one"}, nil
}
func (f *calendarReadFake) CalendarEvents(context.Context, CalendarRangeRequest) ([]calendar.EventInstance, error) {
	f.calls++
	return []calendar.EventInstance{}, nil
}
func TestCalendarRoutesRequireOwnReadGrant(t *testing.T) {
	ops := &calendarReadFake{}
	mux := http.NewServeMux()
	MountCalendarReadRoutes(mux, ops, func(w http.ResponseWriter, r *http.Request, c Capability) bool {
		if c != Capability("calendar.read") {
			t.Errorf("wrong capability %s", c)
		}
		http.Error(w, "no grant", 403)
		return false
	})
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest("GET", "/v1/calendar/sources", nil))
	if out.Code != 403 || ops.calls != 0 {
		t.Fatal("unauthorized request reached store")
	}
}
func TestCalendarRangeBoundsBeforeOperations(t *testing.T) {
	ops := &calendarReadFake{}
	mux := http.NewServeMux()
	MountCalendarReadRoutes(mux, ops, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	for _, body := range []string{`{"calendarIds":["one"],"fromUnix":1,"toUnix":9999999999}`, `{"calendarIds":[""],"fromUnix":1,"toUnix":2}`, `{"calendarIds":["one"],"fromUnix":1,"toUnix":2,"extra":true}`, `{"calendarIds":["one"],"fromUnix":1,"toUnix":2} {}`} {
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, httptest.NewRequest("POST", "/v1/calendar/events/query", strings.NewReader(body)))
		if out.Code != 400 {
			t.Fatalf("invalid input status %d", out.Code)
		}
	}
	if ops.calls != 0 {
		t.Fatal("invalid input reached store")
	}
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest("POST", "/v1/calendar/events/query", strings.NewReader(`{"calendarIds":["one"],"fromUnix":1,"toUnix":2}`)))
	if out.Code != 200 || ops.calls != 1 {
		t.Fatalf("valid query status=%d calls=%d", out.Code, ops.calls)
	}
}
func TestCalendarRangeRejectsOversizedWhitespaceBody(t *testing.T) {
	ops := &calendarReadFake{}
	mux := http.NewServeMux()
	MountCalendarReadRoutes(mux, ops, func(http.ResponseWriter, *http.Request, Capability) bool { return true })
	body := `{"calendarIds":["one"],"fromUnix":1,"toUnix":2}` + strings.Repeat(" ", 9000)
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest("POST", "/v1/calendar/events/query", strings.NewReader(body)))
	if out.Code != 400 || ops.calls != 0 {
		t.Fatalf("oversized query status=%d calls=%d", out.Code, ops.calls)
	}
}
