package scheduledsend

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func helsinki(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	return location
}

func TestRepeatMonthlyClampsAndKeepsWallClock(t *testing.T) {
	location := helsinki(t)
	first := time.Date(2026, time.January, 31, 9, 0, 0, 0, location)
	rule := Repeat{Unit: "month", Every: 1, Zone: "Europe/Helsinki"}
	want := []time.Time{
		time.Date(2026, time.February, 28, 9, 0, 0, 0, location),
		time.Date(2026, time.March, 31, 9, 0, 0, 0, location),
		time.Date(2026, time.April, 30, 9, 0, 0, 0, location),
	}
	for index, expected := range want {
		if got := rule.occurrence(first.UTC(), index+1); !got.Equal(expected) {
			t.Fatalf("run %d: got %s want %s", index+1, got.In(location), expected)
		}
	}
	// March 31 is after the DST change: 09:00 local is 06:00 UTC, not 07:00.
	if got := rule.occurrence(first.UTC(), 2); got.UTC().Hour() != 6 {
		t.Fatalf("wall clock drifted across DST: %s", got)
	}
}

func TestRepeatWeekAndDay(t *testing.T) {
	first := time.Date(2026, time.November, 16, 9, 0, 0, 0, time.UTC)
	if got := (Repeat{Unit: "week", Every: 2, Zone: "UTC"}).occurrence(first, 1); !got.Equal(first.AddDate(0, 0, 14)) {
		t.Fatal(got)
	}
	if got := (Repeat{Unit: "day", Every: 3, Zone: "UTC"}).occurrence(first, 2); !got.Equal(first.AddDate(0, 0, 6)) {
		t.Fatal(got)
	}
}

func TestRepeatNextSkipsMissedRunsAndHonoursLimits(t *testing.T) {
	first := time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)
	rule := Repeat{Unit: "day", Every: 1, Zone: "UTC"}
	index, at, ok := rule.next(first, 0, first.Add(10*24*time.Hour+time.Hour))
	if !ok || index != 11 || !at.Equal(first.AddDate(0, 0, 11)) {
		t.Fatalf("missed runs should be skipped: %d %s %v", index, at, ok)
	}
	rule.Count = 3
	if _, _, ok := rule.next(first, 2, first); ok {
		t.Fatal("count of 3 allows only runs 0..2")
	}
	if _, _, ok := rule.next(first, 1, first); !ok {
		t.Fatal("run 2 should remain")
	}
	until := first.AddDate(0, 0, 1)
	rule = Repeat{Unit: "day", Every: 1, Zone: "UTC", Until: &until}
	if _, _, ok := rule.next(first, 0, first); !ok {
		t.Fatal("run on the end instant is allowed")
	}
	if _, _, ok := rule.next(first, 1, first); ok {
		t.Fatal("run after the end must not be queued")
	}
}

func TestRepeatValidation(t *testing.T) {
	first := time.Now().Add(time.Hour)
	for name, rule := range map[string]Repeat{
		"unit":  {Unit: "year", Every: 1, Zone: "UTC"},
		"every": {Unit: "day", Every: 0, Zone: "UTC"},
		"zone":  {Unit: "day", Every: 1, Zone: "Not/AZone"},
		"count": {Unit: "day", Every: 1, Zone: "UTC", Count: 5000},
	} {
		if err := rule.Validate(first); err == nil {
			t.Fatalf("%s should be rejected", name)
		}
	}
	if err := (Repeat{Unit: "month", Every: 1, Zone: "UTC"}).Validate(first); err != nil {
		t.Fatal(err)
	}
}

func TestOccurrenceMessageRewritesIdentityHeaders(t *testing.T) {
	raw := []byte("From: a@example.com\r\nMessage-ID: <first@example.com>\r\nDate: Mon, 02 Nov 2026 09:00:00 +0200\r\nSubject: Pay up\r\n\tfolded\r\n\r\nBody\r\n")
	if string(occurrenceMessage(raw, "job", 0, 0, time.Now())) != string(raw) {
		t.Fatal("first run must be unchanged")
	}
	at := time.Date(2026, time.December, 1, 7, 0, 0, 0, time.UTC)
	out := string(occurrenceMessage(raw, "job", 0, 1, at))
	if strings.Contains(out, "first@example.com") || strings.Contains(out, "02 Nov") {
		t.Fatalf("old identity headers kept: %q", out)
	}
	for _, want := range []string{"@example.com>", "Date: Tue, 01 Dec 2026 07:00:00 +0000", "Subject: Pay up\r\n\tfolded", "\r\n\r\nBody\r\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if out != string(occurrenceMessage(raw, "job", 0, 1, at)) {
		t.Fatal("message ID must be stable for a retry of the same run")
	}
	if out == string(occurrenceMessage(raw, "job", 0, 2, at)) {
		t.Fatal("each run needs its own message ID")
	}
}

