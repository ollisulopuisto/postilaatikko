package replycontent

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	// maxPlainBytes bounds the work spent on one body. A larger body is returned
	// whole, unanalyzed.
	maxPlainBytes = 1 << 20
	// maxSignatureLines is the longest run after a "-- " delimiter still read as
	// a signature. A longer one is real content and stays visible.
	maxSignatureLines = 15
	// maxMarkerLineLen is the longest line tested against the marker patterns.
	maxMarkerLineLen = 300
	// headerRunLookahead is how far past a "From:" line the header block is searched.
	headerRunLookahead = 8
)

var (
	attributionRe = regexp.MustCompile(`(?i)\b(wrote|kirjoitti|schrieb|skrev|a écrit)\b[^\n]*:\s*$`)
	originalRe    = regexp.MustCompile(`(?i)^-{2,}\s*(original message|alkuperäinen viesti|ursprüngliche nachricht|ursprungligt meddelande)\s*-{2,}$`)
	forwardedRe   = regexp.MustCompile(`(?i)^(-{2,}\s*(forwarded message|edelleenlähetetty viesti|weitergeleitete nachricht)\s*-{2,}|begin forwarded message:?)$`)
	separatorRe   = regexp.MustCompile(`^_{5,}$`)
	mobileRe      = regexp.MustCompile(`(?i)^(sent from my (iphone|ipad|android|galaxy|blackberry|phone|mobile|samsung)\b.{0,40}|sent from (yahoo mail|mail for windows|outlook for (ios|android))\b.{0,40}|get outlook for (ios|android)\b.{0,20}|lähetetty (iphonesta|ipadista|androidista|windows|outlook)\b.{0,30})$`)

	headerStartLabels = map[string]bool{"from": true, "lähettäjä": true, "von": true, "från": true}
	headerLabels      = map[string]bool{
		"from": true, "sent": true, "date": true, "to": true, "cc": true, "subject": true,
		"lähettäjä": true, "lähetetty": true, "päivämäärä": true, "vastaanottaja": true, "kopio": true, "aihe": true,
		"von": true, "gesendet": true, "datum": true, "an": true, "betreff": true,
		"från": true, "skickat": true, "till": true, "ämne": true,
	}
)

type plainLine struct {
	start, end int    // byte range in the body, end before the line break
	text       string // the line without a trailing carriage return
	blank      bool
	quote      bool
}

type plainSegment struct {
	kind     BlockKind
	from, to int // line indexes, to exclusive
	warning  string
}

// ExtractPlain splits a plain-text body into authored text, quoted history,
// signatures and text it cannot classify. It is conservative: anything it is
// not sure about stays visible as an unknown block, a body with no reliable
// authored part comes back as one unknown block, and the stored body is only
// ever sliced, never rewritten.
func ExtractPlain(body string) Result {
	result := Result{Version: ExtractorVersion}
	if strings.TrimSpace(body) == "" {
		result.Status = StatusUnavailable
		return result
	}
	if len(body) > maxPlainBytes {
		return plainFallback(body, WarnInputTooLarge)
	}

	lines := splitPlainLines(body)
	segments := refineLibrarySegments(lines, segmentPlain(lines))

	hasAuthored, hasQuoted, hasForwarded := false, false, false
	for _, seg := range segments {
		switch seg.kind {
		case BlockAuthored:
			hasAuthored = true
		case BlockQuoted:
			hasQuoted = true
		}
		if seg.warning == WarnForwardedContent {
			hasForwarded = true
		}
	}
	if !hasAuthored {
		switch {
		case hasForwarded:
			return plainFallback(body, WarnForwardedContent)
		case hasQuoted:
			return plainFallback(body, WarnQuoteOnly)
		}
		return plainFallback(body)
	}

	status := StatusExtracted
	for _, seg := range segments {
		start, end := segmentBytes(lines, seg, body)
		if seg.kind == BlockUnknown {
			status = StatusPartial
		}
		result.Blocks = append(result.Blocks, Block{
			ID:      blockID(len(result.Blocks) + 1),
			Kind:    seg.kind,
			Format:  PartText,
			Content: body[start:end],
			Source:  &Source{Part: PartText, Start: start, End: end},
		})
		result.Warnings = appendWarning(result.Warnings, seg.warning)
	}
	result.Status = status
	return result
}

