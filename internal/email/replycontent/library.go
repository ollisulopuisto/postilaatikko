package replycontent

import (
	"strings"

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
		if quoted || len(raw) > 64<<10 || !strings.Contains(strings.ToLower(raw), "wrote:") {
			result = append(result, seg)
			continue
		}
		// Ask the library about each strong header independently. A signature
		// before it can make the library stop before the header in the full body.
		split := -1
		attempts := 0
		for i := seg.from + 1; i < seg.to; i++ {
			first := strings.TrimSpace(lines[i].text)
			if len(first) > maxMarkerLineLen || !strings.HasPrefix(strings.ToLower(first), "on ") || !strings.ContainsAny(first, "0123456789") || !strings.Contains(first, "@") || !attributionRe.MatchString(first) {
				continue
			}
			attempts++
			if attempts > 8 {
				break
			}
			tail := make([]string, 0, seg.to-i)
			for _, line := range lines[i:seg.to] {
				tail = append(tail, line.text)
			}
			const marker = "Postilaatikko boundary check"
			proposed := strings.TrimSpace(replyparser.Parse(marker + "\n\n" + strings.Join(tail, "\n")))
			if proposed == marker {
				split = i
				break
			}
		}
		if split < 0 {
			result = append(result, seg)
			continue
		}
		result = append(result, plainSegment{BlockAuthored, seg.from, split, ""}, plainSegment{BlockQuoted, split, seg.to, ""})
	}
	return result
}
