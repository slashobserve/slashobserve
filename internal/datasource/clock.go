package datasource

import (
	"context"
	"time"
)

type clockKey struct{}

func WithNow(ctx context.Context, at time.Time) context.Context {
	return context.WithValue(ctx, clockKey{}, at)
}

func PinnedNow(ctx context.Context) (time.Time, bool) {
	at, ok := ctx.Value(clockKey{}).(time.Time)
	return at, ok && !at.IsZero()
}

func Now(ctx context.Context) time.Time {
	if at, ok := PinnedNow(ctx); ok {
		return at
	}
	return time.Now()
}
