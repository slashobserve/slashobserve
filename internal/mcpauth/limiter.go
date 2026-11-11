package mcpauth

import (
	"sync"
	"time"
)

type limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	counts map[string]int
	start  time.Time
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{limit: limit, window: window, now: time.Now, counts: map[string]int{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.start.IsZero() || now.Sub(l.start) >= l.window {
		l.counts = map[string]int{}
		l.start = now
	}
	if l.counts[key] >= l.limit {
		return false
	}
	l.counts[key]++
	return true
}

func (l *limiter) retryAfterSeconds() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	left := l.start.Add(l.window).Sub(l.now())
	return max(1, int((left+time.Second-1)/time.Second))
}
