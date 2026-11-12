package database

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIsBusy(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"modernc busy", errors.New("failed to begin transaction: database is locked (5) (SQLITE_BUSY)"), true},
		{"snapshot", errors.New("SQLITE_BUSY_SNAPSHOT: database is locked"), true},
		{"table locked", errors.New("database table is locked"), true},
		{"plain locked", errors.New("database is locked"), true},
		{"other error", errors.New("constraint failed: UNIQUE"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBusy(tc.err); got != tc.want {
				t.Fatalf("IsBusy(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRetryOnBusySucceedsAfterTransientBusy(t *testing.T) {
	attempts := 0
	start := time.Now()
	err := RetryOnBusy(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("RetryOnBusy: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	// Backoff must actually wait: 1ms + 2ms between the three attempts.
	if elapsed := time.Since(start); elapsed < 3*time.Millisecond {
		t.Fatalf("RetryOnBusy returned after %s, want backoff of at least 3ms", elapsed)
	}
}

func TestRetryOnBusyDoesNotRetryOtherErrors(t *testing.T) {
	attempts := 0
	want := errors.New("UNIQUE constraint failed")
	err := RetryOnBusy(func() error {
		attempts++
		return want
	}, 5, time.Millisecond)
	if !errors.Is(err, want) {
		t.Fatalf("RetryOnBusy = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 for a non-busy error", attempts)
	}
}

func TestRetryOnBusyExhaustsAttempts(t *testing.T) {
	attempts := 0
	err := RetryOnBusy(func() error {
		attempts++
		return errors.New("database is locked (5) (SQLITE_BUSY)")
	}, 4, time.Millisecond)
	if err == nil || !IsBusy(err) {
		t.Fatalf("RetryOnBusy = %v, want the busy error after exhausting attempts", err)
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4", attempts)
	}
	if !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("RetryOnBusy should surface the last busy error, got %v", err)
	}
}
