// Package mailboxevents implements the helper's bounded mailbox change event
// broker (PERF-SC-004). Helper-owned state changes publish invalidation
// events; the /v1/events SSE endpoint subscribes and streams them to desktop
// clients. The broker guarantees:
//
//   - a strictly monotonic sequence per broker lifetime;
//   - burst coalescing: repeated changes to the same type/account/folder
//     within the coalesce window publish once;
//   - bounded memory: a fixed-size replay ring, a bounded subscriber count,
//     and a bounded per-subscriber queue whose overflow degrades to a
//     resync signal instead of growth.
package mailboxevents

import (
	"errors"
	"fmt"
	"sync"
	"time"

	v1 "github.com/hkdb/aerion/internal/automation/v1"
)

// Defaults chosen so a saturated broker stays under a few hundred KB and a
// burst of per-message sync events collapses to one event per folder.
const (
	DefaultRingCapacity     = 512
	DefaultMaxSubscribers   = 16
	DefaultSubscriberBuffer = 64
	DefaultCoalesceWindow   = 500 * time.Millisecond
	// maxCoalesceKeys bounds the coalescing dedup map; past it the map resets
	// and a few duplicate events may slip through, which subscribers already
	// tolerate.
	maxCoalesceKeys = 1024
)

var (
	// ErrClosed reports use of a closed broker.
	ErrClosed = errors.New("mailboxevents: broker closed")
	// ErrTooManySubscribers reports subscription beyond the configured bound.
	// It aliases v1.ErrEventSourceSaturated so the transport maps it to a
	// retryable 503 without importing this package.
	ErrTooManySubscribers = v1.ErrEventSourceSaturated
)

// Options tunes a Broker; zero fields take the defaults above.
type Options struct {
	RingCapacity     int
	MaxSubscribers   int
	SubscriberBuffer int
	CoalesceWindow   time.Duration
	// now overrides the clock in tests.
	now func() time.Time
}

// TestClock returns an Options clone using now as the clock, for tests.
func (o Options) TestClock(now func() time.Time) Options {
	o.now = now
	return o
}

type subscriber struct {
	ch chan v1.MailboxEvent
}

// Broker is the single helper process's event hub. It is safe for concurrent
// use.
type Broker struct {
	mu             sync.Mutex
	seq            uint64
	ring           []v1.MailboxEvent
	head           int // index of oldest retained event; count==0 when empty
	count          int
	lastPub        map[string]time.Time
	subs           map[uint64]*subscriber
	nextSub        uint64
	closed         bool
	ringCap        int
	maxSubs        int
	subBuf         int
	coalesceWindow time.Duration
	now            func() time.Time
}

// NewBroker creates a broker with the given options.
func NewBroker(opts Options) *Broker {
	b := &Broker{
		lastPub:        make(map[string]time.Time),
		subs:           make(map[uint64]*subscriber),
		ringCap:        opts.RingCapacity,
		maxSubs:        opts.MaxSubscribers,
		subBuf:         opts.SubscriberBuffer,
		coalesceWindow: opts.CoalesceWindow,
		now:            opts.now,
	}
	if b.ringCap <= 0 {
		b.ringCap = DefaultRingCapacity
	}
	if b.maxSubs <= 0 {
		b.maxSubs = DefaultMaxSubscribers
	}
	if b.subBuf <= 0 {
		b.subBuf = DefaultSubscriberBuffer
	}
	if b.coalesceWindow <= 0 {
		b.coalesceWindow = DefaultCoalesceWindow
	}
	if b.now == nil {
		b.now = time.Now
	}
	b.ring = make([]v1.MailboxEvent, b.ringCap)
	return b
}

// Publish records one change. Bursts of the same type/account/folder within
// the coalesce window collapse into the first event: the earlier event still
// tells subscribers to re-read that folder, and the sequence stays gapless.
func (b *Broker) Publish(eventType, accountID, folderID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	now := b.now()
	key := eventType + "\x00" + accountID + "\x00" + folderID
	if last, ok := b.lastPub[key]; ok && now.Sub(last) < b.coalesceWindow {
		return nil
	}
	if len(b.lastPub) >= maxCoalesceKeys {
		b.lastPub = make(map[string]time.Time)
	}
	b.lastPub[key] = now

	b.seq++
	ev := v1.MailboxEvent{Sequence: b.seq, Type: eventType, AccountID: accountID, FolderID: folderID, At: now.UTC()}
	if b.count < b.ringCap {
		b.ring[(b.head+b.count)%b.ringCap] = ev
		b.count++
	} else {
		b.ring[b.head] = ev
		b.head = (b.head + 1) % b.ringCap
	}
	for _, sub := range b.subs {
		select {
		case sub.ch <- ev:
		default:
			// Slow subscriber: drop its backlog and leave a resync marker so
			// it reconciles instead of growing memory.
			drained := false
			for !drained {
				select {
				case <-sub.ch:
				default:
					drained = true
				}
			}
			sub.ch <- v1.MailboxEvent{Type: v1.EventResync, At: now.UTC()}
		}
	}
	return nil
}

// Subscribe implements v1.MailboxEventSource.
func (b *Broker) Subscribe(afterSeq uint64) ([]v1.MailboxEvent, <-chan v1.MailboxEvent, bool, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, nil, false, nil, ErrClosed
	}
	if len(b.subs) >= b.maxSubs {
		return nil, nil, false, nil, ErrTooManySubscribers
	}
	b.nextSub++
	id := b.nextSub
	sub := &subscriber{ch: make(chan v1.MailboxEvent, b.subBuf)}
	b.subs[id] = sub

	var replay []v1.MailboxEvent
	resync := false
	if afterSeq > 0 {
		oldest := b.oldestSeqLocked()
		if b.count > 0 && oldest > afterSeq+1 {
			resync = true
		}
		for i := 0; i < b.count; i++ {
			ev := b.ring[(b.head+i)%b.ringCap]
			if ev.Sequence > afterSeq {
				replay = append(replay, ev)
			}
		}
	}
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs, id)
	}
	return replay, sub.ch, resync, cancel, nil
}

// oldestSeqLocked returns the smallest retained sequence, 0 when empty.
func (b *Broker) oldestSeqLocked() uint64 {
	if b.count == 0 {
		return 0
	}
	return b.ring[b.head].Sequence
}

// Subscribers reports the live subscriber count, for tests and diagnostics.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// Sequence reports the latest published sequence, for tests and diagnostics.
func (b *Broker) Sequence() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

// Close shuts the broker down: publishers get ErrClosed and every live
// subscriber channel closes so SSE streams end promptly on helper shutdown.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	for id, sub := range b.subs {
		close(sub.ch)
		delete(b.subs, id)
	}
	return nil
}

// String describes the broker for logs.
func (b *Broker) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return fmt.Sprintf("mailboxevents.Broker{seq:%d subs:%d retained:%d}", b.seq, len(b.subs), b.count)
}
