package webhooks

import (
	"context"
	"time"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type cursor struct {
	xid uint64
	seq int64
}

func cursorOf(e activity.Event) cursor {
	return cursor{xid: e.Position.Xid(), seq: e.Position.Seq()}
}

type audience struct {
	inner folders.Audience
}

type source struct {
	store *activity.Store
}

func (s source) audience(ctx context.Context, reader auth.Principal) (audience, error) {
	inner, err := s.store.Audience(ctx, reader)
	return audience{inner: inner}, err
}

func (s source) next(ctx context.Context, a audience, types []string, from cursor, limit int) ([]activity.Event, cursor, error) {
	return s.read(ctx, a, activity.Filter{Types: types}, from, limit)
}

func (s source) unexpired(ctx context.Context, a audience, types []string, now time.Time, from cursor, limit int) ([]activity.Event, error) {
	page, _, err := s.read(ctx, a, activity.Filter{Types: types, Since: expiresBefore(now)}, from, limit)
	return page, err
}

func (s source) read(ctx context.Context, a audience, f activity.Filter, from cursor, limit int) ([]activity.Event, cursor, error) {
	page, to, _, err := s.store.Next(ctx, a.inner, f, activity.At(from.xid, from.seq), limit)
	return page, cursor{xid: to.Xid(), seq: to.Seq()}, err
}

func expiresBefore(now time.Time) time.Time {
	return now.Add(-expiry)
}

func expired(e activity.Event, now time.Time) bool {
	return e.CreatedAt.Before(expiresBefore(now))
}

func expiredRun(page []activity.Event, now time.Time) int {
	n := 0
	for n < len(page) && expired(page[n], now) {
		n++
	}
	return n
}
