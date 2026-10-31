package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/database"
)

type trackedPendingSendActivity struct {
	endCalls int
}

func (a *trackedPendingSendActivity) End() {
	a.endCalls++
}

func newPendingSendActivityTestApp(t *testing.T) *App {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "mail.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return &App{
		ctx:                   context.Background(),
		db:                    db,
		pendingSendActivities: make(map[string]pendingSendActivity),
		runtimeEventEmitter:   func(string, ...interface{}) {},
	}
}

func insertPendingSendActivityRow(t *testing.T, a *App, id string) *trackedPendingSendActivity {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := a.db.Exec(`INSERT INTO pending_sends
		(id, account_id, draft_id, payload, dispatch_at, state, created_at)
		VALUES (?, 'account', '', '{}', ?, 'pending', ?)`, id, now, now)
	if err != nil {
		t.Fatalf("insert pending-send row: %v", err)
	}
	activity := &trackedPendingSendActivity{}
	a.pendingSendActivities[id] = activity
	return activity
}

func assertNoPendingSendActivity(t *testing.T, a *App, id string) {
	t.Helper()
	a.pendingSendActivitiesMu.Lock()
	defer a.pendingSendActivitiesMu.Unlock()
	if _, held := a.pendingSendActivities[id]; held {
		t.Fatalf("pending-send activity %q is still held", id)
	}
}

func TestCancelPendingSendReleasesActivity(t *testing.T) {
	a := newPendingSendActivityTestApp(t)
	cancelledActivity := insertPendingSendActivityRow(t, a, "cancel-me")
	keptActivity := insertPendingSendActivityRow(t, a, "keep-me")

	if err := a.CancelPendingSend("cancel-me"); err != nil {
		t.Fatalf("CancelPendingSend() error = %v", err)
	}
	assertNoPendingSendActivity(t, a, "cancel-me")
	if cancelledActivity.endCalls != 1 {
		t.Fatalf("cancelled activity End() calls = %d, want 1", cancelledActivity.endCalls)
	}
	a.pendingSendActivitiesMu.Lock()
	_, stillHeld := a.pendingSendActivities["keep-me"]
	a.pendingSendActivitiesMu.Unlock()
	if !stillHeld {
		t.Fatal("cancelling one pending send released another row's activity")
	}
	if keptActivity.endCalls != 0 {
		t.Fatalf("unrelated activity End() calls = %d, want 0", keptActivity.endCalls)
	}
	if err := a.CancelPendingSend("cancel-me"); err == nil {
		t.Fatal("second CancelPendingSend() unexpectedly succeeded")
	}
	if cancelledActivity.endCalls != 1 {
		t.Fatalf("repeated cancellation called End() %d times, want 1", cancelledActivity.endCalls)
	}
}

func TestCompletePendingSendReleasesActivity(t *testing.T) {
	a := newPendingSendActivityTestApp(t)
	activity := insertPendingSendActivityRow(t, a, "complete-me")

	if err := a.CompletePendingSend("complete-me"); err != nil {
		t.Fatalf("CompletePendingSend() error = %v", err)
	}
	assertNoPendingSendActivity(t, a, "complete-me")
	if activity.endCalls != 1 {
		t.Fatalf("completed activity End() calls = %d, want 1", activity.endCalls)
	}
}

func TestExpirePendingSendsReleasesActivities(t *testing.T) {
	a := newPendingSendActivityTestApp(t)
	activity := insertPendingSendActivityRow(t, a, "expire-me")

	if err := a.ExpirePendingSends(); err != nil {
		t.Fatalf("ExpirePendingSends() error = %v", err)
	}
	assertNoPendingSendActivity(t, a, "expire-me")
	if activity.endCalls != 1 {
		t.Fatalf("expired activity End() calls = %d, want 1", activity.endCalls)
	}
}
