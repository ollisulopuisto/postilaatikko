package app

import (
	"context"
	"encoding/json"
	calendar "github.com/hkdb/aerion/extensions/calendar/backend"
	automationclient "github.com/hkdb/aerion/internal/automation/client"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/settings"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteCalendarReadDoesNotOpenLocalMailbox(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	deviceSettings := settings.NewStore(db)
	if err := deviceSettings.SetExtensionEnabled("calendar", true); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/calendar/events/query" {
			if r.Method != "POST" {
				t.Error("wrong occurrence method")
			}
			_ = json.NewEncoder(w).Encode(v1.Response[[]calendar.EventInstance]{Data: []calendar.EventInstance{{Event: calendar.Event{ID: "occ", DescriptionHTML: "<script>untrusted</script>"}}}})
			return
		}
		if r.URL.Path == "/v1/calendar/events/plain" {
			_ = json.NewEncoder(w).Encode(v1.Response[*calendar.Event]{Data: &calendar.Event{ID: "plain", Description: "One line\nAnother https://example.com", DescriptionHTML: "<b>untrusted</b>"}})
			return
		}
		if r.URL.Path == "/v1/calendar/events/one" {
			_ = json.NewEncoder(w).Encode(v1.Response[*calendar.Event]{Data: &calendar.Event{ID: "one", Description: "<p>safe<script>alert(1)</script></p>", DescriptionHTML: "<script>remote</script>"}})
			return
		}
		_ = json.NewEncoder(w).Encode(v1.Response[[]calendar.Source]{Data: []calendar.Source{{ID: "source"}}})
	}))
	defer server.Close()
	a := &App{settingsStore: deviceSettings, ctx: context.Background(), remoteServiceMode: true, mailboxService: &mailboxServiceConnector{client: automationclient.New(automationclient.Config{Endpoint: server.URL, Token: "test"})}}
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
	if err := deviceSettings.SetExtensionEnabled("calendar", false); err != nil {
		t.Fatal(err)
	}
	before := calls
	if sources, err := a.Calendar_ListSources(); err != nil || len(sources) > 0 {
		t.Fatal("disabled Calendar returned remote sources")
	}
	if cals, err := a.Calendar_ListCalendars("one"); err != nil || len(cals) > 0 {
		t.Fatal("disabled Calendar returned remote calendars")
	}
	if event, err := a.Calendar_GetEvent("one"); err != nil || event != nil {
		t.Fatal("disabled Calendar returned remote event")
	}
	if items, err := a.Calendar_ListEventsInRange([]string{"one"}, 1, 2); err != nil || len(items) > 0 {
		t.Fatal("disabled Calendar returned occurrences")
	}
	if calls != before {
		t.Fatal("disabled Calendar called helper")
	}
	if err := deviceSettings.SetExtensionEnabled("calendar", true); err != nil {
		t.Fatal(err)
	}
	if items, err := a.Calendar_ListEventsInRange([]string{"one"}, 1, 2); err != nil || len(items) != 1 || items[0].DescriptionHTML != "" {
		t.Fatalf("remote occurrences=%v %v", items, err)
	}
}
