package backend

import (
	"strings"
	"testing"
	"time"
)

// DetectDateTimes recognizes date/time mentions in message text for the
// "Add to Calendar" handoff (P-022). Detection is deliberately conservative:
//
//   - an explicit four-digit year is required; relative dates ("tomorrow")
//     and year-less dates are skipped;
//   - numeric UTC offsets and UTC/GMT/Z designators are honored; ambiguous
//     named zones (PST, CST, ...) invalidate the mention entirely;
//   - a mention without a time becomes an all-day event (end = next-day
//     midnight); a time without an end defaults to a one-hour event;
//   - all-day dates anchor to midnight in the configured display timezone;
//     timed mentions anchor to the instant in the explicit zone, or the
//     configured display timezone when none is stated, and serialize as
//     UTC (TZName stays empty, instant semantics).
func TestDetectDateTimes(t *testing.T) {
	SetConfiguredTimezone("UTC")
	t.Cleanup(func() { SetConfiguredTimezone("") })

	utcMidnight := func(y int, m time.Month, d int) int64 {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix()
	}
	at := func(y int, m time.Month, d, h, min int) int64 {
		return time.Date(y, m, d, h, min, 0, 0, time.UTC).Unix()
	}

	tests := []struct {
		name    string
		text    string
		want    []DateTimeMention
		wantRaw string
	}{
		{
			name:    "month day year is all-day",
			text:    "Our review is on June 5, 2026.",
			want:    []DateTimeMention{{StartUnix: utcMidnight(2026, time.June, 5), EndUnix: utcMidnight(2026, time.June, 6), IsAllDay: true}},
			wantRaw: "June 5, 2026",
		},
		{
			name:    "day month year is all-day",
			text:    "Deadline 5 June 2026 for submissions.",
			want:    []DateTimeMention{{StartUnix: utcMidnight(2026, time.June, 5), EndUnix: utcMidnight(2026, time.June, 6), IsAllDay: true}},
			wantRaw: "5 June 2026",
		},
		{
			name:    "abbreviated month is recognized",
			text:    "Kickoff Jan 12 2027 at 09:00.",
			want:    []DateTimeMention{{StartUnix: at(2027, time.January, 12, 9, 0), EndUnix: at(2027, time.January, 12, 10, 0)}},
			wantRaw: "Jan 12 2027 at 09:00",
		},
		{
			name:    "iso date is all-day",
			text:    "Ship date 2026-06-05.",
			want:    []DateTimeMention{{StartUnix: utcMidnight(2026, time.June, 5), EndUnix: utcMidnight(2026, time.June, 6), IsAllDay: true}},
			wantRaw: "2026-06-05",
		},
		{
			name:    "iso date with time",
			text:    "Meeting 2026-06-05 14:00.",
			want:    []DateTimeMention{{StartUnix: at(2026, time.June, 5, 14, 0), EndUnix: at(2026, time.June, 5, 15, 0)}},
			wantRaw: "2026-06-05 14:00",
		},
		{
			name:    "12-hour time with meridiem",
			text:    "Sync June 5, 2026 at 2:30 PM.",
			want:    []DateTimeMention{{StartUnix: at(2026, time.June, 5, 14, 30), EndUnix: at(2026, time.June, 5, 15, 30)}},
			wantRaw: "June 5, 2026 at 2:30 PM",
		},
		{
			name: "range with shared meridiem",
			text: "Workshop June 5, 2026 2-4 pm.",
			want: []DateTimeMention{{
				StartUnix: at(2026, time.June, 5, 14, 0),
				EndUnix:   at(2026, time.June, 5, 16, 0),
			}},
			wantRaw: "June 5, 2026 2-4 pm",
		},
		{
			name:    "numeric offset anchors the instant",
			text:    "All-hands June 5, 2026 14:00+02:00.",
			want:    []DateTimeMention{{StartUnix: at(2026, time.June, 5, 12, 0), EndUnix: at(2026, time.June, 5, 13, 0)}},
			wantRaw: "June 5, 2026 14:00+02:00",
		},
		{
			name:    "UTC designator anchors the instant",
			text:    "Retro June 5, 2026 14:00 UTC.",
			want:    []DateTimeMention{{StartUnix: at(2026, time.June, 5, 14, 0), EndUnix: at(2026, time.June, 5, 15, 0)}},
			wantRaw: "June 5, 2026 14:00 UTC",
		},
		{
			name:    "12 AM is midnight",
			text:    "Launch June 5, 2026 at 12 AM.",
			want:    []DateTimeMention{{StartUnix: at(2026, time.June, 5, 0, 0), EndUnix: at(2026, time.June, 5, 1, 0)}},
			wantRaw: "June 5, 2026 at 12 AM",
		},
		{
			name:    "noon stays noon",
			text:    "Lunch June 5, 2026 at 12 PM.",
			want:    []DateTimeMention{{StartUnix: at(2026, time.June, 5, 12, 0), EndUnix: at(2026, time.June, 5, 13, 0)}},
			wantRaw: "June 5, 2026 at 12 PM",
		},
		{
			name: "named time zone invalidates the mention",
			text: "Call June 5, 2026 at 2 PM PST.",
			want: nil,
		},
		{
			name: "year-less date is skipped",
			text: "Party on June 5, bring cake.",
			want: nil,
		},
		{
			name: "relative dates are skipped",
			text: "Tomorrow at 3pm and next Tuesday.",
			want: nil,
		},
		{
			name: "ambiguous numeric dates are skipped",
			text: "Due 12/05/2026.",
			want: nil,
		},
		{
			name: "impossible dates are skipped",
			text: "Outing June 31, 2026.",
			want: nil,
		},
		{
			name: "impossible time is skipped",
			text: "Sync June 5, 2026 at 25:00.",
			want: nil,
		},
		{
			name: "multiple mentions return in order",
			text: "Plan June 5, 2026 and review July 2, 2026.",
			want: []DateTimeMention{
				{StartUnix: utcMidnight(2026, time.June, 5), EndUnix: utcMidnight(2026, time.June, 6), IsAllDay: true},
				{StartUnix: utcMidnight(2026, time.July, 2), EndUnix: utcMidnight(2026, time.July, 3), IsAllDay: true},
			},
			wantRaw: "June 5, 2026",
		},
		{
			name:    "duplicate instants are deduplicated",
			text:    "June 5, 2026 ... June 5, 2026 again.",
			want:    []DateTimeMention{{StartUnix: utcMidnight(2026, time.June, 5), EndUnix: utcMidnight(2026, time.June, 6), IsAllDay: true}},
			wantRaw: "June 5, 2026",
		},
		{
			name: "empty text yields nothing",
			text: "",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectDateTimes(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("DetectDateTimes() = %d mentions, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].StartUnix != tc.want[i].StartUnix {
					t.Errorf("mention %d StartUnix = %d, want %d", i, got[i].StartUnix, tc.want[i].StartUnix)
				}
				if got[i].EndUnix != tc.want[i].EndUnix {
					t.Errorf("mention %d EndUnix = %d, want %d", i, got[i].EndUnix, tc.want[i].EndUnix)
				}
				if got[i].IsAllDay != tc.want[i].IsAllDay {
					t.Errorf("mention %d IsAllDay = %v, want %v", i, got[i].IsAllDay, tc.want[i].IsAllDay)
				}
				if tc.wantRaw != "" && !strings.Contains(got[0].RawText, tc.wantRaw) {
					t.Errorf("mention 0 RawText = %q, want it to contain %q", got[0].RawText, tc.wantRaw)
				}
			}
		})
	}
}

