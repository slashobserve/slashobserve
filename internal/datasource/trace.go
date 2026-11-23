package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	TraceSpanBudget    = 20000
	CompareSpanBudget  = 2000
	MaxCompareSelected = 50
	MaxCompareBaseline = 150
)

type TraceSource interface {
	Trace(ctx context.Context, id string, r TimeRange, maxSpans int) (Frame, error)
}

type TraceAttribute struct{ Key, Value string }

type TraceAttributeSet struct {
	ID         string
	Attributes []TraceAttribute
}

type TraceAttributesRequest struct {
	IDs      []string
	Range    TimeRange
	MaxSpans int
}

type TraceAttributeSource interface {
	TraceAttributes(ctx context.Context, req TraceAttributesRequest) ([]TraceAttributeSet, error)
}

func CompareTraceIDs(selected, baseline []string) (sel, base []string, err error) {
	if sel, err = ValidateTraceIDs("selected", selected, MaxCompareSelected); err != nil {
		return nil, nil, err
	}
	if base, err = ValidateTraceIDs("baseline", baseline, MaxCompareBaseline); err != nil {
		return nil, nil, err
	}
	for _, id := range base {
		if slices.Contains(sel, id) {
			return nil, nil, errors.New("a trace id cannot be in both selected and baseline")
		}
	}
	return sel, base, nil
}

func ValidateTraceIDs(field string, ids []string, maxIDs int) ([]string, error) {
	if len(ids) < 1 || len(ids) > maxIDs {
		return nil, fmt.Errorf("%s must hold from 1 to %d trace ids", field, maxIDs)
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !ValidTraceID(id) {
			return nil, fmt.Errorf("%s must hold trace ids of 16 or 32 hex characters", field)
		}
		if id = strings.ToLower(id); !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
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

func SpanStatusName(raw string) string {
	switch strings.ToUpper(strings.TrimPrefix(raw, "STATUS_CODE_")) {
	case "1", "OK":
		return "Ok"
	case "2", "ERROR":
		return "Error"
	}
	return "Unset"
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
	return jsonText(events)
}

type SpanLink struct {
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	Attributes map[string]any `json:"attributes"`
}

func SpanLinksJSON(links []SpanLink) (string, error) {
	if len(links) == 0 {
		return "[]", nil
	}
	for i := range links {
		if links[i].Attributes == nil {
			links[i].Attributes = map[string]any{}
		}
	}
	return jsonText(links)
}

func jsonText(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
