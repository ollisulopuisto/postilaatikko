package sync

import "testing"

// The allocation-free pre-check must not reject anything the regexes accept.
func TestExtractCharsetFromHTMLPrecheckMatchesRegexFolding(t *testing.T) {
	cases := map[string]string{
		"<meta charset=Shift_JIS>": "Shift_JIS",
		"<META CHARSET=UTF-8>":     "UTF-8",
		"<meta charſet=Shift_JIS>": "Shift_JIS",
		"<meta http-equiv=\"Content-Type\" content=\"text/html; charſet=ISO-8859-1\">": "ISO-8859-1",
		"<meta name=x>":        "",
		"<p>charset=UTF-8</p>": "",
	}
	for in, want := range cases {
		if got := extractCharsetFromHTML([]byte(in)); got != want {
			t.Errorf("extractCharsetFromHTML(%q) = %q, want %q", in, got, want)
		}
	}
}
