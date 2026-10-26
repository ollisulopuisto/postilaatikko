package replycontent

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"github.com/hkdb/aerion/internal/views"
)

const (
	// maxServiceInputBytes bounds each body part the service will analyze. The
	// parsers guard their own classification at 1 MiB, but the HTML fallback still
	// sanitizes its whole input, so the service refuses larger parts outright.
	maxServiceInputBytes = 8 << 20

	defaultMaxConcurrent = 4
	defaultCacheEntries  = 256
	defaultCacheBytes    = 32 << 20
)

// ErrSuperseded is returned when a newer request on the same channel started
// while this one was running. The caller selected something else; the result
// would be stale.
var ErrSuperseded = errors.New("replycontent: superseded by a newer request")

// Source names the body part a result was extracted from.
const (
	SourceHTML = "html"
	SourceText = "text"
)

// Body is a message body as the caller has it. The service never fetches: a
// header-only message yields an unavailable result, and the caller decides
// whether to run the application's explicit body fetch and ask again.
type Body struct {
	Text string
	HTML string
	// Fetched says the body was downloaded. Failed says a fetch ran and produced
	// no usable body.
	Fetched bool
	Failed  bool
	// Sensitive marks content derived from decrypted mail. It is analyzed but
	// never cached.
	Sensitive bool
}

// Reply is the extraction for one message.
type Reply struct {
	Ref   views.MessageRef   `json:"ref"`
	State views.ContentState `json:"state"`
	// Source is the body part the blocks come from; empty when there are none.
	Source    string `json:"source,omitempty"`
	FromCache bool   `json:"fromCache"`
	Result    Result `json:"result"`
}

// ServiceOptions sets the bounds. Zero values use the defaults.
type ServiceOptions struct {
	MaxConcurrent int
	CacheEntries  int
	CacheBytes    int64
}

// Service extracts reply content on demand from bodies that are already stored,
// with a bounded cache, a bound on concurrent work, cancellation, and
// supersession of stale requests.
type Service struct {
	cache *Cache
	slots chan struct{}

	plain func(string) Result
	html  func(string) Result

	mu   sync.Mutex
	gens map[string]uint64
}

// NewService returns a service with the given bounds.
func NewService(opts ServiceOptions) *Service {
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = defaultMaxConcurrent
	}
	if opts.CacheEntries <= 0 {
		opts.CacheEntries = defaultCacheEntries
	}
	if opts.CacheBytes <= 0 {
		opts.CacheBytes = defaultCacheBytes
	}
	return &Service{
		cache: NewCache(opts.CacheEntries, opts.CacheBytes),
		slots: make(chan struct{}, opts.MaxConcurrent),
		plain: ExtractPlain,
		html:  ExtractHTML,
		gens:  make(map[string]uint64),
	}
}

// PurgeAccount drops cached results of a removed account.
func (s *Service) PurgeAccount(accountID string) { s.cache.PurgeAccount(accountID) }

// Purge drops all cached results.
func (s *Service) Purge() { s.cache.Purge() }

// Extract returns the reply content of one message body.
//
// channel groups requests that replace each other, such as "reader" for the
// open thread: a newer call on the channel makes older ones return
// ErrSuperseded. An empty channel never supersedes. A cancelled context
// returns its error and caches nothing.
func (s *Service) Extract(ctx context.Context, channel string, ref views.MessageRef, body Body) (Reply, error) {
	reply := Reply{Ref: ref}
	if err := ctx.Err(); err != nil {
		return reply, err
	}
	generation := s.begin(channel)

	switch {
	case body.Failed:
		reply.State = views.ContentFetchFailed
		reply.Result = unavailable()
		return reply, nil
	case !body.Fetched:
		reply.State = views.ContentNotFetched
		reply.Result = unavailable()
		return reply, nil
	}
	reply.State = views.ContentCached

	text, htmlBody, tooLarge := usableParts(body)
	if text == "" && htmlBody == "" {
		reply.Result = unavailable()
		if tooLarge {
			reply.Result.Warnings = []string{WarnInputTooLarge}
		}
		return reply, nil
	}

	cacheable := !body.Sensitive && !tooLarge
	var key cacheKey
	if cacheable {
		key = cacheKey{AccountID: ref.AccountID, MessageID: ref.MessageID, BodyHash: hashBody(text, htmlBody), Version: ExtractorVersion}
		if cached, ok := s.cache.Get(key); ok {
			if err := s.check(ctx, channel, generation); err != nil {
				return reply, err
			}
			reply.Result, reply.FromCache = cached, true
			reply.Source = sourceOf(cached)
			return reply, nil
		}
	}

	if err := s.acquire(ctx); err != nil {
		return reply, err
	}
	result := s.extract(text, htmlBody)
	s.release()

	if err := s.check(ctx, channel, generation); err != nil {
		return reply, err
	}
	if cacheable {
		s.cache.Put(key, result)
	}
	reply.Result = cloneResult(result)
	reply.Source = sourceOf(result)
	return reply, nil
}

// extract runs the parsers and keeps the more reliable result. Rich HTML wins a
// tie; plain text is only run when the HTML result is not a clean extraction.
func (s *Service) extract(text, htmlBody string) Result {
	var htmlResult Result
	if htmlBody != "" {
		htmlResult = s.html(htmlBody)
		if htmlResult.Status == StatusExtracted {
			return htmlResult
		}
	}
	if text == "" {
		return htmlResult
	}
	textResult := s.plain(text)
	if htmlBody != "" && rank(htmlResult.Status) >= rank(textResult.Status) {
		return htmlResult
	}
	return textResult
}

func rank(s Status) int {
	switch s {
	case StatusExtracted:
		return 3
	case StatusPartial:
		return 2
	case StatusFallback:
		return 1
	}
	return 0
}

// sourceOf tells which part a result's blocks were extracted from.
func sourceOf(r Result) string {
	if len(r.Blocks) == 0 {
		return ""
	}
	if r.Blocks[0].Format == PartHTML {
		return SourceHTML
	}
	return SourceText
}

func unavailable() Result {
	return Result{Version: ExtractorVersion, Status: StatusUnavailable}
}

// usableParts returns the body parts worth analyzing: blank parts and parts over
// the service limit are dropped, and tooLarge reports that a part was dropped
// for size.
func usableParts(body Body) (text, htmlBody string, tooLarge bool) {
	if strings.TrimSpace(body.Text) != "" {
		if len(body.Text) > maxServiceInputBytes {
			tooLarge = true
		} else {
			text = body.Text
		}
	}
	if strings.TrimSpace(body.HTML) != "" {
		if len(body.HTML) > maxServiceInputBytes {
			tooLarge = true
		} else {
			htmlBody = body.HTML
		}
	}
	return text, htmlBody, tooLarge
}

// hashBody hashes both parts with their lengths, so moving text between the two
// parts cannot produce the same hash.
func hashBody(text, htmlBody string) string {
	h := sha256.New()
	var length [binary.MaxVarintLen64]byte
	for _, part := range []string{text, htmlBody} {
		n := binary.PutUvarint(length[:], uint64(len(part)))
		h.Write(length[:n])
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) begin(channel string) uint64 {
	if channel == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gens[channel]++
	return s.gens[channel]
}

// check reports why a finished request must not be delivered.
func (s *Service) check(ctx context.Context, channel string, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if channel == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gens[channel] != generation {
		return ErrSuperseded
	}
	return nil
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) release() { <-s.slots }
