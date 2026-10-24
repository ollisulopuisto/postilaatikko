package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCreateICSFromDetails verifies that structured event details are
// serialized to a valid ICS file written to a temp location. This is the
// foundational handoff used by P-022 ("Add to Calendar" from email).
func TestCreateICSFromDetails(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	api := NewAPI(store, nil, nil, nil)

	start := time.Date(2026, 10, 25, 14, 0, 0, 0, time.UTC).Unix()
	end := time.Date(2026, 10, 25, 15, 0, 0, 0, time.UTC).Unix()

	path, err := api.CreateICSFromDetails(EventInput{
		Summary:     "Team Sync",
		Description: "Weekly team sync",
		Location:    "Conference Room A",
		DTStartUnix: start,
		DTEndUnix:   end,
		TZName:      "America/Los_Angeles",
	})
	if err != nil {
		t.Fatalf("CreateICSFromDetails: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty file path")
	}
	defer os.RemoveAll(filepath.Dir(path))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ics file: %v", err)
	}
	blob := string(data)

	if !strings.Contains(blob, "BEGIN:VCALENDAR") {
		t.Errorf("missing VCALENDAR marker:\n%s", blob)
	}
	if !strings.Contains(blob, "BEGIN:VEVENT") {
		t.Errorf("missing VEVENT marker:\n%s", blob)
	}
	if !strings.Contains(blob, "SUMMARY:Team Sync") {
		t.Errorf("missing SUMMARY:\n%s", blob)
	}
	if !strings.Contains(blob, "DESCRIPTION:Weekly team sync") {
		t.Errorf("missing DESCRIPTION:\n%s", blob)
	}
	if !strings.Contains(blob, "LOCATION:Conference Room A") {
		t.Errorf("missing LOCATION:\n%s", blob)
	}
}

// TestCreateICSFromDetailsValidation rejects incomplete input.
func TestCreateICSFromDetailsValidation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	api := NewAPI(store, nil, nil, nil)

	tests := []struct {
		name string
		in   EventInput
	}{
		{"empty summary", EventInput{DTStartUnix: 1, DTEndUnix: 2}},
		{"missing dtstart", EventInput{Summary: "X", DTEndUnix: 2}},
		{"missing dtend", EventInput{Summary: "X", DTStartUnix: 1}},
		{"dtend before dtstart", EventInput{Summary: "X", DTStartUnix: 10, DTEndUnix: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := api.CreateICSFromDetails(tc.in)
			if err == nil {
				t.Fatalf("expected error for %q", tc.name)
			}
		})
	}
}
