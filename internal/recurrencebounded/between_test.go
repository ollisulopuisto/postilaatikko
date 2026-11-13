package recurrencebounded

import (
	"context"
	upstream "github.com/teambition/rrule-go"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBetweenMatchesRecurrenceAndDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 3, 27, 9, 0, 0, 0, loc)
	rules := []string{"FREQ=DAILY;COUNT=7", "FREQ=WEEKLY;BYDAY=MO,WE,FR;COUNT=10", "FREQ=MONTHLY;BYDAY=1MO;COUNT=6", "FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU;COUNT=3", "FREQ=HOURLY;COUNT=12", "FREQ=MINUTELY;INTERVAL=15;COUNT=8", "FREQ=SECONDLY;INTERVAL=10;COUNT=6"}
	for _, text := range rules {
		t.Run(text, func(t *testing.T) {
			options, err := upstream.StrToROptionInLocation(text, loc)
			if err != nil {
				t.Fatal(err)
			}
			options.Dtstart = start
			r, err := upstream.NewRRule(*options)
			if err != nil {
				t.Fatal(err)
			}
			set := &upstream.Set{}
			set.RRule(r)
			set.RDate(start.Add(time.Hour))
			set.ExDate(start)
			end := start.AddDate(4, 0, 0)
			want := set.Between(start, end, true)
			budget := 100000
			got, err := Between(context.Background(), set, start, end, 1000, &budget)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got=%v want=%v err=%v", got, want, err)
			}
		})
	}
}
func TestImpossibleInternalLoopExhaustsBudget(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, err := upstream.NewRRule(upstream.ROption{Freq: upstream.HOURLY, Interval: 2, Byhour: []int{1}, Dtstart: start})
	if err != nil {
		t.Fatal(err)
	}
	set := &upstream.Set{}
	set.RRule(r)
	budget := 500
	got, err := Between(context.Background(), set, start, start.AddDate(0, 0, 1), 100, &budget)
	if err == nil || len(got) != 0 || budget > 0 {
		t.Fatalf("impossible rule bypassed budget: %v %d", err, budget)
	}
}
func TestCancelledBeforeGeneration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget := 100
	if _, err := Between(ctx, &upstream.Set{}, time.Now(), time.Now().Add(time.Hour), 10, &budget); err != context.Canceled {
		t.Fatalf("err=%v", err)
	}
}

