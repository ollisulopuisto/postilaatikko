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
	rules := []string{"FREQ=DAILY;COUNT=5", "FREQ=DAILY;INTERVAL=2;COUNT=10", "FREQ=MONTHLY;BYMONTHDAY=-1;COUNT=5", "FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1;COUNT=5", "FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=29;COUNT=2", "FREQ=WEEKLY;BYDAY=SU;WKST=SU;COUNT=8"}
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
