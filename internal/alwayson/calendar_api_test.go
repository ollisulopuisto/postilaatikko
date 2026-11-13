package alwayson

import (
	"context"
	v1 "github.com/hkdb/aerion/internal/automation/v1"
	"github.com/hkdb/aerion/internal/database"
	"path/filepath"
	"testing"
)

func TestCalendarReadRuntimeClosesWithoutStartingProviders(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	r := newCalendarReadRuntime(db, t.TempDir())
	if err := r.settings.SetExtensionEnabled("calendar", true); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sources, err := r.CalendarSources(ctx)
	if err != nil || len(sources) != 0 {
		t.Fatalf("sources=%v err=%v", sources, err)
	}
	if _, err := r.CalendarEvents(ctx, v1.CalendarRangeRequest{CalendarIDs: []string{"one"}, FromUnix: 1, ToUnix: 2}); err == nil {
		t.Fatal("unbounded recurrence route became enabled")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CalendarSources(ctx); err == nil {
		t.Fatal("read reopened closed runtime")
	}
}
