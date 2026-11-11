package webhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
)

type cursor struct {
	xid uint64
	seq int64
}

func cursorOf(e activity.Event) cursor {
	return cursor{xid: e.Position.Xid(), seq: e.Position.Seq()}
}

type audience struct {
	inner activity.Audience
}

type source struct {
	store *activity.Store
}

func (s source) head(ctx context.Context) (cursor, error) {
	head, _, err := s.store.Start(ctx, "")
	if err != nil {
		return cursor{}, fmt.Errorf("read activity head: %w", err)
	}
	return cursor{xid: head.Xid(), seq: head.Seq()}, nil
}

func (s source) audience(ctx context.Context, reader auth.Principal) (audience, error) {
	inner, err := s.store.Audience(ctx, reader)
	return audience{inner: inner}, err
}

func (s source) next(ctx context.Context, a audience, types []string, since time.Time, from cursor, limit int) ([]activity.Event, cursor, error) {
	page, to, _, err := s.store.Next(ctx, a.inner, activity.Filter{Types: types, Since: since}, activity.At(from.xid, from.seq), limit)
	return page, cursor{xid: to.Xid(), seq: to.Seq()}, err
}
