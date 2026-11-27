package triage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

type Series struct {
	Labels map[string]string
	Points []Point
}

type TraceStatsInput struct {
	Query   string
	GroupBy string
	Range   Window
	Buckets int
}

type TraceGroup struct {
	Value         string
	Spans, Errors int64
	P95MS         *float64
}

type TraceBucket struct {
	Start         time.Time
	Spans, Errors int64
	P95MS         *float64
}

type TraceStats struct {
	Groups  []TraceGroup
	Buckets []TraceBucket
}

type PatternTrend struct {
	Text  string
	Level string
	Trend []int64
}

type Reader interface {
	Services(ctx context.Context, uid, signal string) (field string, names []string, err error)
	MetricNames(ctx context.Context, uid, search string) ([]string, error)
	Range(ctx context.Context, uid, query string, rng Window, points int) ([]Series, error)
	TraceStats(ctx context.Context, uid string, in TraceStatsInput) (TraceStats, error)
	ErrorOrigins(ctx context.Context, uid string, rng Window) ([]Origin, error)
	Patterns(ctx context.Context, uid, query string, rng Window) ([]PatternTrend, error)
	Annotations(ctx context.Context, rng Window) ([]Event, error)
}

type Tools interface {
	Call(ctx context.Context, name string, input json.RawMessage) (tools.Result, error)
	SweepRange(ctx context.Context, uid, query string, rng datasource.TimeRange, points int) ([]datasource.Frame, error)
}

type ToolReader struct{ Tools Tools }

type ToolError struct{ Code, Message string }

func (e *ToolError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

var errUnavailable = errors.New("the store did not answer")

func (t ToolReader) call(ctx context.Context, name string, in, out any) error {
	input, err := json.Marshal(in)
	if err != nil {
		return err
	}
	res, err := t.Tools.Call(ctx, name, input)
	if err != nil {
		return err
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		return err
	}
	if res.IsError {
		var e ToolError
		var fields struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &fields) == nil {
			e.Code, e.Message = fields.Code, fields.Message
		}
		if e.Code == "" {
			e.Code = "tool_error"
		}
		return &e
	}
	return json.Unmarshal(body, out)
}

