package backend

import (
	"context"
	"testing"
	"time"
)

func TestBoundedRecurrenceRejectsExcessWorkWithoutPartialSuccess(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := Event{ID: "dense", DTStartUnix: start.Unix(), DTEndUnix: start.Add(time.Second).Unix(), RRuleText: "FREQ=SECONDLY", ICSBlob: "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:dense\r\nDTSTART:20260101T000000Z\r\nDTEND:20260101T000001Z\r\nRRULE:FREQ=SECONDLY\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"}
	got, err := ExpandInRangeBounded(context.Background(), ev, nil, start, start.Add(time.Hour), 10, 30)
	if err == nil || len(got) != 0 {
		t.Fatalf("dense recurrence silently returned partial data: %d %v", len(got), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExpandInRangeBounded(ctx, ev, nil, start, start.Add(time.Hour), 10, 30); err == nil {
		t.Fatal("cancelled work continued")
	}
}
func TestBoundedRecurrenceMatchesOrdinaryDailyExpansion(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := Event{ID: "daily", DTStartUnix: start.Unix(), DTEndUnix: start.Add(time.Hour).Unix(), RRuleText: "FREQ=DAILY;COUNT=3", ICSBlob: "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:daily\r\nDTSTART:20260101T000000Z\r\nDTEND:20260101T010000Z\r\nRRULE:FREQ=DAILY;COUNT=3\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"}
	got, err := ExpandInRangeBounded(context.Background(), ev, nil, start, start.AddDate(0, 0, 4), 10, 30)
	if err != nil || len(got) != 3 {
		t.Fatalf("daily expansion=%d %v", len(got), err)
	}
}

func TestRecurrenceBudgetSharedAcrossEvents(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := Event{RRuleText: "FREQ=DAILY;COUNT=3", ICSBlob: "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:daily\r\nDTSTART:20260101T000000Z\r\nDTEND:20260101T010000Z\r\nRRULE:FREQ=DAILY;COUNT=3\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"}
	remaining := 100000
	if _, err := expandInRange(context.Background(), ev, nil, start, start.AddDate(0, 0, 4), 10, &remaining); err != nil {
		t.Fatal(err)
	}
	used := 100000 - remaining
	remaining = used + used/2
	if _, err := expandInRange(context.Background(), ev, nil, start, start.AddDate(0, 0, 4), 10, &remaining); err != nil {
		t.Fatal(err)
	}
	if got, err := expandInRange(context.Background(), ev, nil, start, start.AddDate(0, 0, 4), 10, &remaining); err == nil || len(got) != 0 {
		t.Fatal("second event bypassed shared budget")
	}
}

func TestBoundedRecurrenceStopsInsideImpossibleRule(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := Event{RRuleText: "FREQ=HOURLY;INTERVAL=2;BYHOUR=1", ICSBlob: "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:impossible\r\nDTSTART:20260101T000000Z\r\nDTEND:20260101T010000Z\r\nRRULE:FREQ=HOURLY;INTERVAL=2;BYHOUR=1\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExpandInRangeBounded(ctx, ev, nil, start, start.Add(time.Hour), 10, 500); err != context.Canceled {
		t.Fatal(err)
	}
	// The finite work budget, not an outside timeout goroutine, stops next().
	if got, err := ExpandInRangeBounded(context.Background(), ev, nil, start, start.Add(time.Hour), 10, 500); err == nil || len(got) != 0 {
		t.Fatalf("impossible rule=%v err=%v", got, err)
	}
}
