package replycontent

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func cacheResult(content string) Result {
	return Result{
		Version: ExtractorVersion,
		Status:  StatusExtracted,
		Blocks:  []Block{{ID: "b1", Kind: BlockAuthored, Format: PartText, Content: content, Source: &Source{Part: PartText, Start: 0, End: len(content)}}},
	}
}

func key(account, message, hash string) cacheKey {
	return cacheKey{AccountID: account, MessageID: message, BodyHash: hash, Version: ExtractorVersion}
}

func TestCacheStoresAndReturnsCopies(t *testing.T) {
	c := NewCache(10, 1<<20)
	k := key("acc", "m1", "h1")
	if _, ok := c.Get(k); ok {
		t.Fatal("empty cache returned a hit")
	}
	if !c.Put(k, cacheResult("hello")) {
		t.Fatal("Put refused a small entry")
	}
	got, ok := c.Get(k)
	if !ok || got.Blocks[0].Content != "hello" {
		t.Fatalf("Get = %+v, %v", got, ok)
	}

	// Mutating what Get returned must not change what is cached.
	got.Blocks[0].Content = "changed"
	got.Blocks[0].Source.End = 99
	again, _ := c.Get(k)
	if again.Blocks[0].Content != "hello" || again.Blocks[0].Source.End != 5 {
		t.Errorf("cached entry was mutated through a returned copy: %+v", again.Blocks[0])
	}
}

func TestCacheKeysSeparateAccountsBodiesAndVersions(t *testing.T) {
	c := NewCache(10, 1<<20)
	c.Put(key("acc-a", "m1", "h1"), cacheResult("a"))
	c.Put(key("acc-b", "m1", "h1"), cacheResult("b"))
	if got, _ := c.Get(key("acc-a", "m1", "h1")); got.Blocks[0].Content != "a" {
		t.Error("two accounts with one message id share an entry")
	}
	if _, ok := c.Get(key("acc-a", "m1", "h2")); ok {
		t.Error("a changed body hash hit the old entry")
	}
	old := key("acc-a", "m1", "h1")
	old.Version = "reply-v0"
	if _, ok := c.Get(old); ok {
		t.Error("another extractor version hit the entry")
	}
}

func TestCacheEvictsLeastRecentlyUsedByCount(t *testing.T) {
	c := NewCache(2, 1<<20)
	c.Put(key("a", "1", "h"), cacheResult("one"))
	c.Put(key("a", "2", "h"), cacheResult("two"))
	c.Get(key("a", "1", "h")) // makes entry 2 the oldest
	c.Put(key("a", "3", "h"), cacheResult("three"))
	if _, ok := c.Get(key("a", "2", "h")); ok {
		t.Error("the least recently used entry survived")
	}
	for _, id := range []string{"1", "3"} {
		if _, ok := c.Get(key("a", id, "h")); !ok {
			t.Errorf("entry %s was evicted", id)
		}
	}
	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}
}

func TestCacheEvictsByBytesAndRefusesOversizeEntries(t *testing.T) {
	big := strings.Repeat("x", 1000)
	one := entrySize(cacheResult(big))
	c := NewCache(100, one*2+one/2) // room for two entries
	c.Put(key("a", "1", "h"), cacheResult(big))
	c.Put(key("a", "2", "h"), cacheResult(big))
	c.Put(key("a", "3", "h"), cacheResult(big))
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2 after byte eviction", c.Len())
	}
	if c.Size() > one*2+one/2 {
		t.Errorf("Size %d exceeds the limit", c.Size())
	}
	if c.Put(key("a", "4", "h"), cacheResult(strings.Repeat("y", 100000))) {
		t.Error("an entry larger than the whole cache was stored")
	}
	if c.Len() != 2 {
		t.Errorf("a refused entry must not evict others; Len = %d", c.Len())
	}
}

func TestCacheReplacingAnEntryKeepsSizeExact(t *testing.T) {
	c := NewCache(10, 1<<20)
	k := key("a", "1", "h")
	c.Put(k, cacheResult(strings.Repeat("x", 500)))
	before := c.Size()
	c.Put(k, cacheResult(strings.Repeat("x", 500)))
	if c.Size() != before || c.Len() != 1 {
		t.Errorf("replace changed size %d -> %d, len %d", before, c.Size(), c.Len())
	}
	c.Put(k, cacheResult("s"))
	if c.Size() >= before {
		t.Errorf("a smaller replacement did not shrink the cache: %d >= %d", c.Size(), before)
	}
}

func TestCachePurge(t *testing.T) {
	c := NewCache(10, 1<<20)
	c.Put(key("a", "1", "h"), cacheResult("one"))
	c.Put(key("a", "2", "h"), cacheResult("two"))
	c.Put(key("b", "1", "h"), cacheResult("three"))
	c.PurgeAccount("a")
	if c.Len() != 1 {
		t.Fatalf("Len = %d after PurgeAccount, want 1", c.Len())
	}
	if _, ok := c.Get(key("b", "1", "h")); !ok {
		t.Error("PurgeAccount removed another account's entry")
	}
	c.Purge()
	if c.Len() != 0 || c.Size() != 0 {
		t.Errorf("Purge left len %d size %d", c.Len(), c.Size())
	}
}

func TestCacheIsSafeForConcurrentUse(t *testing.T) {
	c := NewCache(8, 1<<16)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				k := key("a", fmt.Sprint((g*7+i)%20), "h")
				c.Put(k, cacheResult(strings.Repeat("z", i%50+1)))
				c.Get(k)
				if i%97 == 0 {
					c.PurgeAccount("a")
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Len() > 8 {
		t.Errorf("Len = %d exceeds the entry limit", c.Len())
	}
}
