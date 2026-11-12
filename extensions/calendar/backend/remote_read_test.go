package backend

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRemoteCalendarRangeValidatesBeforeStoreAccess(t *testing.T) {
	var store *Store
	now := time.Now().Unix()
	for _, test := range []struct {
		ids      []string
		from, to int64
	}{
		{[]string{"one"}, now, now},
		{[]string{"one"}, now, now + 400*86400},
		{make([]string, 33), now, now + 86400},
		{[]string{""}, now, now + 86400},
	} {
		if _, err := store.RemoteEventsInRange(context.Background(), test.ids, test.from, test.to); err == nil {
			t.Fatal("invalid range accepted")
		}
	}
}

func TestRemoteCalendarRangeReadsLocalCalendarSnapshot(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	api := NewAPI(store, nil, nil, nil)
	source, err := api.AddLocalSource("Local")
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := api.AddLocalCalendar(source, "Personal", "#123456")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	id, err := api.CreateEvent(EventInput{CalendarID: calendar, Summary: "One event", DTStartUnix: now + 3600, DTEndUnix: now + 7200})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.RemoteEventsInRange(context.Background(), []string{calendar}, now, now+86400)
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatalf("read=%v err=%v", items, err)
	}
}

func TestRemoteCalendarReadRejectsOversizedStoredEvent(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	api := NewAPI(store, nil, nil, nil)
	source, err := api.AddLocalSource("Local")
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := api.AddLocalCalendar(source, "Personal", "#123456")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	id, err := api.CreateEvent(EventInput{CalendarID: calendar, Summary: "One", DTStartUnix: now + 3600, DTEndUnix: now + 7200})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec(`UPDATE events SET description=? WHERE id=?`, strings.Repeat("x", (1<<20)+1), id); err != nil {
		t.Fatal(err)
	}
	got, err := store.RemoteEventsInRange(context.Background(), []string{calendar}, now, now+86400)
	if err == nil || len(got) != 0 {
		t.Fatal("oversized event returned a partial success")
	}
}
