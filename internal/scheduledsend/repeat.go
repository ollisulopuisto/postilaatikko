package scheduledsend

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Repeat describes a recurring schedule for one prepared message. The first
// run is Delivery.DueAt; run n is computed from it in Zone so that a 09:00 run
// stays at 09:00 across daylight-saving changes and a monthly run on the 31st
// lands on the last day of shorter months, without drift.
type Repeat struct {
	Unit  string     `json:"unit"` // day, week or month
	Every int        `json:"every"`
	Zone  string     `json:"zone"`            // IANA time zone name
	Count int        `json:"count,omitempty"` // total runs including the first; 0 = no limit
	Until *time.Time `json:"until,omitempty"` // no run after this instant
}

const (
	maxRepeatEvery = 366
	maxRepeatCount = 1000
	maxRepeatSkips = 100000
)

// Validate checks a rule against the time of its first run.
func (r Repeat) Validate(first time.Time) error {
	switch r.Unit {
	case "day", "week", "month":
	default:
		return errors.New("repeat unit must be day, week or month")
	}
	if r.Every < 1 || r.Every > maxRepeatEvery {
		return errors.New("repeat interval must be between 1 and 366")
	}
	if r.Count < 0 || r.Count > maxRepeatCount {
		return errors.New("repeat count must be between 0 and 1000")
	}
	if len(r.Zone) == 0 || len(r.Zone) > 64 {
		return errors.New("repeat requires a time zone name")
	}
	if _, err := time.LoadLocation(r.Zone); err != nil {
		return errors.New("repeat time zone is not recognised")
	}
	if r.Until != nil && !r.Until.After(first) {
		return errors.New("repeat end must be after the first run")
	}
	return nil
}

// Summary is a compact machine-readable form such as "month:1" for previews.
func (r Repeat) Summary() string { return fmt.Sprintf("%s:%d", r.Unit, r.Every) }

// occurrence returns the UTC time of run n (0 is the first run).
func (r Repeat) occurrence(first time.Time, n int) time.Time {
	location, err := time.LoadLocation(r.Zone)
	if err != nil {
		location = time.UTC
	}
	base := first.In(location)
	year, month, day := base.Date()
	hour, minute, second := base.Clock()
	steps := n * r.Every
	switch r.Unit {
	case "day":
		return time.Date(year, month, day+steps, hour, minute, second, 0, location).UTC()
	case "week":
		return time.Date(year, month, day+7*steps, hour, minute, second, 0, location).UTC()
	default:
		target := time.Date(year, month+time.Month(steps), 1, 0, 0, 0, 0, location)
		last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, location).Day()
		return time.Date(target.Year(), target.Month(), min(day, last), hour, minute, second, 0, location).UTC()
	}
}

// next returns the first run after index current that is later than now. Runs
// missed while the helper was offline are skipped rather than sent in a burst.
// ok is false when the series has ended.
func (r Repeat) next(first time.Time, current int, now time.Time) (index int, at time.Time, ok bool) {
	for index = current + 1; index-current <= maxRepeatSkips; index++ {
		if r.Count > 0 && index >= r.Count {
			return 0, time.Time{}, false
		}
		at = r.occurrence(first, index)
		if r.Until != nil && at.After(*r.Until) {
			return 0, time.Time{}, false
		}
		if at.After(now) {
			return index, at, true
		}
	}
	return 0, time.Time{}, false
}

// occurrenceMessage gives a repeated run its own Message-ID and Date. Run 0 of
// the original series is returned unchanged. The ID is derived from the job and run so a retry of the
// same run reuses it.
func occurrenceMessage(raw []byte, jobID string, epoch, index int, at time.Time) []byte {
	if index == 0 && epoch == 0 {
		return raw
	}
	text := string(raw)
	eol := "\r\n"
	end := strings.Index(text, eol+eol)
	if end < 0 {
		eol = "\n"
		end = strings.Index(text, eol+eol)
	}
	if end < 0 {
		return raw
	}
	head, body := text[:end], text[end:]
	domain := "localhost"
	var kept []string
	skipping := false
	for _, line := range strings.Split(head, eol) {
		if line != "" && (line[0] == ' ' || line[0] == '\t') {
			if !skipping {
				kept = append(kept, line)
			}
			continue
		}
		skipping = false
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "message-id:"):
			if at := strings.LastIndex(line, "@"); at >= 0 {
				domain = strings.Trim(strings.TrimSpace(line[at+1:]), "<> ")
			}
			skipping = true
		case strings.HasPrefix(lower, "date:"):
			skipping = true
		default:
			kept = append(kept, line)
		}
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%d", jobID, epoch, index)))
	header := "Message-ID: <" + hex.EncodeToString(digest[:16]) + "@" + domain + ">" + eol +
		"Date: " + at.Format(time.RFC1123Z)
	return []byte(header + eol + strings.Join(kept, eol) + body)
}
