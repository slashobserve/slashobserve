package revisions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type asOfKey struct{}

var (
	errAsOfTime   = errors.New("as_of must be epoch milliseconds or an RFC 3339 time")
	errAsOfFuture = errors.New("as_of must not be in the future")
)

func CheckAsOf(raw string, by auth.Principal, now time.Time) (time.Time, error) {
	if !by.Role.AtLeast(auth.RoleAdmin) {
		return time.Time{}, ErrForbidden
	}
	if strings.HasPrefix(raw, "now") {
		return time.Time{}, errAsOfTime
	}
	at, err := spec.ResolveTime(raw, now, false, spec.TimeOptions{})
	if err != nil {
		return time.Time{}, errAsOfTime
	}
	if at.After(now) {
		return time.Time{}, errAsOfFuture
	}
	return at.UTC(), nil
}

func WithAsOf(ctx context.Context, at time.Time) context.Context {
	return context.WithValue(ctx, asOfKey{}, at)
}

func clockOf(ctx context.Context) time.Time {
	if at, ok := ctx.Value(asOfKey{}).(time.Time); ok && !at.IsZero() {
		return at
	}
	return time.Now()
}

func asOfColumn(ctx context.Context) pgtype.Timestamptz {
	at, _ := ctx.Value(asOfKey{}).(time.Time)
	return pgtype.Timestamptz{Time: at, Valid: !at.IsZero()}
}