func plainFallback(body string, warnings ...string) Result {
	start := len(body) - len(strings.TrimLeftFunc(body, unicode.IsSpace))
	end := len(strings.TrimRightFunc(body, unicode.IsSpace))
	result := Result{
		Version: ExtractorVersion,
		Status:  StatusFallback,
		Blocks: []Block{{
			ID:      blockID(1),
			Kind:    BlockUnknown,
			Format:  PartText,
			Content: body[start:end],
			Source:  &Source{Part: PartText, Start: start, End: end},
		}},
	}
	for _, w := range warnings {
		result.Warnings = appendWarning(result.Warnings, w)
	}
	return result
}

func blockID(n int) string {
	return "b" + strconv.Itoa(n)
}

func appendWarning(list []string, w string) []string {
	if w == "" {
		return list
	}
	for _, have := range list {
		if have == w {
			return list
		}
	}
	return append(list, w)
}

func splitPlainLines(body string) []plainLine {
	lines := make([]plainLine, 0, strings.Count(body, "\n")+1)
	for start := 0; start <= len(body); {
		end := strings.IndexByte(body[start:], '\n')
		next := len(body) + 1
		if end < 0 {
			end = len(body)
		} else {
			end += start
			next = end + 1
		}
		text := strings.TrimSuffix(body[start:end], "\r")
		trimmed := strings.TrimSpace(text)
		lines = append(lines, plainLine{
			start: start,
			end:   end,
			text:  text,
			blank: trimmed == "",
			quote: strings.HasPrefix(strings.TrimLeft(text, " \t"), ">"),
		})
		start = next
	}
	return lines
}

// segmentBytes returns the byte range of a segment: from its first line to the
// end of its last non-blank line, without trailing white space.
func segmentBytes(lines []plainLine, seg plainSegment, body string) (int, int) {
	first, last := seg.from, seg.to-1
	for first <= last && lines[first].blank {
		first++
	}
	for last >= first && lines[last].blank {
		last--
	}
	start, end := lines[first].start, lines[last].end
	for end > start && strings.ContainsRune(" \t\r", rune(body[end-1])) {
		end--
	}
	return start, end
}

func segmentPlain(lines []plainLine) []plainSegment {
	var segments []plainSegment
	haveContent, sawAttribution := false, false
	n := len(lines)

	for i := 0; i < n; {
		if lines[i].blank {
			i++
			continue
		}

		if forwardedAt(lines, i) {
			segments = append(segments, plainSegment{BlockUnknown, i, n, WarnForwardedContent})
			return segments
		}
		if quotedTailAt(lines, i) {
			segments = append(segments, plainSegment{BlockQuoted, i, n, ""})
			return segments
		}
		if count := attributionAt(lines, i); count > 0 {
			end := consumeQuoteRun(lines, i+count)
			segments = append(segments, plainSegment{BlockQuoted, i, end, ""})
			sawAttribution, haveContent = true, true
			i = end
			continue
		}
		if lines[i].quote {
			end := consumeQuoteRun(lines, i)
			seg := plainSegment{BlockQuoted, i, end, ""}
			if !sawAttribution && !restIsBlank(lines, end) {
				seg.kind, seg.warning = BlockUnknown, WarnAmbiguousQuoteMarker
			}
			segments = append(segments, seg)
			haveContent = true
			i = end
			continue
		}
		if haveContent && signatureDelimiterAt(lines, i) {
			end := i + 1
			for end < n && !historyBoundaryAt(lines, end) {
				end++
			}
			kind := BlockSignature
			if nonBlankCount(lines, i, end) > maxSignatureLines {
				kind = BlockUnknown
			}
			segments = append(segments, plainSegment{kind, i, end, ""})
			i = end
			continue
		}

		end := i + 1
		for end < n && !historyBoundaryAt(lines, end) && !(signatureDelimiterAt(lines, end)) {
			end++
		}
		segments = append(segments, splitMobileFooter(lines, i, end)...)
		haveContent = true
		i = end
	}
	return segments
}

// historyBoundaryAt reports whether line i starts quoted or forwarded material.
func historyBoundaryAt(lines []plainLine, i int) bool {
	return lines[i].quote || forwardedAt(lines, i) || quotedTailAt(lines, i) || attributionAt(lines, i) > 0
}

// splitMobileFooter splits a trailing "Sent from my iPhone" style footer off an
// authored run, but only when real text comes before it.
func splitMobileFooter(lines []plainLine, from, to int) []plainSegment {
	last := to - 1
	for last > from && lines[last].blank {
		last--
	}
	if last > from && len(lines[last].text) <= maxMarkerLineLen && mobileRe.MatchString(strings.TrimSpace(lines[last].text)) {
		before := last - 1
		for before >= from && lines[before].blank {
			before--
		}
		if before >= from {
			return []plainSegment{{BlockAuthored, from, last, ""}, {BlockSignature, last, to, ""}}
		}
	}
	return []plainSegment{{BlockAuthored, from, to, ""}}
}

