package claims

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	listenBackoff    = time.Second
	maxListenBackoff = 30 * time.Second
)

type hub struct {
	mu      sync.Mutex
	changed chan struct{}
	waiting map[string]int
	total   int
}

func newHub() *hub {
	return &hub{changed: make(chan struct{}), waiting: map[string]int{}}
}

func (h *hub) wait() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.changed
}

func (h *hub) wake() {
	h.mu.Lock()
	defer h.mu.Unlock()
	close(h.changed)
	h.changed = make(chan struct{})
}

func (h *hub) acquire(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.waiting[key] >= maxWaitersPerClaimer || h.total >= maxWaiters {
		return false
	}
	h.waiting[key]++
	h.total++
	return true
}

func (h *hub) release(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.waiting[key] == 0 {
		return
	}
	h.waiting[key]--
	if h.waiting[key] == 0 {
		delete(h.waiting, key)
	}
	h.total--
}

func (s *Service) Run(ctx context.Context) {
	backoff := listenBackoff
	for ctx.Err() == nil {
		established := false
		err := s.listen(ctx, revisions.WakeChannel, func() {
			established = true
			s.hub.wake()
		}, func(string) { s.hub.wake() })
		if ctx.Err() != nil {
			return
		}
		s.logger.Warn("agent claim listener failed", logattrs.Failure("claims_listener_failed", err, slog.Duration("retry_in", backoff))...)
		if established {
			backoff = listenBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxListenBackoff)
	}
}
