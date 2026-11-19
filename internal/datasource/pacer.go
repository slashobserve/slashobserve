package datasource

import (
	"context"
	"errors"
	"time"
)

const windowReserve = time.Second

type Pacer struct {
	took, span time.Duration
}

type Window struct {
	Ctx     context.Context
	parent  context.Context
	cancel  context.CancelFunc
	pacer   *Pacer
	started time.Time
	span    time.Duration
}

func (p *Pacer) Start(ctx context.Context, span time.Duration) (*Window, bool) {
	w := &Window{Ctx: ctx, parent: ctx, cancel: func() {}, pacer: p, started: time.Now(), span: span}
	deadline, bounded := ctx.Deadline()
	if !bounded {
		return w, true
	}
	budget := time.Until(deadline) - windowReserve
	if budget <= 0 || p.projected(span) > budget {
		return nil, false
	}
	w.Ctx, w.cancel = context.WithTimeout(ctx, budget)
	return w, true
}

func (p *Pacer) projected(span time.Duration) time.Duration {
	if p.span <= 0 {
		return 0
	}
	return time.Duration(float64(p.took) * float64(span) / float64(p.span))
}

func (w *Window) End(err error) (ranOut bool) {
	ranOut = err != nil && errors.Is(w.Ctx.Err(), context.DeadlineExceeded) && w.parent.Err() == nil
	w.cancel()
	if err == nil {
		w.pacer.took, w.pacer.span = time.Since(w.started), w.span
	}
	return ranOut
}

func NewestFirst(r TimeRange, first time.Duration) []TimeRange {
	var out []TimeRange
	to, span := r.To, first
	for to.After(r.From) {
		from := to.Add(-span)
		if from.Before(r.From) {
			from = r.From
		}
		out = append(out, TimeRange{From: from, To: to})
		to, span = from, span*2
	}
	return out
}