func stampOf(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func (t ToolReader) Services(ctx context.Context, uid, signal string) (string, []string, error) {
	var out struct {
		Sources []struct {
			Field       string   `json:"field"`
			Services    []string `json:"services"`
			Unavailable string   `json:"unavailable"`
		} `json:"sources"`
	}
	if err := t.call(ctx, "list_services", map[string]string{"datasource": uid, "signal": signal, "window": "6h"}, &out); err != nil {
		return "", nil, err
	}
	if len(out.Sources) == 0 {
		return "", nil, nil
	}
	s := out.Sources[0]
	if s.Unavailable != "" {
		return "", nil, fmt.Errorf("%w: %s", errUnavailable, s.Unavailable)
	}
	return s.Field, s.Services, nil
}

func (t ToolReader) MetricNames(ctx context.Context, uid, search string) ([]string, error) {
	var out struct {
		Metrics []struct {
			Name string `json:"name"`
		} `json:"metrics"`
	}
	if err := t.call(ctx, "list_metrics", map[string]any{"datasource": uid, "search": search, "limit": 200}, &out); err != nil {
		return nil, err
	}
	names := make([]string, len(out.Metrics))
	for i, m := range out.Metrics {
		names[i] = m.Name
	}
	return names, nil
}

func (t ToolReader) Range(ctx context.Context, uid, query string, rng Window, points int) ([]Series, error) {
	frames, err := t.Tools.SweepRange(ctx, uid, query, datasource.TimeRange{From: rng.From, To: rng.To}, points)
	if err != nil {
		return nil, err
	}
	return seriesOf(frames), nil
}

func seriesOf(frames []datasource.Frame) []Series {
	var out []Series
	for _, f := range frames {
		if !f.IsSeries() {
			continue
		}
		times := f.Fields[0].Values
		for _, field := range f.Fields[1:] {
			s := Series{Labels: f.Labels}
			for i, v := range field.Values {
				at, ok := times[i].(time.Time)
				x, isNum := v.(float64)
				if ok && isNum {
					s.Points = append(s.Points, Point{At: at, Value: x})
				}
			}
			out = append(out, s)
		}
	}
	return out
}

func (t ToolReader) TraceStats(ctx context.Context, uid string, in TraceStatsInput) (TraceStats, error) {
	var out struct {
		Buckets []struct {
			Start  string   `json:"start"`
			Spans  int64    `json:"spans"`
			Errors int64    `json:"errors"`
			P95MS  *float64 `json:"p95_ms"`
		} `json:"buckets"`
		Groups []struct {
			Value  string   `json:"value"`
			Spans  int64    `json:"spans"`
			Errors int64    `json:"errors"`
			P95MS  *float64 `json:"p95_ms"`
		} `json:"groups"`
	}
	input := map[string]any{"datasource": uid, "query": in.Query, "from": stampOf(in.Range.From), "to": stampOf(in.Range.To)}
	if in.Buckets > 0 {
		input["buckets"] = in.Buckets
	}
	if in.GroupBy != "" {
		input["group_by"] = in.GroupBy
	}
	if err := t.call(ctx, "trace_stats", input, &out); err != nil {
		return TraceStats{}, err
	}
	var stats TraceStats
	for _, b := range out.Buckets {
		start, err := time.Parse(time.RFC3339, b.Start)
		if err != nil {
			continue
		}
		stats.Buckets = append(stats.Buckets, TraceBucket{Start: start, Spans: b.Spans, Errors: b.Errors, P95MS: b.P95MS})
	}
	for _, g := range out.Groups {
		stats.Groups = append(stats.Groups, TraceGroup{Value: g.Value, Spans: g.Spans, Errors: g.Errors, P95MS: g.P95MS})
	}
	return stats, nil
}

func (t ToolReader) ErrorOrigins(ctx context.Context, uid string, rng Window) ([]Origin, error) {
	var out struct {
		Groups []struct {
			Service   string  `json:"service"`
			Operation string  `json:"operation"`
			Message   string  `json:"status_message"`
			Traces    int     `json:"traces"`
			Share     float64 `json:"share"`
			Example   string  `json:"example_trace_id"`
			RelayedBy []struct {
				Service string `json:"service"`
			} `json:"relayed_by"`
		} `json:"groups"`
	}
	if err := t.call(ctx, "trace_error_origins", map[string]string{"datasource": uid, "from": stampOf(rng.From), "to": stampOf(rng.To)}, &out); err != nil {
		return nil, err
	}
	origins := make([]Origin, len(out.Groups))
	for i, g := range out.Groups {
		o := Origin{Source: uid, Service: g.Service, Operation: g.Operation, Message: g.Message, Traces: g.Traces, Share: g.Share, Example: g.Example}
		for _, r := range g.RelayedBy {
			o.RelayedBy = append(o.RelayedBy, r.Service)
		}
		origins[i] = o
	}
	return origins, nil
}

func (t ToolReader) Patterns(ctx context.Context, uid, query string, rng Window) ([]PatternTrend, error) {
	var out struct {
		Patterns []struct {
			Pattern string  `json:"pattern"`
			Level   string  `json:"level"`
			Trend   []int64 `json:"trend"`
		} `json:"patterns"`
	}
	if err := t.call(ctx, "log_patterns", map[string]any{"datasource": uid, "query": query, "from": stampOf(rng.From), "to": stampOf(rng.To), "limit": 30}, &out); err != nil {
		return nil, err
	}
	patterns := make([]PatternTrend, len(out.Patterns))
	for i, p := range out.Patterns {
		patterns[i] = PatternTrend{Text: p.Pattern, Level: p.Level, Trend: p.Trend}
	}
	return patterns, nil
}

func (t ToolReader) Annotations(ctx context.Context, rng Window) ([]Event, error) {
	var out struct {
		Annotations []struct {
			TimeFrom string   `json:"time_from"`
			Text     string   `json:"text"`
			Tags     []string `json:"tags"`
		} `json:"annotations"`
	}
	if err := t.call(ctx, "list_annotations", map[string]any{"from": stampOf(rng.From), "to": stampOf(rng.To), "limit": 100}, &out); err != nil {
		return nil, err
	}
	var events []Event
	for _, a := range out.Annotations {
		at, err := time.Parse(time.RFC3339, a.TimeFrom)
		if err != nil {
			continue
		}
		text := a.Text
		if len(a.Tags) > 0 {
			text = fmt.Sprintf("%v %s", a.Tags, text)
		}
		events = append(events, Event{Kind: EventAnnotation, At: at, Text: text})
	}
	return events, nil
}
