// Package textrepair provides conservative, reversible display repairs for
// cached text. It never modifies stored messages or original MIME sources.
package textrepair

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// FinnishUTF8 repairs repeated, characteristic UTF-8-as-Windows-1252 Finnish
// vowel sequences only when reversing them produces valid UTF-8. A single
// quoted example is left alone because its intent is ambiguous.
func FinnishUTF8(text string) string {
	count := 0
	for _, marker := range []string{"Ã¤", "Ã¶", "Ã„", "Ã–", "Ã¥", "Ã…"} {
		count += strings.Count(text, marker)
	}
	if count < 2 {
		return text
	}
	raw, err := charmap.Windows1252.NewEncoder().Bytes([]byte(text))
	if err != nil || !utf8.Valid(raw) {
		return text
	}
	fixed := string(raw)
	if fixed == text {
		return text
	}
	return fixed
}
