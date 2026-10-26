package database

import (
	"path/filepath"
	"testing"
	"time"
)

// TestUnixEpochSQL pins that every date format the app has stored reads back as
// the same instant. The Go driver writes a time.Time as its String() form,
// which SQLite's unixepoch() alone returns NULL for.
func TestUnixEpochSQL(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "time.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	want := time.Date(2026, 10, 26, 7, 0, 0, 0, time.UTC).Unix()
	eest := time.FixedZone("EEST", 3*3600)
	west := time.FixedZone("", -5*3600-30*60) // an unnamed -05:30 zone

	tests := []struct {
		name  string
		value any
		want  *int64
	}{
		{"Go time in UTC", time.Date(2026, 10, 26, 7, 0, 0, 0, time.UTC), &want},
		{"Go time with a named positive offset", time.Date(2026, 10, 26, 10, 0, 0, 0, eest), &want},
		{"Go time with fractional seconds", time.Date(2026, 10, 26, 10, 0, 0, 123456789, eest), &want},
		{"Go time with an unnamed negative offset", time.Date(2026, 10, 26, 1, 30, 0, 0, west), &want},
		{"RFC3339 with Z", "2026-10-26T07:00:00Z", &want},
		{"RFC3339 with offset", "2026-10-26T10:00:00+03:00", &want},
		{"SQLite datetime", "2026-10-26 07:00:00", &want},
		{"empty string", "", nil},
		{"garbage", "not a date", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *int64
			err := db.QueryRow(`SELECT `+UnixEpochSQL("d")+` FROM (SELECT ? AS d)`, tt.value).Scan(&got)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("got %d, want NULL", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("got NULL, want %d", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("got %d, want %d", *got, *tt.want)
			}
		})
	}
}
