package mailboxevents

import (
	"testing"
	"time"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
)

func TestPublishAssignsMonotonicSequence(t *testing.T) {
	b := NewBroker(Options{})
	for i := uint64(1); i <= 6; i++ {
		// Distinct key each time so coalescing does not collapse them.
		if err := b.Publish(v1.EventMailboxChanged, "acc1", string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.Sequence(); got != 6 {
		t.Fatalf("sequence = %d, want 6", got)
	}
	replay, ch, resync, cancel, err := b.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if resync {
		t.Fatal("resync on fresh subscribe")
	}
	_ = ch
	// afterSeq 0 means no replay wanted; replay only serves reconnects.
	replay2, _, _, cancel2, err := b.Subscribe(3)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel2()
	if len(replay) != 0 {
		t.Fatalf("replay for afterSeq 0 = %d events, want 0", len(replay))
	}
	if len(replay2) != 3 {
		t.Fatalf("replay for afterSeq 3 = %d events, want 3", len(replay2))
	}
	if replay2[0].Sequence != 4 || replay2[2].Sequence != 6 {
		t.Fatalf("replay sequences = %d..%d, want 4..6", replay2[0].Sequence, replay2[2].Sequence)
	}
}

func TestPublishCoalescesBursts(t *testing.T) {
	now := time.Now()
	clock := &now
	b := NewBroker(Options{CoalesceWindow: time.Second}.TestClock(func() time.Time { return *clock }))
	for i := 0; i < 500; i++ {
		if err := b.Publish(v1.EventMailboxChanged, "acc1", "inbox"); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.Sequence(); got != 1 {
		t.Fatalf("500-change burst published %d events, want 1", got)
	}
	// A different folder in the same window still publishes.
	if err := b.Publish(v1.EventMailboxChanged, "acc1", "archive"); err != nil {
		t.Fatal(err)
	}
	if got := b.Sequence(); got != 2 {
		t.Fatalf("distinct key coalesced away; sequence = %d, want 2", got)
	}
	// Past the window the same key publishes again.
	*clock = now.Add(2 * time.Second)
	if err := b.Publish(v1.EventMailboxChanged, "acc1", "inbox"); err != nil {
		t.Fatal(err)
	}
	if got := b.Sequence(); got != 3 {
		t.Fatalf("post-window publish missing; sequence = %d, want 3", got)
	}
}

func TestSubscribeReplaysAndDetectsGap(t *testing.T) {
	b := NewBroker(Options{RingCapacity: 4, CoalesceWindow: time.Nanosecond})
	for i := 0; i < 6; i++ {
		if err := b.Publish(v1.EventMailboxChanged, "acc1", "inbox"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	// Ring holds sequences 3..6. A subscriber at 5 replays 6 without resync.
	replay, _, resync, cancel, err := b.Subscribe(5)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if resync {
		t.Fatal("unexpected resync: event 6 is retained")
	}
	if len(replay) != 1 || replay[0].Sequence != 6 {
		t.Fatalf("replay = %+v, want [seq 6]", replay)
	}
	// A subscriber at 2 has no gap (event 3 is retained); one at 1 missed
	// event 2: resync plus everything retained.
	replay, _, resync, cancel2, err := b.Subscribe(1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel2()
	if !resync {
		t.Fatal("gap not detected: event 2 dropped before subscribe")
	}
	if len(replay) != 4 {
		t.Fatalf("replay after gap = %d events, want 4", len(replay))
	}
}

func TestSlowSubscriberGetsResyncInsteadOfGrowth(t *testing.T) {
	b := NewBroker(Options{RingCapacity: 8, SubscriberBuffer: 2, CoalesceWindow: time.Nanosecond})
	_, ch, _, cancel, err := b.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	for i := 0; i < 10; i++ {
		if err := b.Publish(v1.EventMailboxChanged, "acc1", "inbox"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	// The queue overflowed: it must never exceed its bound, and a resync
	// marker must be present (events delivered after the marker stay valid:
	// the client reconciles, then applies them).
	n := 0
	sawResync := false
	for {
		select {
		case ev := <-ch:
			n++
			if ev.Type == v1.EventResync {
				sawResync = true
			}
			continue
		default:
		}
		break
	}
	if n > 2 {
		t.Fatalf("subscriber queue grew to %d, bound 2", n)
	}
	if !sawResync {
		t.Fatal("no resync marker queued after overflow")
	}
}

func TestSubscriberLimitAndCancel(t *testing.T) {
	b := NewBroker(Options{MaxSubscribers: 2})
	var cancels []func()
	for i := 0; i < 2; i++ {
		_, _, _, cancel, err := b.Subscribe(0)
		if err != nil {
			t.Fatal(err)
		}
		cancels = append(cancels, cancel)
	}
	if _, _, _, _, err := b.Subscribe(0); err != ErrTooManySubscribers {
		t.Fatalf("third subscribe err = %v, want ErrTooManySubscribers", err)
	}
	cancels[0]()
	cancels[0]() // idempotent
	if _, _, _, _, err := b.Subscribe(0); err != nil {
		t.Fatalf("subscribe after cancel: %v", err)
	}
	if got := b.Subscribers(); got != 2 {
		t.Fatalf("subscribers = %d, want 2", got)
	}
}

func TestLiveDeliveryAndClose(t *testing.T) {
	b := NewBroker(Options{})
	_, ch, _, cancel, err := b.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if err := b.Publish(v1.EventDraftsChanged, "acc1", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		if ev.Type != v1.EventDraftsChanged || ev.Sequence != 1 {
			t.Fatalf("event = %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("live event not delivered")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("channel not closed on Close")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed on Close")
	}
	if err := b.Publish(v1.EventMailboxChanged, "a", "b"); err != ErrClosed {
		t.Fatalf("publish after close = %v, want ErrClosed", err)
	}
	if _, _, _, _, err := b.Subscribe(0); err != ErrClosed {
		t.Fatalf("subscribe after close = %v, want ErrClosed", err)
	}
}
