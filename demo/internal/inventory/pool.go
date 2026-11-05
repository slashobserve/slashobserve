package inventory

import (
	"context"
	"time"
)

type Pool struct {
	slots chan struct{}
}

func NewPool(size int) *Pool {
	return &Pool{slots: make(chan struct{}, size)}
}

func (p *Pool) Size() int {
	return cap(p.slots)
}

func (p *Pool) InUse() int {
	return len(p.slots)
}

func (p *Pool) Hold(ctx context.Context, d time.Duration) bool {
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return false
	}
	defer func() { <-p.slots }()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
