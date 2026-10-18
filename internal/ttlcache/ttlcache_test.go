package ttlcache

import (
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time { return f.now }

func newTestCache(ttl time.Duration, maxCost int64) (*Cache[string, string], *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	c := New[string, string](ttl, maxCost)
	c.now = clock.Now
	return c, clock
}

func TestCacheExpiresAfterTTL(t *testing.T) {
	c, clock := newTestCache(10*time.Second, 100)
	c.Put("a", "1", 1)

	clock.now = clock.now.Add(10*time.Second - time.Nanosecond)
	if _, _, ok := c.Get("a"); !ok {
		t.Fatal("entry expired before its ttl")
	}
	clock.now = clock.now.Add(time.Nanosecond)
	if _, _, ok := c.Get("a"); ok {
		t.Fatal("entry survived its ttl")
	}
	if c.cost != 0 || len(c.items) != 0 {
		t.Fatalf("expired entry still accounted: cost %d, items %d", c.cost, len(c.items))
	}
}

func TestCacheZeroTTLKeepsEntries(t *testing.T) {
	c, clock := newTestCache(0, 100)
	c.Put("a", "1", 1)
	clock.now = clock.now.Add(1000 * time.Hour)
	if _, _, ok := c.Get("a"); !ok {
		t.Fatal("entry expired with a zero ttl")
	}
}

func TestCacheEvictsLeastRecentlyUsedByCost(t *testing.T) {
	c, _ := newTestCache(0, 10)
	c.Put("a", "1", 4)
	c.Put("b", "2", 4)
	if _, _, ok := c.Get("a"); !ok {
		t.Fatal("a missing")
	}
	c.Put("c", "3", 4)

	if _, _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted as the least recently used")
	}
	for _, key := range []string{"a", "c"} {
		if _, _, ok := c.Get(key); !ok {
			t.Fatalf("%s was evicted", key)
		}
	}
	if c.cost != 8 {
		t.Fatalf("cost = %d, want 8", c.cost)
	}
}

func TestCacheEvictsSeveralEntriesForOneLargeEntry(t *testing.T) {
	c, _ := newTestCache(0, 10)
	c.Put("a", "1", 3)
	c.Put("b", "2", 3)
	c.Put("c", "3", 3)
	c.Put("d", "4", 9)

	for _, key := range []string{"a", "b", "c"} {
		if _, _, ok := c.Get(key); ok {
			t.Fatalf("%s should have been evicted", key)
		}
	}
	if _, _, ok := c.Get("d"); !ok {
		t.Fatal("d missing")
	}
}

func TestCacheNeverStoresOversizedEntries(t *testing.T) {
	c, _ := newTestCache(0, 10)
	c.Put("a", "1", 4)
	c.Put("big", "x", 11)
	if _, _, ok := c.Get("big"); ok {
		t.Fatal("oversized entry was stored")
	}
	if _, _, ok := c.Get("a"); !ok {
		t.Fatal("oversized entry evicted a smaller one")
	}

	c.Put("a", "2", 11)
	if _, _, ok := c.Get("a"); ok {
		t.Fatal("replacing with an oversized value must drop the stale entry")
	}
	if c.cost != 0 {
		t.Fatalf("cost = %d, want 0", c.cost)
	}
}

func TestCacheReplaceAdjustsCost(t *testing.T) {
	c, _ := newTestCache(0, 10)
	c.Put("a", "1", 4)
	c.Put("a", "2", 6)
	if c.cost != 6 || len(c.items) != 1 {
		t.Fatalf("after replace: cost %d, items %d", c.cost, len(c.items))
	}
	c.Put("b", "3", 4)
	if _, _, ok := c.Get("a"); !ok {
		t.Fatal("a evicted although 6 + 4 fits")
	}
	if got, _, _ := c.Get("a"); got != "2" {
		t.Fatalf("a = %q, want the replacement", got)
	}

	c.Put("a", "3", 1)
	if c.cost != 5 {
		t.Fatalf("cost after shrinking = %d, want 5", c.cost)
	}
}

func TestCacheGetReturnsStoredTime(t *testing.T) {
	c, clock := newTestCache(time.Minute, 10)
	stored := c.Put("a", "1", 1)
	if !stored.Equal(clock.now) {
		t.Fatalf("Put returned %v, want %v", stored, clock.now)
	}
	clock.now = clock.now.Add(30 * time.Second)
	_, at, ok := c.Get("a")
	if !ok || !at.Equal(stored) {
		t.Fatalf("Get stored-at = %v (ok %v), want %v", at, ok, stored)
	}
}
