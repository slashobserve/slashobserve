package datasource

import (
	"context"
	"regexp"
)

const MaxTraceLogsServiceBytes = 200

var (
	traceIDPattern = regexp.MustCompile(`^([0-9a-fA-F]{16}|[0-9a-fA-F]{32})$`)
	spanIDPattern  = regexp.MustCompile(`^[0-9a-fA-F]{16}$`)
)

func ValidTraceID(id string) bool { return traceIDPattern.MatchString(id) }

func ValidSpanID(id string) bool { return spanIDPattern.MatchString(id) }

type TraceLogsRequest struct {
	TraceID, SpanID, Service string
	Range                    TimeRange
	Limit                    int
	User                     QueryUser
}

func (r TraceLogsRequest) Validate() error {
	switch {
	case !ValidTraceID(r.TraceID):
		return Errorf(ErrInvalidQuery, "trace_id must be 16 or 32 hex characters")
	case r.SpanID != "" && !ValidSpanID(r.SpanID):
		return Errorf(ErrInvalidQuery, "span_id must be 16 hex characters")
	case len(r.Service) > MaxTraceLogsServiceBytes:
		return Errorf(ErrInvalidQuery, "service must be at most %d bytes", MaxTraceLogsServiceBytes)
	case r.Limit < 1:
		return Errorf(ErrInvalidQuery, "limit must be at least 1")
	}
	return r.Range.Validate()
}

type TraceLogSource interface {
	TraceLogs(ctx context.Context, req TraceLogsRequest) ([]Frame, error)
}
