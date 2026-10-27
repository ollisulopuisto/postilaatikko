package gmail

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Budget is shared by all clients for one account. Pace quota UNITS, not request
// counts: current projects charge 20/60 units per metadata/body get and 40 per
// thread get. 70 units/sec leaves headroom below Google's 6000/user/min limit.
// FIFO reservations prevent small background reads starving a requested body.
type Budget struct {
	mu                 sync.Mutex
	next, blockedUntil time.Time
}

func NewBudget() *Budget { return &Budget{} }

func (b *Budget) Paused() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.blockedUntil.After(time.Now())
}
func (b *Budget) reserve(now time.Time, cost int) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	ready := b.next
	if now.After(ready) {
		ready = now
	}
	if b.blockedUntil.After(ready) {
		ready = b.blockedUntil
	}
	b.next = ready.Add(time.Duration(cost) * time.Second / 70)
	return ready
}
func (b *Budget) Cooldown(delay time.Duration) {
	if delay < time.Minute {
		delay = time.Minute
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	until := time.Now().Add(delay)
	if until.After(b.blockedUntil) {
		b.blockedUntil = until
	}
}
func (b *Budget) Wait(ctx context.Context, method, path string, query url.Values) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ready := b.reserve(time.Now(), quotaCost(method, path, query))
	for {
		if err := sleepContext(ctx, max(time.Duration(0), time.Until(ready))); err != nil {
			return err
		}
		b.mu.Lock()
		blocked := b.blockedUntil
		b.mu.Unlock()
		if !blocked.After(time.Now()) {
			return nil
		}
		// Re-reserve after a server cooldown to avoid releasing all pending calls
		// at the same instant when that cooldown expires.
		ready = b.reserve(time.Now(), quotaCost(method, path, query))
	}
}

func quotaCost(method, path string, query url.Values) int {
	if method == http.MethodGet {
		switch {
		case path == "/profile":
			return 1
		case path == "/history":
			return 2
		case strings.HasPrefix(path, "/labels"):
			return 1
		case path == "/messages", path == "/drafts":
			return 5
		case path == "/threads":
			return 10
		case strings.Contains(path, "/attachments/"):
			return 20
		case strings.HasPrefix(path, "/threads/"):
			if query.Get("format") == "full" {
				return 120
			}
			return 40
		case strings.HasPrefix(path, "/messages/"), strings.HasPrefix(path, "/drafts/"):
			if query.Get("format") == "full" || query.Get("format") == "raw" || query.Get("format") == "html" {
				return 60
			}
			return 20
		}
	}
	switch {
	case strings.HasSuffix(path, "/send"):
		return 100
	case strings.HasSuffix(path, "/batchModify"), strings.HasSuffix(path, "/batchDelete"):
		return 50
	case strings.HasSuffix(path, "/insert"), strings.HasSuffix(path, "/import"):
		return 25
	case strings.HasSuffix(path, "/modify"):
		return 10
	case strings.HasPrefix(path, "/labels"):
		return 5
	case strings.HasPrefix(path, "/drafts"):
		return 15
	}
	return 100 // conservative admission for future methods
}
