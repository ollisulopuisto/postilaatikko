package gmail

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestQuotaCostsIncludeFormatMultipliers(t *testing.T) {
	for _, test := range []struct {
		method, path, format string
		want                 int
	}{
		{"GET", "/messages/id", "metadata", 20}, {"GET", "/messages/id", "full", 60}, {"GET", "/messages/id", "raw", 60},
		{"GET", "/threads/id", "metadata", 40}, {"GET", "/threads/id", "full", 120}, {"GET", "/messages", "", 5}, {"GET", "/threads", "", 10}, {"POST", "/messages/send", "", 100},
	} {
		if got := quotaCost(test.method, test.path, url.Values{"format": {test.format}}); got != test.want {
			t.Fatalf("%+v cost %d", test, got)
		}
	}
}

func TestBudgetSharesReservationsAndCooldown(t *testing.T) {
	b := NewBudget()
	now := time.Now()
	one := b.reserve(now, 70)
	two := b.reserve(now, 70)
	if !one.Equal(now) || two.Sub(one) != time.Second {
		t.Fatalf("reservation %s %s", one, two)
	}
	b.Cooldown(time.Minute)
	if next := b.reserve(time.Now(), 20); next.Before(now.Add(59 * time.Second)) {
		t.Fatalf("cooldown bypassed %s", next)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Wait(ctx, http.MethodGet, "/messages/id", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
}

func TestRateLimitPausesOtherClientsWithoutImmediateRetry(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"rateLimitExceeded"}],"message":"quota"}}`))
	})
	budget := NewBudget()
	c.Budget = budget
	if _, err := c.Search(context.Background(), SearchRequest{}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("quota error %v", err)
	}
	if calls != 1 {
		t.Fatalf("immediate quota retries: %d", calls)
	}
	other := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("shared cooldown bypassed") })
	other.Budget = budget
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := other.Search(ctx, SearchRequest{}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("cooldown cancellation %v", err)
	}
}

func TestMetadataBatchStopsOnQuotaAndPreservesCause(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"rateLimitExceeded"}]}}`))
	})
	c.Budget = NewBudget()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	start := time.Now()
	_, err := c.Metadata(ctx, ids)
	if !errors.Is(err, ErrRateLimited) || calls.Load() != 1 || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("batch did not stop on quota: %v requests %d elapsed %s", err, calls.Load(), time.Since(start))
	}
}
