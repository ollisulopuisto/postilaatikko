package app

import (
	"context"
	"encoding/json"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteCalendarReadDoesNotOpenLocalMailbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/calendar/events/query" {
			if r.Method != "POST" {
				t.Error("wrong occurrence method")
			}
			json.NewEncoder(w).Encode(v1.Response[[]calendar.EventInstance]{Data: []calendar.EventInstance{{Event: calendar.Event{ID: "occ", DescriptionHTML: "<script>untrusted</script>"}}}})
			return
		}
		if r.URL.Path == "/v1/calendar/events/plain" {
			json.NewEncoder(w).Encode(v1.Response[*calendar.Event]{Data: &calendar.Event{ID: "plain", Description: "One line\nAnother https://example.com", DescriptionHTML: "<b>untrusted</b>"}})
			return
		}
		if r.URL.Path == "/v1/calendar/events/one" {
			json.NewEncoder(w).Encode(v1.Response[*calendar.Event]{Data: &calendar.Event{ID: "one", Description: "<p>safe<script>alert(1)</script></p>", DescriptionHTML: "<script>remote</script>"}})
			return
		}
		json.NewEncoder(w).Encode(v1.Response[[]calendar.Source]{Data: []calendar.Source{{ID: "source"}}})
	}))
	defer server.Close()
	a := &App{ctx: context.Background(), remoteServiceMode: true, mailboxService: &mailboxServiceConnector{client: automationclient.New(automationclient.Config{Endpoint: server.URL, Token: "test"})}}
	sources, err := a.Calendar_ListSources()
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources=%v %v", sources, err)
	}
	event, err := a.Calendar_GetEvent("one")
	if err != nil || strings.Contains(strings.ToLower(event.DescriptionHTML), "script") {
		t.Fatalf("trusted remote HTML: %v %v", event, err)
	}
	plain, err := a.Calendar_GetEvent("plain")
	if err != nil || plain.DescriptionHTML != "" || !strings.Contains(plain.Description, "\n") {
		t.Fatalf("plaintext render changed: %+v %v", plain, err)
	}
	if a.db != nil || a.credStore != nil || a.CalendarBridge != nil {
		t.Fatal("opened local Calendar/mailbox dependencies")
	}
	if items, err := a.Calendar_ListEventsInRange([]string{"one"}, 1, 2); err != nil || len(items) != 1 || items[0].DescriptionHTML != "" {
		t.Fatalf("remote occurrences=%v %v", items, err)
	}
}
