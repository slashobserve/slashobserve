package datasource

import (
	"context"
	"time"
)

const TraceListBudget = 2000

type TraceFilter struct {
	Service   string
	Route     string
	Operation string
}

type TraceListRequest struct {
	Filter      TraceFilter
	Range       TimeRange
	Errors      bool
	MinDuration time.Duration
	Limit       int
	Limits      Limits
}

type TraceSummary struct {
	ID          string
	RootService string
	Route       string
	Start       time.Time
	Duration    time.Duration
}

type TraceList struct {
	Traces   []TraceSummary
	Complete bool
}

type TraceListSource interface {
	ListTraces(ctx context.Context, req TraceListRequest) (TraceList, error)
}
