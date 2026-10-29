package backend

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DateTimeMention is one recognized date/time mention in message text, ready
// to hand off to Calendar_CreateICSFromDetails for review/import.
//
// EndUnix is always populated: an explicit end time from the text when
// stated, else the next-day midnight for all-day mentions, else start plus
// one hour (the ICS handoff requires an end; the user adjusts on import).
// IsAllDay marks date-only mentions.
type DateTimeMention struct {
	StartUnix int64  `json:"startUnix"`
	EndUnix   int64  `json:"endUnix"`
	IsAllDay  bool   `json:"isAllDay"`
	RawText   string `json:"rawText"`
}

// Conservative-detection limits.
const (
	// maxMentionsPerText caps the list size for UI sanity.
	maxMentionsPerText = 8
	// maxTimeLookahead bounds how far after a date match a trailing time
	// spec may appear (covers "at 2:30 PM", "14:00-15:00").
	maxTimeLookahead = 40
	// defaultDuration is applied when the text states no end time.
	defaultDuration = time.Hour
)

var monthNames = map[string]time.Month{
	"january": time.January, "jan": time.January,
	"february": time.February, "feb": time.February,
	"march": time.March, "mar": time.March,
	"april": time.April, "apr": time.April,
	"may":  time.May,
	"june": time.June, "jun": time.June,
	"july": time.July, "jul": time.July,
	"august": time.August, "aug": time.August,
	"september": time.September, "sept": time.September, "sep": time.September,
	"october": time.October, "oct": time.October,
	"november": time.November, "nov": time.November,
	"december": time.December, "dec": time.December,
}

var monthPattern = `january|february|march|april|may|june|july|august|september|october|november|december|sept|jan|feb|mar|apr|jun|jul|aug|sep|oct|nov|dec`

var (
	// "June 5, 2026", "Jan 12 2027" (optional ordinal suffixes).
	reMonthDayYear = regexp.MustCompile(`(?i)\b(` + monthPattern + `)\.?\s+([0-9]{1,2})(?:st|nd|rd|th)?,?\s+([0-9]{4})\b`)
	// "5 June 2026".
	reDayMonthYear = regexp.MustCompile(`(?i)\b([0-9]{1,2})(?:st|nd|rd|th)?\s+(` + monthPattern + `)\.?,?\s+([0-9]{4})\b`)
	// ISO "2026-06-05" with an optional " 14:00" or "T14:00:00+02:00" tail.
	reISODate = regexp.MustCompile(`\b([0-9]{4})-([0-9]{2})-([0-9]{2})(?:[T ]([0-9]{1,2}):([0-9]{2})(?::([0-9]{2}))?(Z|[+-][0-9]{2}:?[0-9]{2})?)?\b`)
)

// timeSpecRe parses a time (and optional end/range + fixed zone designator)
// at the start of the remaining text after a date match.
var timeSpecRe = regexp.MustCompile(`(?i)^\s*(?:,\s*|\bat\s+)?([0-9]{1,2})(?::([0-9]{2}))?(?::([0-9]{2}))?(?:\s*([ap])\.?(?:\s?m\.?)?)?(?:\s*(?:-|–|to\s+)([0-9]{1,2})(?::([0-9]{2}))?(?:\s*([ap])\.?(?:\s?m\.?)?)?)?(?:\s*(Z|UTC|GMT|[+-][0-9]{2}:?[0-9]{2}))?\b`)

// namedZoneRe matches ambiguous civil zone abbreviations that invalidate a
// mention when they directly follow the recognized time: PST maps to
// different regions and does not encode a reliable fixed offset.
var namedZoneRe = regexp.MustCompile(`(?i)^\s*(PST|PDT|EST|EDT|CST|CDT|MST|MDT|AKST|AKDT|HST|CET|CEST|EET|EEST|BST|IST|JST|AEST|AEDT|NZST|NZDT)\b`)

// dateHit is one raw date match awaiting time resolution.
type dateHit struct {
	year, month, day int
	matchEnd         int
	rawLen           int
	// Inline ISO time (present when reISODate captured hour groups).
	inlineHour, inlineMinute, inlineSecond int
	hasInlineTime                          bool
	inlineZone                             string
}

