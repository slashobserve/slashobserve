package threads

import "testing"

func TestPanelCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := newPanelCache()
	key := func(n int) panelKey { return panelKey{version: n} }
	for n := 1; n <= panelCacheSize; n++ {
		cache.put(key(n), map[string]struct{}{})
	}
	if _, ok := cache.get(key(1)); !ok {
		t.Fatal("first entry evicted before the cache was full")
	}
	cache.put(key(panelCacheSize+1), map[string]struct{}{})
	if _, ok := cache.get(key(2)); ok {
		t.Error("least recently used entry survived")
	}
	if _, ok := cache.get(key(1)); !ok {
		t.Error("recently used entry was evicted")
	}
}
