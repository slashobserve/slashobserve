package datasource

import (
	"context"
	"time"
)

type PatternRequest struct {
	Query Query
	Range TimeRange
	Step  time.Duration
}

type PatternSample struct {
	At    time.Time
	Count int64
}

type NativePattern struct {
	Pattern string
	Samples []PatternSample
}

type LogPatternSource interface {
	LogPatterns(ctx context.Context, req PatternRequest) ([]NativePattern, error)
}
