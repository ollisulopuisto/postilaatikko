package replycontent

import (
	"strings"
	"unicode"

	replyparser "github.com/web-ridge/email-reply-parser"
)

// refineLibrarySegments uses the upstream parser as a boundary proposal. Only
// exact prefixes ending before a dated, addressed reply attribution are trusted.
// Inline quoted runs and non-prefix/reordered output stay with our lossless
// segmenter. The library never rewrites stored content or source offsets.
func refineLibrarySegments(lines []plainLine, segments []plainSegment) []plainSegment {
	var result []plainSegment
	for _, seg := range segments {
		if seg.kind != BlockAuthored || seg.to-seg.from > 2000 {
			result = append(result, seg)
			continue
		}
		text := make([]string, 0, seg.to-seg.from)
		quoted := false
		for _, line := range lines[seg.from:seg.to] {
			text = append(text, line.text)
			quoted = quoted || line.quote
		}
		raw := strings.Join(text, "\n")
		if quoted || !strings.Contains(strings.ToLower(raw), "wrote:") {
			result = append(result, seg)
			continue
		}
		proposed := strings.TrimSpace(replyparser.Parse(raw))
		start := len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
		if proposed == "" || !strings.HasPrefix(raw[start:], proposed) {
			result = append(result, seg)
			continue
		}
		boundary := start + len(proposed)
		remaining := strings.TrimSpace(raw[boundary:])
		first, _, _ := strings.Cut(remaining, "\n")
		dated := strings.ContainsAny(first, "0123456789")
		if remaining == "" || !strings.HasPrefix(strings.ToLower(first), "on ") || !dated || !strings.Contains(first, "@") || !attributionRe.MatchString(first) {
			result = append(result, seg)
			continue
		}
		split := seg.from + strings.Count(raw[:boundary], "\n") + 1
		for split < seg.to && lines[split].blank {
			split++
		}
		if split >= seg.to {
			result = append(result, seg)
			continue
		}
		result = append(result, plainSegment{BlockAuthored, seg.from, split, ""}, plainSegment{BlockQuoted, split, seg.to, ""})
	}
	return result
}