func TestRuleConstructionRejectsDuplicateCartesianExplosion(t *testing.T) {
	if err := ValidateRuleText("FREQ=DAILY;BYHOUR=" + strings.Repeat("1,", 10000) + "1"); err == nil {
		t.Fatal("oversized pre-construction options accepted")
	}
}
func TestCalendarBoundaryParity(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Helsinki")
	starts := []time.Time{time.Date(2026, 10, 23, 3, 30, 0, 0, loc), time.Date(2024, 2, 28, 9, 0, 0, 0, loc), time.Date(2026, 3, 27, 9, 0, 0, 0, loc)}
	rules := []string{"FREQ=DAILY;COUNT=5", "FREQ=DAILY;INTERVAL=2;COUNT=10", "FREQ=MONTHLY;BYMONTHDAY=-1;COUNT=5", "FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1;COUNT=5", "FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=29;COUNT=2", "FREQ=WEEKLY;BYDAY=SU;WKST=SU;COUNT=8", "FREQ=DAILY;UNTIL=20261101T000000Z"}
	for _, start := range starts {
		for _, text := range rules {
			options, err := upstream.StrToROptionInLocation(text, loc)
			if err != nil {
				t.Fatal(err)
			}
			options.Dtstart = start
			r, err := upstream.NewRRule(*options)
			if err != nil {
				t.Fatal(err)
			}
			set := &upstream.Set{}
			set.RRule(r)
			set.RDate(start)
			set.RDate(start)
			set.ExDate(start.AddDate(0, 0, 2))
			for _, bounds := range [][2]time.Time{{start, start.AddDate(5, 0, 0)}, {start.AddDate(0, 0, 1), start.AddDate(0, 0, 3)}, {start, start}} {
				want := set.Between(bounds[0], bounds[1], true)
				budget := 100000
				got, err := Between(context.Background(), set, bounds[0], bounds[1], 1000, &budget)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("start=%v rule=%s got=%v want=%v err=%v", start, text, got, want, err)
				}
			}
		}
	}
}
func TestInternalCancellationAndOutputLimitReturnNoPartialData(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, _ := upstream.NewRRule(upstream.ROption{Freq: upstream.SECONDLY, Dtstart: start})
	set := &upstream.Set{}
	set.RRule(r)
	budget := 10000
	got, err := Between(context.Background(), set, start, start.Add(time.Hour), 2, &budget)
	if err == nil || len(got) > 0 {
		t.Fatal("partial success after output limit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget = 10000
	if _, err := Between(ctx, set, start, start.Add(time.Hour), 2, &budget); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestUnsafeOptionsRejectedBeforeConstruction(t *testing.T) {
	for _, rule := range []string{"FREQ=DAILY;BYEASTER=99999", "FREQ=WEEKLY;INTERVAL=9223372036854775807", "FREQ=DAILY;BYHOUR=" + strings.Repeat("1,", 24) + "1", "FREQ=DAILY;BYMINUTE=" + strings.Repeat("1,", 59) + "1;BYSECOND=" + strings.Repeat("1,", 59) + "1;BYHOUR=0,1,2"} {
		if err := ValidateRuleText(rule); err == nil {
			t.Fatalf("unsafe rule accepted: %s", rule)
		}
	}
}

// A deterministic context cancels while next() is inside an impossible loop.
// No timeout goroutine (and hence no leaked evaluator) is used by this test.
type cancelAfterChecks struct {
	context.Context
	calls int
}

func (c *cancelAfterChecks) Err() error {
	c.calls++
	if c.calls > 20 {
		return context.Canceled
	}
	return nil
}
func TestCancellationInsideIterator(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, err := upstream.NewRRule(upstream.ROption{Freq: upstream.MINUTELY, Interval: 2, Byminute: []int{1}, Dtstart: start})
	if err != nil {
		t.Fatal(err)
	}
	set := &upstream.Set{}
	set.RRule(r)
	budget := 10000
	ctx := &cancelAfterChecks{Context: context.Background()}
	got, err := Between(ctx, set, start, start.Add(time.Hour), 100, &budget)
	if err != context.Canceled || len(got) != 0 || budget <= 0 {
		t.Fatalf("inner cancellation: %v budget=%d", err, budget)
	}
}

func TestOutOfMonthOrdinalDoesNotPanic(t *testing.T) {
	for _, start := range []time.Time{time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)} {
		for _, freq := range []upstream.Frequency{upstream.MONTHLY, upstream.YEARLY} {
			for _, n := range []int{-53, -5, 5, 53} {
				options := upstream.ROption{Freq: freq, Dtstart: start, Byweekday: []upstream.Weekday{upstream.MO.Nth(n)}, Count: 2, Until: start.AddDate(0, 2, 0)}
				if freq == upstream.YEARLY {
					options.Bymonth = []int{int(start.Month())}
				}
				r, err := upstream.NewRRule(options)
				if err != nil {
					t.Fatal(err)
				}
				set := &upstream.Set{}
				set.RRule(r)
				budget := 100000
				got, err := Between(context.Background(), set, start, start.AddDate(0, 1, 0), 10, &budget)
				// An impossible ordinal may run until the work budget; it must return
				// an explicit error, no partial list, and never panic during rebuild.
				if err != nil && (len(got) != 0 || !strings.Contains(err.Error(), "work budget")) {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestZeroRemainingCapacityAllowsEmptySeriesOnly(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, inside := range []bool{false, true} {
		dt := start.AddDate(0, 1, 0)
		if inside {
			dt = start
		}
		r, err := upstream.NewRRule(upstream.ROption{Freq: upstream.DAILY, Count: 1, Dtstart: dt})
		if err != nil {
			t.Fatal(err)
		}
		set := &upstream.Set{}
		set.RRule(r)
		budget := 10000
		got, err := Between(context.Background(), set, start, start.Add(time.Hour), 0, &budget)
		if inside && (err == nil || len(got) != 0) {
			t.Fatal("over-capacity occurrence accepted")
		}
		if !inside && (err != nil || len(got) != 0) {
			t.Fatalf("empty series at capacity: %v %v", got, err)
		}
	}
}