// atoi converts a digit string to an int, returning 0 on malformed input;
// callers validate the parsed values against range checks.
func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// DetectDateTimes scans text for absolute date/time mentions and returns
// conservative candidates in document order, deduplicated by instant.
// See datetimes_test.go for the confidence rules this must satisfy.
func DetectDateTimes(text string) []DateTimeMention {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	var hits []dateHit
	subexpOf := func(loc []int) []string {
		subexp := make([]string, 0, len(loc)/2)
		for i := 0; i < len(loc); i += 2 {
			if loc[i] < 0 {
				subexp = append(subexp, "")
				continue
			}
			subexp = append(subexp, text[loc[i]:loc[i+1]])
		}
		return subexp
	}

	for _, loc := range reMonthDayYear.FindAllStringSubmatchIndex(text, -1) {
		s := subexpOf(loc)
		hits = append(hits, dateHit{year: atoi(s[3]), month: int(monthNames[strings.ToLower(s[1])]), day: atoi(s[2]), matchEnd: loc[1], rawLen: loc[1] - loc[0]})
	}
	for _, loc := range reDayMonthYear.FindAllStringSubmatchIndex(text, -1) {
		s := subexpOf(loc)
		hits = append(hits, dateHit{year: atoi(s[3]), month: int(monthNames[strings.ToLower(s[2])]), day: atoi(s[1]), matchEnd: loc[1], rawLen: loc[1] - loc[0]})
	}
	for _, loc := range reISODate.FindAllStringSubmatchIndex(text, -1) {
		s := subexpOf(loc)
		hit := dateHit{year: atoi(s[1]), month: atoi(s[2]), day: atoi(s[3]), matchEnd: loc[1], rawLen: loc[1] - loc[0]}
		if s[4] != "" {
			hit.hasInlineTime = true
			hit.inlineHour = atoi(s[4])
			hit.inlineMinute = atoi(s[5])
			hit.inlineSecond = atoi(s[6])
			hit.inlineZone = s[7]
		}
		hits = append(hits, hit)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].matchEnd < hits[j].matchEnd })

	var mentions []DateTimeMention
	seen := make(map[[2]int64]bool)
	for _, hit := range hits {
		if len(mentions) >= maxMentionsPerText {
			break
		}
		dateRaw := text[hit.matchEnd-hit.rawLen : hit.matchEnd]
		mention, ok := buildMention(hit, dateRaw, text[hit.matchEnd:])
		if !ok {
			continue
		}
		key := [2]int64{mention.StartUnix, mention.EndUnix}
		if seen[key] {
			continue
		}
		seen[key] = true
		mentions = append(mentions, mention)
	}
	return mentions
}

// buildMention resolves one date hit plus its optional trailing time spec.
// ok=false means the mention failed a confidence or validity check.
func buildMention(hit dateHit, dateRaw, rest string) (DateTimeMention, bool) {
	if !validDate(hit.year, time.Month(hit.month), hit.day) {
		return DateTimeMention{}, false
	}
	loc := configuredTZ()

	if hit.hasInlineTime {
		hour, minute, second, ok := resolveClock(hit.inlineHour, hit.inlineMinute, hit.inlineSecond, "")
		if !ok {
			return DateTimeMention{}, false
		}
		zone := hit.inlineZone
		if zone != "" && isAmbiguousZone(zone) {
			return DateTimeMention{}, false
		}
		start := anchorInstant(hit.year, time.Month(hit.month), hit.day, hour, minute, second, zone, loc)
		return DateTimeMention{
			StartUnix: start.Unix(),
			EndUnix:   start.Add(defaultDuration).Unix(),
			RawText:   dateRaw,
		}, true
	}

	spec := timeSpecRe.FindStringSubmatch(rest[:min(maxTimeLookahead, len(rest))])
	if spec == nil {
		// Date only → all-day, anchored at the day's midnight in the
		// display timezone so setDateValue renders the same calendar date.
		start := time.Date(hit.year, time.Month(hit.month), hit.day, 0, 0, 0, 0, loc)
		end := start.AddDate(0, 0, 1)
		return DateTimeMention{StartUnix: start.Unix(), EndUnix: end.Unix(), IsAllDay: true, RawText: dateRaw}, true
	}
	if spec[8] != "" && isAmbiguousZone(spec[8]) {
		return DateTimeMention{}, false
	}
	timeText := timeSpecRe.FindString(rest[:min(maxTimeLookahead, len(rest))])
	if namedZoneFollows(rest[len(timeText):]) {
		return DateTimeMention{}, false
	}

	startHour := atoi(spec[1])
	if spec[4] == "" && spec[7] != "" && startHour < 12 && strings.EqualFold(spec[7], "p") {
		// "2-4 pm": the range's meridiem covers the whole range.
		startHour += 12
	}
	hour, minute, second, ok := resolveClock(startHour, atoi(spec[2]), atoi(spec[3]), spec[4])
	if !ok {
		return DateTimeMention{}, false
	}
	start := anchorInstant(hit.year, time.Month(hit.month), hit.day, hour, minute, second, spec[8], loc)

	endUnix := start.Add(defaultDuration).Unix()
	if spec[5] != "" {
		eHour := atoi(spec[5])
		if spec[7] == "" && spec[4] != "" {
			// Range without its own meridiem inherits the start's.
			if eHour < 12 && strings.EqualFold(spec[4], "p") {
				eHour += 12
			}
		}
		eMinute := atoi(spec[6])
		eHour, eMinute, _, eOk := resolveClock(eHour, eMinute, 0, spec[7])
		if !eOk {
			return DateTimeMention{}, false
		}
		end := anchorInstant(hit.year, time.Month(hit.month), hit.day, eHour, eMinute, 0, spec[8], loc)
		if !end.After(start) {
			return DateTimeMention{}, false
		}
		endUnix = end.Unix()
	}

	raw := dateRaw + timeText
	return DateTimeMention{
		StartUnix: start.Unix(),
		EndUnix:   endUnix,
		RawText:   raw,
	}, true
}

