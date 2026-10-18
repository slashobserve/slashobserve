package ttlcache

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrPanic = errors.New("internal error")

type Group[V any] struct {
	timeout time.Duration

	mu    sync.Mutex
	calls map[string]*call[V]
}

type call[V any] struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	value   V
	err     error
}

func newCall[V any](ctx context.Context, timeout time.Duration) *call[V] {
	shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout) //nolint:gosec // cancel is stored on the call and invoked when it finishes or its last caller leaves
	return &call[V]{ctx: shared, cancel: cancel, done: make(chan struct{})}
}

func NewGroup[V any](timeout time.Duration) *Group[V] {
	return &Group[V]{timeout: timeout, calls: map[string]*call[V]{}}
}

// The shared call runs on a context detached from every caller, so one caller going away does not fail the others,
// and it is canceled once the last waiting caller is gone so abandoned work stops holding upstream capacity.
func (g *Group[V]) Do(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error) {
	g.mu.Lock()
	c, ok := g.calls[key]
	if !ok {
		c = newCall[V](ctx, g.timeout)
		g.calls[key] = c
		go g.run(key, c, fn)
	}
	c.waiters++
	g.mu.Unlock()

	select {
	case <-c.done:
		return c.value, c.err
	case <-ctx.Done():
		g.leave(key, c)
		var zero V
		return zero, ctx.Err()
	}
}

func (g *Group[V]) run(key string, c *call[V], fn func(context.Context) (V, error)) {
	defer func() {
		if recover() != nil {
			var zero V
			c.value, c.err = zero, ErrPanic
		}
		c.cancel()
		g.mu.Lock()
		if g.calls[key] == c {
			delete(g.calls, key)
		}
		g.mu.Unlock()
		close(c.done)
	}()
	c.value, c.err = fn(c.ctx)
}

func (g *Group[V]) leave(key string, c *call[V]) {
	g.mu.Lock()
	c.waiters--
	last := c.waiters == 0
	if last && g.calls[key] == c {
		delete(g.calls, key)
	}
	g.mu.Unlock()
	if last {
		c.cancel()
	}
}
