// Package recurrencebounded wraps the existing recurrence semantics with
// context and work checks inside generation, not around an uninterruptible call.
package recurrencebounded

import (
	"context"
	"errors"
	local "github.com/hkdb/aerion/internal/recurrencebounded/rrule"
	upstream "github.com/teambition/rrule-go"
	"sort"
	"time"
)

// Between expands an existing parsed recurrence set inclusively. Budgets are
// shared by the caller across events. Errors return no partial occurrence list.
func Between(ctx context.Context, set *upstream.Set, from, to time.Time, maxInstances int, budget *int) ([]time.Time, error) {
	if ctx == nil || set == nil || budget == nil || maxInstances < 0 {
		return nil, errors.New("recurrence requires context, set and nonnegative instance limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var stopped error
	check := func() bool {
		if stopped != nil {
			return false
		}
		if err := ctx.Err(); err != nil {
			stopped = err
			return false
		}
		if *budget <= 0 {
			stopped = errors.New("recurrence work budget exceeded")
			return false
		}
		*budget--
		return true
	}
	rawDates := set.GetRDate()
	exdates := set.GetExDate()
	if len(rawDates) > 10000 || len(exdates) > 10000 {
		return nil, errors.New("recurrence explicit-date budget exceeded")
	}
	rdates := append([]time.Time(nil), rawDates...)
	excluded := map[int64]bool{}
	for _, d := range exdates {
		if !check() {
			return nil, stopped
		}
		excluded[d.Unix()] = true
	}
	sort.Slice(rdates, func(i, j int) bool { return rdates[i].Before(rdates[j]) })
	var next local.Next
	if rule := set.GetRRule(); rule != nil {
		options, err := convertOptions(rule.OrigOptions)
		if err != nil {
			return nil, err
		}
		r, err := local.NewRRule(options)
		if err != nil {
			return nil, err
		}
		next = r.IteratorWithCheck(check)
	}
	out := []time.Time{}
	var last time.Time
	add := func(d time.Time) error {
		if !last.IsZero() && d.Equal(last) {
			return nil
		}
		last = d
		if d.Before(from) || d.After(to) || excluded[d.Unix()] {
			return nil
		}
		if len(out) >= maxInstances {
			return errors.New("recurrence instance budget exceeded")
		}
		out = append(out, d)
		return nil
	}
	var current time.Time
	var valid bool
	if next != nil {
		current, valid = next()
		if stopped != nil {
			return nil, stopped
		}
	}
	for valid || len(rdates) > 0 {
		if !check() {
			return nil, stopped
		}
		var d time.Time
		if !valid || len(rdates) > 0 && rdates[0].Before(current) {
			d = rdates[0]
			rdates = rdates[1:]
		} else {
			d = current
			if d.After(to) {
				break
			}
			current, valid = next()
			if stopped != nil {
				return nil, stopped
			}
		}
		if d.After(to) {
			break
		}
		if err := add(d); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func convertOptions(o upstream.ROption) (local.ROption, error) {
	// Upstream does not validate frequency or bound integer arithmetic, and its
	// non-standard BYEASTER offset indexes a year mask without range checks.
	if o.Freq < upstream.YEARLY || o.Freq > upstream.SECONDLY || o.Interval < 0 || o.Interval > 1000000 || len(o.Byeaster) > 0 {
		return local.ROption{}, errors.New("recurrence frequency, interval or BYEASTER unsupported by bounded evaluator")
	}
	// Bound Cartesian time-set construction before NewRRule, which runs before
	// iteration callbacks. Duplicate provider values do not get an unlimited product.
	if len(o.Byhour) > 24 || len(o.Byminute) > 60 || len(o.Bysecond) > 60 || len(o.Byweekday) > 366 || len(o.Bysetpos) > 366 || len(o.Bymonth) > 12 || len(o.Bymonthday) > 62 || len(o.Byyearday) > 732 || len(o.Byweekno) > 106 || len(o.Byeaster) > 366 {
		return local.ROption{}, errors.New("recurrence option budget exceeded")
	}
	product := 1
	for _, n := range []int{len(o.Byhour), len(o.Byminute), len(o.Bysecond)} {
		if n > 0 {
			product *= n
		}
	}
	if product > 10000 {
		return local.ROption{}, errors.New("recurrence time-set budget exceeded")
	}
	weekdays := make([]local.Weekday, len(o.Byweekday))
	for i, d := range o.Byweekday {
		weekdays[i] = local.WeekdayFromDay(d.Day(), d.N())
	}
	return local.ROption{Freq: local.Frequency(o.Freq), Dtstart: o.Dtstart, Interval: o.Interval, Wkst: local.WeekdayFromDay(o.Wkst.Day(), o.Wkst.N()), Count: o.Count, Until: o.Until, Bysetpos: o.Bysetpos, Bymonth: o.Bymonth, Bymonthday: o.Bymonthday, Byyearday: o.Byyearday, Byweekno: o.Byweekno, Byweekday: weekdays, Byhour: o.Byhour, Byminute: o.Byminute, Bysecond: o.Bysecond, Byeaster: o.Byeaster}, nil
}

// ValidateRuleText bounds option parsing and Cartesian construction before the
// existing ICS library constructs its recurrence set. Call for every RRULE.
func ValidateRuleText(text string) error {
	if len(text) > 8192 {
		return errors.New("recurrence rule text exceeds budget")
	}
	options, err := upstream.StrToROption(text)
	if err != nil {
		return err
	}
	_, err = convertOptions(*options)
	return err
}
