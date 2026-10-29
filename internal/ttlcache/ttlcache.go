package ttlcache

import (
	"container/list"
	"sync"
	"time"
)

type Cache[K comparable, V any] struct {
	ttl     time.Duration
	maxCost int64
	now     func() time.Time

	mu    sync.Mutex
	order *list.List
	items map[K]*list.Element
	cost  int64
}

type entry[K comparable, V any] struct {
	key    K
	value  V
	cost   int64
	stored time.Time
}

// A zero ttl keeps entries until the cost bound evicts them, and an entry costing more than maxCost is never stored.
func New[K comparable, V any](ttl time.Duration, maxCost int64) *Cache[K, V] {
	return &Cache[K, V]{ttl: ttl, maxCost: maxCost, order: list.New(), items: map[K]*list.Element{}, now: time.Now}
}

func (c *Cache[K, V]) Get(key K) (value V, stored time.Time, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, found := c.items[key]
	if !found {
		return value, stored, false
	}
	e := el.Value.(*entry[K, V])
	if c.ttl > 0 && c.now().Sub(e.stored) >= c.ttl {
		c.remove(el)
		return value, stored, false
	}
	c.order.MoveToFront(el)
	return e.value, e.stored, true
}

func (c *Cache[K, V]) Put(key K, value V, cost int64) (stored time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stored = c.now()
	if el, found := c.items[key]; found {
		c.remove(el)
	}
	c.sweepExpired(stored)
	if cost > c.maxCost {
		return stored
	}
	c.items[key] = c.order.PushFront(&entry[K, V]{key: key, value: value, cost: cost, stored: stored})
	c.cost += cost
	for c.cost > c.maxCost {
		c.remove(c.order.Back())
	}
	return stored
}

func (c *Cache[K, V]) sweepExpired(now time.Time) {
	if c.ttl <= 0 {
		return
	}
	for el := c.order.Back(); el != nil; el = c.order.Back() {
		if now.Sub(el.Value.(*entry[K, V]).stored) < c.ttl {
			return
		}
		c.remove(el)
	}
}

func (c *Cache[K, V]) remove(el *list.Element) {
	e := c.order.Remove(el).(*entry[K, V])
	delete(c.items, e.key)
	c.cost -= e.cost
}
