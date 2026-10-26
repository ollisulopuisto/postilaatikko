package replycontent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hkdb/aerion/internal/views"
)

func ref(account, message string) views.MessageRef {
	return views.MessageRef{AccountID: account, MessageID: message}
}

// newTestService counts calls to the real extractors.
func newTestService(maxConcurrent int) (*Service, *atomic.Int32) {
	var calls atomic.Int32
	s := NewService(ServiceOptions{MaxConcurrent: maxConcurrent, CacheEntries: 50, CacheBytes: 1 << 20})
	plain, html := s.plain, s.html
	s.plain = func(b string) Result { calls.Add(1); return plain(b) }
	s.html = func(b string) Result { calls.Add(1); return html(b) }
	return s, &calls
}

const replyText = "Tuesday works.\n\nOn Mon, Oct 19, 2026 at 9:00 AM Alice <a@example.org> wrote:\n> Could we meet?\n"
const replyHTML = `<div>Tuesday works.</div><div class="gmail_quote"><blockquote class="gmail_quote">Could we meet?</blockquote></div>`

func TestServiceNeverFetchesAndReportsContentState(t *testing.T) {
	s, calls := newTestService(2)
	tests := []struct {
		name  string
		body  Body
		state views.ContentState
	}{
		{"header only", Body{Fetched: false}, views.ContentNotFetched},
		{"header only with a stray body", Body{Fetched: false, Text: replyText}, views.ContentNotFetched},
		{"fetch failed", Body{Failed: true}, views.ContentFetchFailed},
		{"fetched but empty", Body{Fetched: true}, views.ContentCached},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply, err := s.Extract(context.Background(), "", ref("acc", "m"), tt.body)
			if err != nil {
				t.Fatal(err)
			}
			if reply.State != tt.state || reply.Result.Status != StatusUnavailable || len(reply.Result.Blocks) != 0 {
				t.Errorf("reply = %+v, want unavailable with state %q", reply, tt.state)
			}
			if err := reply.Result.Validate(); err != nil {
				t.Errorf("invalid result: %v", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Errorf("extractors ran %d times for bodies that are not available", calls.Load())
	}
}

func TestServicePicksTheRichestUsableExtraction(t *testing.T) {
	s, _ := newTestService(2)
	ctx := context.Background()

	reply, _ := s.Extract(ctx, "", ref("acc", "html"), Body{Fetched: true, Text: replyText, HTML: replyHTML})
	if reply.Source != SourceHTML || reply.Result.Blocks[0].Format != PartHTML {
		t.Errorf("html and text present: source %q, format %q; want html", reply.Source, reply.Result.Blocks[0].Format)
	}

	reply, _ = s.Extract(ctx, "", ref("acc", "text"), Body{Fetched: true, Text: replyText})
	if reply.Source != SourceText || reply.Result.Blocks[0].Format != PartText {
		t.Errorf("text only: source %q; want text", reply.Source)
	}

	// An HTML body the HTML extractor cannot split is not better than clean text.
	quoteOnlyHTML := `<div class="gmail_quote"><blockquote class="gmail_quote">only history</blockquote></div>`
	reply, _ = s.Extract(ctx, "", ref("acc", "mixed"), Body{Fetched: true, Text: replyText, HTML: quoteOnlyHTML})
	if reply.Source != SourceText || reply.Result.Status != StatusExtracted {
		t.Errorf("html fallback with good text: source %q status %q; want text/extracted", reply.Source, reply.Result.Status)
	}

	// With nothing better, the HTML fallback is kept (still the whole body, visible).
	reply, _ = s.Extract(ctx, "", ref("acc", "html-only-fallback"), Body{Fetched: true, HTML: quoteOnlyHTML})
	if reply.Source != SourceHTML || reply.Result.Status != StatusFallback {
		t.Errorf("html-only fallback: source %q status %q", reply.Source, reply.Result.Status)
	}
	for _, r := range []Reply{reply} {
		if err := r.Result.Validate(); err != nil {
			t.Errorf("invalid: %v", err)
		}
	}
}

func TestServiceCachesByBodyHashAndAccount(t *testing.T) {
	s, calls := newTestService(2)
	ctx := context.Background()
	body := Body{Fetched: true, Text: replyText}

	first, _ := s.Extract(ctx, "", ref("acc-a", "m1"), body)
	second, _ := s.Extract(ctx, "", ref("acc-a", "m1"), body)
	if calls.Load() != 1 || first.FromCache || !second.FromCache {
		t.Fatalf("calls = %d, first.FromCache = %v, second.FromCache = %v; want one extraction and a hit", calls.Load(), first.FromCache, second.FromCache)
	}

	// The body changed (for example it was fetched in full later): recompute.
	changed := Body{Fetched: true, Text: replyText + "\nPS: bring slides.\n"}
	third, _ := s.Extract(ctx, "", ref("acc-a", "m1"), changed)
	if calls.Load() != 2 || third.FromCache {
		t.Errorf("a changed body hit the cache (calls = %d)", calls.Load())
	}

	// The same message id in another account is another message.
	s.Extract(ctx, "", ref("acc-b", "m1"), body)
	if calls.Load() != 3 {
		t.Errorf("a second account reused the first account's entry (calls = %d)", calls.Load())
	}

	// Different text/html split with the same concatenation must not collide.
	a, _ := s.Extract(ctx, "", ref("acc-a", "m2"), Body{Fetched: true, Text: "ab", HTML: "<div>c</div>"})
	b, _ := s.Extract(ctx, "", ref("acc-a", "m2"), Body{Fetched: true, Text: "a", HTML: "b<div>c</div>"})
	if b.FromCache || a.Result.Blocks[0].Content == b.Result.Blocks[0].Content {
		t.Errorf("body hashes collide across the text/html boundary")
	}
}

func TestServiceDoesNotCacheDecryptedContent(t *testing.T) {
	s, calls := newTestService(2)
	ctx := context.Background()
	body := Body{Fetched: true, Text: replyText, Sensitive: true}
	s.Extract(ctx, "", ref("acc", "enc"), body)
	reply, _ := s.Extract(ctx, "", ref("acc", "enc"), body)
	if reply.FromCache || calls.Load() != 2 {
		t.Errorf("decrypted content was cached (calls = %d)", calls.Load())
	}
	if s.cache.Len() != 0 {
		t.Errorf("cache holds %d entries after sensitive extractions", s.cache.Len())
	}
}

func TestServiceReturnsCancellationAndDoesNotCache(t *testing.T) {
	s, _ := newTestService(2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Extract(ctx, "", ref("acc", "m"), Body{Fetched: true, Text: replyText}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled before start: err = %v", err)
	}

	// Cancelled while the work is running.
	started, release := make(chan struct{}), make(chan struct{})
	s.plain = func(b string) Result { close(started); <-release; return ExtractPlain(b) }
	ctx, cancel = context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := s.Extract(ctx, "", ref("acc", "m"), Body{Fetched: true, Text: replyText})
		done <- err
	}()
	<-started
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled during work: err = %v", err)
	}
	if s.cache.Len() != 0 {
		t.Error("a cancelled extraction was cached")
	}
}

// A newer request on the same channel (the reader selected another thread)
// makes the older one return ErrSuperseded instead of a stale result.
func TestServiceSupersedesStaleRequestsPerChannel(t *testing.T) {
	s, _ := newTestService(4)
	started, release := make(chan string, 4), make(chan struct{})
	real := s.plain
	s.plain = func(b string) Result {
		started <- b
		if strings.HasPrefix(b, "old") {
			<-release
		}
		return real(b)
	}

	oldDone := make(chan error, 1)
	go func() {
		_, err := s.Extract(context.Background(), "reader", ref("acc", "old"), Body{Fetched: true, Text: "old thread body"})
		oldDone <- err
	}()
	<-started

	newReply, err := s.Extract(context.Background(), "reader", ref("acc", "new"), Body{Fetched: true, Text: "new thread body"})
	if err != nil || newReply.Result.Status != StatusExtracted {
		t.Fatalf("new request: %+v, %v", newReply, err)
	}
	close(release)
	if err := <-oldDone; !errors.Is(err, ErrSuperseded) {
		t.Errorf("stale request: err = %v, want ErrSuperseded", err)
	}

	// Another channel is not affected, and an empty channel never supersedes.
	other, err := s.Extract(context.Background(), "agent", ref("acc", "x"), Body{Fetched: true, Text: "other channel"})
	if err != nil || other.Result.Status != StatusExtracted {
		t.Errorf("other channel: %+v, %v", other, err)
	}
}

func TestServiceBoundsConcurrentWork(t *testing.T) {
	const limit = 2
	s, _ := newTestService(limit)
	var running, peak atomic.Int32
	real := s.plain
	s.plain = func(b string) Result {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		running.Add(-1)
		return real(b)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := Body{Fetched: true, Text: "message number " + string(rune('a'+i))}
			if _, err := s.Extract(context.Background(), "", ref("acc", string(rune('a'+i))), body); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if peak.Load() > limit {
		t.Errorf("peak concurrency %d exceeds the limit %d", peak.Load(), limit)
	}
}

func TestServiceWaitingForASlotHonorsCancellation(t *testing.T) {
	s, _ := newTestService(1)
	started, release := make(chan struct{}), make(chan struct{})
	real := s.plain
	s.plain = func(b string) Result {
		if b == "holder" {
			close(started)
			<-release
		}
		return real(b)
	}
	go s.Extract(context.Background(), "", ref("acc", "holder"), Body{Fetched: true, Text: "holder"})
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := s.Extract(ctx, "", ref("acc", "waiter"), Body{Fetched: true, Text: "waiter"})
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waiting request: err = %v, want deadline exceeded", err)
	}
}

func TestServiceBoundsInputSize(t *testing.T) {
	s, calls := newTestService(2)
	huge := strings.Repeat("a", maxServiceInputBytes+1)

	reply, err := s.Extract(context.Background(), "", ref("acc", "big-html"), Body{Fetched: true, HTML: huge, Text: replyText})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Source != SourceText || reply.Result.Status != StatusExtracted {
		t.Errorf("oversize html with usable text: source %q status %q", reply.Source, reply.Result.Status)
	}

	calls.Store(0)
	reply, _ = s.Extract(context.Background(), "", ref("acc", "big-only"), Body{Fetched: true, HTML: huge})
	if reply.Result.Status != StatusUnavailable || len(reply.Result.Warnings) != 1 || reply.Result.Warnings[0] != WarnInputTooLarge {
		t.Errorf("oversize html only: %+v", reply.Result)
	}
	if err := reply.Result.Validate(); err != nil {
		t.Errorf("invalid: %v", err)
	}
	if calls.Load() != 0 {
		t.Error("an oversize body reached a parser")
	}
}

func TestServiceResultsAreValidAndIndependentOfTheCache(t *testing.T) {
	s, _ := newTestService(2)
	ctx := context.Background()
	body := Body{Fetched: true, HTML: replyHTML}
	first, _ := s.Extract(ctx, "", ref("acc", "m"), body)
	if err := first.Result.Validate(); err != nil {
		t.Fatal(err)
	}
	first.Result.Blocks[0].Content = "tampered"
	second, _ := s.Extract(ctx, "", ref("acc", "m"), body)
	if second.Result.Blocks[0].Content == "tampered" {
		t.Error("a caller's edit reached the cache")
	}
}

func TestServiceConcurrentUseIsRaceFree(t *testing.T) {
	s, _ := newTestService(3)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				id := string(rune('a' + (g+i)%10))
				s.Extract(context.Background(), "reader", ref("acc", id), Body{Fetched: true, Text: "body " + id})
				if i%13 == 0 {
					s.PurgeAccount("acc")
				}
			}
		}(g)
	}
	wg.Wait()
}