// resolveClock converts 24-hour clock values plus an optional am/pm marker
// (already applied when meridiem is empty) into validated clock values.
// Returns ok=false on out-of-range values.
func resolveClock(hour, minute, second int, meridiem string) (int, int, int, bool) {
	switch strings.ToLower(meridiem) {
	case "a":
		if hour == 12 {
			hour = 0
		}
	case "p":
		if hour != 12 {
			hour += 12
		}
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
		return 0, 0, 0, false
	}
	return hour, minute, second, true
}

// anchorInstant resolves the wall-clock values into a Unix instant using the
// stated zone designator when present, else the display timezone. Mentions
// carry no TZName: instants serialize as UTC and stay correct everywhere.
func anchorInstant(year int, month time.Month, day, hour, minute, second int, zone string, loc *time.Location) time.Time {
	if zone == "" {
		return time.Date(year, month, day, hour, minute, second, 0, loc)
	}
	if zone == "Z" || strings.EqualFold(zone, "UTC") || strings.EqualFold(zone, "GMT") {
		return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
	}
	return time.Date(year, month, day, hour, minute, second, 0, time.FixedZone(zone, parseOffsetSeconds(zone)))
}

// parseOffsetSeconds converts "+HH:MM" / "+HHMM" / "-HHMM" into seconds.
func parseOffsetSeconds(zone string) int {
	sign := 1
	body := strings.TrimPrefix(zone, "+")
	if strings.HasPrefix(zone, "-") {
		sign = -1
		body = strings.TrimPrefix(zone, "-")
	}
	body = strings.ReplaceAll(body, ":", "")
	h, _ := strconv.Atoi(body[:2])
	m, _ := strconv.Atoi(body[2:])
	return sign * (h*3600 + m*60)
}

// isAmbiguousZone reports whether a zone designator is too ambiguous to
// anchor an event to. Named civil zones (PST, CST, ...) map to different
// regions; only fixed-offset forms are accepted.
func isAmbiguousZone(zone string) bool {
	switch strings.ToUpper(zone) {
	case "Z", "UTC", "GMT":
		return false
	}
	if (strings.HasPrefix(zone, "+") || strings.HasPrefix(zone, "-")) && len(strings.ReplaceAll(zone[1:], ":", "")) == 4 {
		return false
	}
	return true
}

// namedZoneFollows reports whether an ambiguous civil zone abbreviation
// directly follows the time match (e.g. "2 PM PST") — such mentions are
// invalidated rather than anchored to the reader's zone.
func namedZoneFollows(afterText string) bool {
	return namedZoneRe.MatchString(afterText)
}

func validDate(year int, month time.Month, day int) bool {
	if month < time.January || month > time.December || year < 1 {
		return false
	}
	return day >= 1 && day <= daysIn(year, month)
}

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