// TestDetectDateTimesAnchorsAllDayInDisplayTimezone checks the all-day
// anchoring: the calendar DATE must stay on the mentioned day when rendered
// in the configured display timezone, even if that zone is far from UTC.
func TestDetectDateTimesAnchorsAllDayInDisplayTimezone(t *testing.T) {
	SetConfiguredTimezone("Pacific/Kiritimati") // UTC+14
	t.Cleanup(func() { SetConfiguredTimezone("") })

	loc, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Skip("zone data unavailable")
	}
	want := time.Date(2026, time.June, 5, 0, 0, 0, 0, loc).Unix()

	got := DetectDateTimes("Party June 5, 2026.")
	if len(got) != 1 || got[0].StartUnix != want {
		t.Fatalf("all-day anchoring: got %+v, want StartUnix %d", got, want)
	}
}

// TestCalendarBridgeDetectDateTimesDisabled verifies the disabled-extension
// gate: a bridge without a settings store reports no mentions and no error.
func TestCalendarBridgeDetectDateTimesDisabled(t *testing.T) {
	b := &CalendarBridge{}
	got, err := b.Calendar_DetectDateTimes("June 5, 2026")
	if err != nil {
		t.Fatalf("Calendar_DetectDateTimes: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("disabled bridge should return no mentions, got %+v", got)
	}
}
