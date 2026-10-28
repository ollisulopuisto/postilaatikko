package textrepair

import "testing"

func TestFinnishMojibakeRepairIsConservative(t *testing.T) {
	for _, test := range []struct{ in, want string }{
		{"HyvÃ¤ pÃ¤ivÃ¤, nÃ¤hdÃ¤Ã¤n myÃ¶hemmin!", "Hyvä päivä, nähdään myöhemmin!"},
		{"Hyvä päivä, nähdään myöhemmin!", "Hyvä päivä, nähdään myöhemmin!"},
		{"A single example Ã¤ is quoted here.", "A single example Ã¤ is quoted here."},
		{"Français déjà vu", "Français déjà vu"},
	} {
		if got := FinnishUTF8(test.in); got != test.want {
			t.Fatalf("%q => %q want %q", test.in, got, test.want)
		}
	}
}
