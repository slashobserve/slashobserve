package datasource

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const (
	TraceSpanBudget = 20000

	traceIDHexLen    = 32
	shortTraceHexLen = 16
)

type TraceSource interface {
	Trace(ctx context.Context, id string, r TimeRange, maxSpans int) (Frame, error)
}

type TraceStatsRequest struct {
	Query  string
	Range  TimeRange
	Step   time.Duration
	Limits Limits
	User   QueryUser
}

type TraceBucket struct {
	Start         time.Time
	Spans, Errors int64
	P50, P95, P99 *float64
}

type DurationBucket struct {
	LeMS  float64
	Count int64
}

type TraceStats struct {
	Step      time.Duration
	Partial   bool
	Spans     int64
	Errors    int64
	Buckets   []TraceBucket
	Histogram []DurationBucket
}

type TraceStatsSource interface {
	TraceStats(ctx context.Context, req TraceStatsRequest) (TraceStats, error)
}

type SpanEvent struct {
	TimeMS     int64          `json:"time_ms"`
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes"`
}

func IsTraceID(s string) bool {
	if len(s) != traceIDHexLen && len(s) != shortTraceHexLen {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func SpanKindName(raw string) string {
	switch strings.ToUpper(strings.TrimPrefix(raw, "SPAN_KIND_")) {
	case "1", "INTERNAL":
		return "Internal"
	case "2", "SERVER":
		return "Server"
	case "3", "CLIENT":
		return "Client"
	case "4", "PRODUCER":
		return "Producer"
	case "5", "CONSUMER":
		return "Consumer"
	}
	return "Unspecified"
}

func IsErrorStatus(code string) bool {
	switch strings.ToLower(code) {
	case "error", "status_code_error", "2":
		return true
	}
	return false
}

func SpanEventsJSON(events []SpanEvent) (string, error) {
	if len(events) == 0 {
		return "[]", nil
	}
	for i := range events {
		if events[i].Attributes == nil {
			events[i].Attributes = map[string]any{}
		}
	}
	b, err := json.Marshal(events)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
