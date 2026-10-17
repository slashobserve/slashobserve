package ttlcache

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

type Group[V any] struct {
	flights singleflight.Group
	timeout time.Duration
}

func NewGroup[V any](timeout time.Duration) *Group[V] {
	return &Group[V]{timeout: timeout}
}

// The shared call runs on a context detached from every caller, so one caller going away does not fail the others;
// each caller still stops waiting when its own context ends.
func (g *Group[V]) Do(ctx context.Context, key string, fn func(context.Context) (V, error)) (V, error) {
	ch := g.flights.DoChan(key, func() (any, error) {
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.timeout)
		defer cancel()
		return fn(shared)
	})
	select {
	case res := <-ch:
		value, _ := res.Val.(V)
		return value, res.Err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}
