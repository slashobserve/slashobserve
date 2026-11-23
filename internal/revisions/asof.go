package revisions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type asOfKey struct{}

var (
	errAsOfTime   = errors.New("as_of must be epoch milliseconds or an RFC 3339 time")
	errAsOfFuture = errors.New("as_of must not be in the future")
)

func mayRunAsOf(by auth.Principal) error {
	if !by.Role.AtLeast(auth.RoleAdmin) {
		return ErrForbidden
	}
	return nil
}

func asOfBars(asOf pgtype.Timestamptz, by auth.Principal) bool {
	return asOf.Valid && mayRunAsOf(by) != nil
}

func keepAsOf(row dbsqlc.AgentRequest, by auth.Principal) error {
	if asOfBars(row.AsOf, by) {
		return ErrForbidden
	}
	return nil
}

func CheckAsOf(raw string, by auth.Principal, now time.Time) (time.Time, error) {
	if err := mayRunAsOf(by); err != nil {
		return time.Time{}, err
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

func asOfOf(ctx context.Context) (time.Time, bool) {
	at, ok := ctx.Value(asOfKey{}).(time.Time)
	return at, ok && !at.IsZero()
}

func clockOf(ctx context.Context) time.Time {
	if at, ok := asOfOf(ctx); ok {
		return at
	}
	return time.Now()
}

func asOfColumn(ctx context.Context) pgtype.Timestamptz {
	at, ok := asOfOf(ctx)
	return pgtype.Timestamptz{Time: at, Valid: ok}
}
