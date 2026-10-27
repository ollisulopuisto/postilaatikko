package app

import (
	"context"
	"testing"
	"time"
)

func TestAutomaticRefreshDoesNotCancelRunningSync(t *testing.T) {
	cancelled := false
	a := &App{ctx: context.Background(), syncContexts: map[string]context.CancelFunc{"a:f": func() { cancelled = true }}, syncLastRequest: map[string]time.Time{}}
	if err := a.syncFolder("a", "f", false); err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("automatic refresh cancelled a manual sync")
	}
}