func TestRepeatingJobRequeuesAfterEachRun(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	command.Delivery.DueAt = time.Now().Add(-time.Minute)
	command.Delivery.Repeat = &Repeat{Unit: "day", Every: 1, Zone: "UTC", Count: 3}
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	var ids []string
	worker := Worker{Store: store, Transport: baselineTransport(func(_ context.Context, delivery Delivery) error {
		for _, line := range strings.Split(string(delivery.RFC822), "\r\n") {
			if strings.HasPrefix(strings.ToLower(line), "message-id:") {
				ids = append(ids, line)
			}
		}
		return nil
	})}
	if processed, err := worker.ProcessNext(ctx); err != nil || !processed {
		t.Fatalf("run 0: %v %v", processed, err)
	}
	status, err := store.GetStatus(ctx, command.JobID)
	if err != nil || status.State != Queued || status.Attempts != 0 || !status.DueAt.After(time.Now()) {
		t.Fatalf("after run 0: %+v %v", status, err)
	}
	if processed, _ := worker.ProcessNext(ctx); processed {
		t.Fatal("next run is not due yet")
	}
	for step := 1; step <= 2; step++ {
		if _, err := store.db.Exec("UPDATE jobs SET next_attempt = 0 WHERE id = ?", command.JobID); err != nil {
			t.Fatal(err)
		}
		if processed, err := worker.ProcessNext(ctx); err != nil || !processed {
			t.Fatalf("run %d: %v %v", step, processed, err)
		}
	}
	status, err = store.GetStatus(ctx, command.JobID)
	if err != nil || status.State != Sent {
		t.Fatalf("series should end Sent after 3 runs: %+v %v", status, err)
	}
	if len(ids) != 3 || ids[0] == ids[1] || ids[1] == ids[2] {
		t.Fatalf("each run needs a distinct Message-ID: %v", ids)
	}
	var payload []byte
	if err := store.db.QueryRow("SELECT envelope FROM jobs WHERE id = ?", command.JobID).Scan(&payload); err != nil || len(payload) != 0 {
		t.Fatalf("payload must be cleared when the series ends: %d %v", len(payload), err)
	}
}

func TestRepeatingJobStopsOnFailureAndCancels(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	command.Delivery.Repeat = &Repeat{Unit: "day", Every: 1, Zone: "UTC"}
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Store: store, Transport: baselineTransport(func(context.Context, Delivery) error {
		return &DeliveryError{Kind: PermanentFailure, Err: errors.New("rejected")}
	})}
	if _, err := worker.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ := store.GetStatus(ctx, command.JobID)
	if status.State != Failed {
		t.Fatalf("a failed run must stop the series: %+v", status)
	}

	cancel := Command{Version: ProtocolVersion, JobID: command.JobID, Revision: 2, Action: "cancel"}
	if _, err := store.Accept(ctx, baselineSeal(t, cancel, client, helper)); err != nil {
		t.Fatal(err)
	}
	status, _ = store.GetStatus(ctx, command.JobID)
	if status.State != Cancelled {
		t.Fatalf("cancel: %+v", status)
	}
}

func TestRepeatingJobCancelBetweenRuns(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	command.Delivery.Repeat = &Repeat{Unit: "week", Every: 1, Zone: "UTC"}
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Store: store, Transport: baselineTransport(func(context.Context, Delivery) error { return nil })}
	if _, err := worker.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	cancel := Command{Version: ProtocolVersion, JobID: command.JobID, Revision: 2, Action: "cancel"}
	if _, err := store.Accept(ctx, baselineSeal(t, cancel, client, helper)); err != nil {
		t.Fatalf("a queued series must be cancellable: %v", err)
	}
	status, _ := store.GetStatus(ctx, command.JobID)
	if status.State != Cancelled {
		t.Fatalf("%+v", status)
	}
}

func TestRestartedSeriesNeverReusesAnEarlierMessageID(t *testing.T) {
	ctx := context.Background()
	store, client, helper := baselineQueue(t)
	command := baselineCommand()
	command.Delivery.Repeat = &Repeat{Unit: "week", Every: 1, Zone: "UTC"}
	var ids []string
	worker := Worker{Store: store, Transport: baselineTransport(func(_ context.Context, delivery Delivery) error {
		for _, line := range strings.Split(string(delivery.RFC822), "\r\n") {
			if strings.HasPrefix(strings.ToLower(line), "message-id:") {
				ids = append(ids, line)
			}
		}
		return nil
	})}
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	// Replace the series (what Reschedule does) with a due time that is already past.
	command.Revision = 2
	command.Delivery.DueAt = time.Now().Add(-time.Minute)
	if _, err := store.Accept(ctx, baselineSeal(t, command, client, helper)); err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.ProcessNext(ctx); err != nil || !processed {
		t.Fatalf("restarted run: %v %v", processed, err)
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("a restarted series reused a Message-ID: %v", ids)
	}
}
