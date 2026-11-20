package credentials

import (
	"context"
	"sync"
)

// Refresh coordination is separate from Mail snapshot/CAS locking so consent
// can replace a grant during a network request. The losing refresh must still
// fail its CAS and must never overwrite that replacement.
var mailOAuthRefreshes sync.Map

// LockOAuthRefresh serializes ordinary Mail refresh operations for an account
// across credential Stores in this process. Callers must reload the tokens
// after acquiring it and release it after persistence. Waiting is cancellable.
func LockOAuthRefresh(ctx context.Context, accountID string) (func(), error) {
	lock, _ := mailOAuthRefreshes.LoadOrStore(accountID, make(chan struct{}, 1))
	gate := lock.(chan struct{})
	select {
	case gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate
			return nil, err
		}
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
