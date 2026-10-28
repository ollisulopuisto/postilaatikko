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
