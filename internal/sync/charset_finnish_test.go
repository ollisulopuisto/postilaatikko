package sync

import "testing"

func TestMislabeledFinnishUTF8VersusActualLatin1(t *testing.T) {
	original := "Hyvää päivää, nähdään myöhemmin."
	if got := decodeCharset([]byte(original), "iso-8859-1"); got != original {
		t.Fatalf("mislabeled UTF-8: %q", got)
	}
	latin := []byte{'H', 'y', 'v', 0xe4, 0xe4, ' ', 'p', 0xe4, 'i', 'v', 0xe4, 0xe4}
	if got := decodeCharset(latin, "iso-8859-1"); got != "Hyvää päivää" {
		t.Fatalf("Latin-1: %q", got)
	}
}

func TestExtractCharsetFromHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "no meta tag",
			input:    "<html><head><title>No meta tag</title></head><body><p>Hello world</p></body></html>",
			expected: "",
		},
		{
			name:     "meta without charset",
			input:    `<html><head><meta name="viewport" content="width=device-width"></head><body></body></html>`,
			expected: "",
		},
		{
			name:     "meta charset standard lowercase",
			input:    `<html><head><meta charset="utf-8"></head><body></body></html>`,
			expected: "utf-8",
		},
		{
			name:     "meta charset uppercase",
			input:    `<HTML><HEAD><META CHARSET="ISO-8859-1"></HEAD><BODY></BODY></HTML>`,
			expected: "ISO-8859-1",
		},
		{
			name:     "meta charset unquoted",
			input:    `<html><head><meta charset=gb2312></head><body></body></html>`,
			expected: "gb2312",
		},
		{
			name:     "meta http-equiv content-type standard",
			input:    `<!DOCTYPE html><html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"></head><body></body></html>`,
			expected: "windows-1252",
		},
		{
			name:     "meta http-equiv mixed case with spaces",
			input:    `<html><head><META HTTP-EQUIV="CONTENT-TYPE" CONTENT="text/html;   charset=Shift_JIS"></head><body></body></html>`,
			expected: "Shift_JIS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCharsetFromHTML([]byte(tt.input))
			if got != tt.expected {
				t.Errorf("extractCharsetFromHTML(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}
