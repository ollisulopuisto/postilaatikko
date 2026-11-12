package database

import (
	"strings"
	"time"
)

// IsBusy reports whether err is SQLite lock contention (SQLITE_BUSY,
// "database is locked", "database table is locked"). These are transient:
// another connection held the write lock past this connection's busy_timeout,
// so retrying after a short backoff is the correct response. Matching is
// textual because modernc.org/sqlite errors do not wrap a comparable sentinel.
func IsBusy(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToUpper(err.Error())
	return strings.Contains(text, "SQLITE_BUSY") ||
		strings.Contains(text, "DATABASE IS LOCKED") ||
		strings.Contains(text, "DATABASE TABLE IS LOCKED")
}

// RetryOnBusy runs op, retrying only SQLite lock-contention errors with
// exponential backoff (baseDelay, 2x, 4x, ...). Any other error — and a nil
// error — returns immediately. When every attempt is busy, the last busy
// error is returned.
//
// The DSN busy_timeout already absorbs short lock holds; this retry layer is
// for sustained writer churn (sync, FTS batches, cache rebuilds) where a
// single waiter can starve past the timeout. Keep maxAttempts small so a
// genuinely stuck database still fails within a reasonable time.
func RetryOnBusy(op func() error, maxAttempts int, baseDelay time.Duration) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err = op(); err == nil || !IsBusy(err) {
			return err
		}
		if attempt+1 < maxAttempts {
			time.Sleep(baseDelay * time.Duration(1<<uint(attempt)))
		}
	}
	return err
}
