package datasource

import (
	"context"
	"time"
)

type unavailable struct {
	kind string
	err  error
}

func NewUnavailable(kind string, err error) Datasource { return unavailable{kind: kind, err: err} }

func (u unavailable) Type() string { return u.kind }

func (u unavailable) CheckHealth(context.Context) error { return u.err }

func (u unavailable) Discover(context.Context, DiscoverRequest) (*Catalog, error) {
	return nil, u.err
}

func (u unavailable) Lint(context.Context, string) ([]Diagnostic, error) { return nil, u.err }

func (u unavailable) VariableValues(context.Context, Query, TimeRange) ([]string, error) {
	return nil, u.err
}

func (u unavailable) QueryRange(context.Context, Query, TimeRange, time.Duration) ([]Frame, error) {
	return nil, u.err
}

func (u unavailable) QueryInstant(context.Context, Query, TimeRange) ([]Frame, error) {
	return nil, u.err
}

func (u unavailable) Close() error { return nil }
