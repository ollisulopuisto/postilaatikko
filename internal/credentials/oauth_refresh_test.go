package credentials

import (
	"context"
	"errors"
	"testing"
)

func TestMailRefreshLockIsAccountScopedAndCancellable(t *testing.T) {
	release, err := LockOAuthRefresh(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	otherRelease, err := LockOAuthRefresh(context.Background(), "other")
	if err != nil {
		t.Fatal(err)
	}
	otherRelease()
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan error, 1)
	go func() {
		unlock, err := LockOAuthRefresh(ctx, "work")
		if unlock != nil {
			unlock()
		}
		waiting <- err
	}()
	cancel()
	if err := <-waiting; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting operation did not cancel: %v", err)
	}
	release()
	next, err := LockOAuthRefresh(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	next()
}
