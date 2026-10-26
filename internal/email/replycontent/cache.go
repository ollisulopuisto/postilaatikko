package replycontent

import (
	"container/list"
	"sync"
)

// cacheKey identifies one extraction: the exact message, the exact body and the
// extractor that produced it. A body that changes, or a new extractor version,
// is a different key, so stale entries can never be returned.
type cacheKey struct {
	AccountID string
	MessageID string
	BodyHash  string
	Version   string
}

type cacheEntry struct {
	key    cacheKey
	result Result
	size   int64
}

// Cache is a bounded, in-memory least-recently-used cache of extraction
// results. It is never written to disk: derived text follows the same policy as
// the bodies it came from, and anything durable would need its own decision.
type Cache struct {
	mu         sync.Mutex
	maxEntries int
	maxBytes   int64
	bytes      int64
	order      *list.List // front is most recently used
	items      map[cacheKey]*list.Element
}

// NewCache returns a cache holding at most maxEntries results and maxBytes of
// estimated size.
func NewCache(maxEntries int, maxBytes int64) *Cache {
	return &Cache{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		order:      list.New(),
		items:      make(map[cacheKey]*list.Element),
	}
}

// Get returns a copy of the cached result, so callers cannot change the cache.
func (c *Cache) Get(key cacheKey) (Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[key]
	if !ok {
		return Result{}, false
	}
	c.order.MoveToFront(element)
	return cloneResult(element.Value.(*cacheEntry).result), true
}

// Put stores a copy of the result and evicts the least recently used entries
// until the limits hold. An entry larger than the whole cache is refused and
// evicts nothing.
func (c *Cache) Put(key cacheKey, result Result) bool {
	size := entrySize(result)
	if c.maxEntries <= 0 || size > c.maxBytes {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.items[key]; ok {
		c.remove(element)
	}
	c.items[key] = c.order.PushFront(&cacheEntry{key: key, result: cloneResult(result), size: size})
	c.bytes += size
	for c.order.Len() > c.maxEntries || c.bytes > c.maxBytes {
		c.remove(c.order.Back())
	}
	return true
}

// Len is the number of cached results.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Size is the estimated bytes held.
func (c *Cache) Size() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// Purge drops everything.
func (c *Cache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	c.items = make(map[cacheKey]*list.Element)
	c.bytes = 0
}

// PurgeAccount drops every result of one account, for example when the account
// is removed.
func (c *Cache) PurgeAccount(accountID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, element := range c.items {
		if key.AccountID == accountID {
			c.remove(element)
		}
	}
}

func (c *Cache) remove(element *list.Element) {
	entry := c.order.Remove(element).(*cacheEntry)
	delete(c.items, entry.key)
	c.bytes -= entry.size
}

// entrySize estimates the memory a result holds.
func entrySize(r Result) int64 {
	size := int64(128 + len(r.Version) + len(r.Status))
	for _, b := range r.Blocks {
		size += int64(96 + len(b.ID) + len(b.Kind) + len(b.Format) + len(b.Content))
		if b.Source != nil {
			size += 32
		}
	}
	for _, w := range r.Warnings {
		size += int64(16 + len(w))
	}
	return size
}

func cloneResult(r Result) Result {
	out := r
	if r.Blocks != nil {
		out.Blocks = make([]Block, len(r.Blocks))
		for i, b := range r.Blocks {
			out.Blocks[i] = b
			if b.Source != nil {
				source := *b.Source
				out.Blocks[i].Source = &source
			}
		}
	}
	if r.Warnings != nil {
		out.Warnings = append([]string(nil), r.Warnings...)
	}
	return out
}