// consumeQuoteRun returns the line after the quote lines that start at from,
// skipping blank lines only when another quote line follows them.
func consumeQuoteRun(lines []plainLine, from int) int {
	end := from
	for end < len(lines) {
		switch {
		case lines[end].quote:
			end++
		case lines[end].blank:
			next := end
			for next < len(lines) && lines[next].blank {
				next++
			}
			if next < len(lines) && lines[next].quote {
				end = next
			} else {
				return end
			}
		default:
			return end
		}
	}
	return end
}

func restIsBlank(lines []plainLine, from int) bool {
	for _, l := range lines[from:] {
		if !l.blank {
			return false
		}
	}
	return true
}

func nonBlankCount(lines []plainLine, from, to int) int {
	count := 0
	for _, l := range lines[from:to] {
		if !l.blank {
			count++
		}
	}
	return count
}

func signatureDelimiterAt(lines []plainLine, i int) bool {
	marker := strings.TrimSpace(lines[i].text)
	if marker == "--" {
		return true
	}
	// Apple Mail commonly uses a standalone em dash. Require a separated,
	// short footer containing contact information, not merely a prose divider.
	if marker != "—" || i == 0 || !lines[i-1].blank {
		return false
	}
	end := i + 1
	contact := false
	for end < len(lines) && end-i <= maxSignatureLines {
		if historyBoundaryAt(lines, end) {
			break
		}
		contact = contact || strings.Contains(lines[end].text, "@")
		end++
	}
	return contact && end-i <= maxSignatureLines
}

func forwardedAt(lines []plainLine, i int) bool {
	text := strings.TrimSpace(lines[i].text)
	return len(text) <= maxMarkerLineLen && forwardedRe.MatchString(text)
}

// nextNonBlank returns the first non-blank line at or after i, or len(lines).
func nextNonBlank(lines []plainLine, i int) int {
	for i < len(lines) && lines[i].blank {
		i++
	}
	return i
}

// attributionAt returns how many lines make up a "On ... wrote:" header that
// starts at i, or 0. The header only counts when a quote follows it, so prose
// that happens to end in a colon stays authored.
func attributionAt(lines []plainLine, i int) int {
	count := 0
	text := strings.TrimSpace(lines[i].text)
	switch {
	case len(text) <= maxMarkerLineLen && attributionRe.MatchString(text):
		count = 1
	case i+1 < len(lines) && len(text) <= maxMarkerLineLen && len(lines[i+1].text) <= maxMarkerLineLen &&
		len(text) >= 3 && strings.EqualFold(text[:3], "on ") &&
		attributionRe.MatchString(strings.TrimSpace(lines[i+1].text)):
		count = 2
	default:
		return 0
	}
	next := nextNonBlank(lines, i+count)
	if next < len(lines) && lines[next].quote {
		return count
	}
	return 0
}

// quotedTailAt reports whether line i starts a client-style quoted block that
// runs to the end of the message: an "Original Message" marker or a header run
// such as From/Sent/To/Subject.
func quotedTailAt(lines []plainLine, i int) bool {
	text := strings.TrimSpace(lines[i].text)
	if len(text) > maxMarkerLineLen {
		return false
	}
	if originalRe.MatchString(text) {
		return true
	}
	if separatorRe.MatchString(text) {
		next := nextNonBlank(lines, i+1)
		return next < len(lines) && headerRunAt(lines, next)
	}
	return headerRunAt(lines, i)
}

func headerLabel(text string) string {
	colon := strings.IndexByte(text, ':')
	if colon <= 0 || colon > 20 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(text[:colon]))
}

// headerRunAt reports whether a block of at least three consecutive header
// lines (From, Sent, To, Subject and their Finnish, German and Swedish
// equivalents) starts at i.
func headerRunAt(lines []plainLine, i int) bool {
	if !headerStartLabels[headerLabel(lines[i].text)] {
		return false
	}
	found := 1
	for j := i + 1; j < len(lines) && j <= i+headerRunLookahead; j++ {
		if lines[j].blank {
			break
		}
		if !headerLabels[headerLabel(lines[j].text)] {
			break
		}
		found++
	}
	return found >= 3
}
