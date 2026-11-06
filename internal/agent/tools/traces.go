package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	traceDefaultFrom      = "now-24h"
	traceListLimit        = 20
	traceStatsMaxBuckets  = 60
	traceIDStepChars      = 8
	traceSourcesMessage   = "get_trace reads Tempo and ClickHouse datasources"
	traceStatsSourcesText = "trace_stats reads Tempo and ClickHouse datasources"
	upstreamWait          = 5 * time.Second
	nanosPerMilli         = 1e6

	descGetTrace   = "Read one trace by id from a Tempo or ClickHouse datasource: its services, root span, error spans and slowest spans. Use it after listing traces, or when a log line carries a trace_id, to see where the time and the errors are. A trace over 20,000 spans is truncated to its earliest spans and says so."
	descTraceStats = "Count spans and errors per time bucket and get p50, p95 and p99 durations and a duration histogram over a TraceQL filter on Tempo, or a SQL condition over otel_traces on ClickHouse. Use it to see whether latency or errors moved before reading single traces."

	propTraceFrom    = `"from":{"type":"string","description":"Start time such as now-1h or an RFC 3339 time. Default now-24h."}`
	schemaGetTrace   = `{"type":"object","properties":{` + propDatasource + `,"trace_id":{"type":"string","description":"The trace id as 16 or 32 hex characters."},` + propTraceFrom + `,` + propTo + `},"required":["datasource","trace_id"],"additionalProperties":false}`
	propTraceFilter  = `"query":{"type":"string","description":"A TraceQL spanset filter such as { resource.service.name = \"checkout\" } on Tempo, {} for every span; a SQL boolean expression over otel_traces on ClickHouse, empty for every span."}`
	schemaTraceStats = `{"type":"object","properties":{` + propDatasource + `,` + propTraceFilter + `,` + propFrom + `,` + propTo + `,"buckets":{"type":"integer","minimum":1,"maximum":60,"description":"The most time buckets wanted. Default 60."}},"required":["datasource","query"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).traceEntries) }

func (r *Registry) traceEntries() []entry {
	return []entry{
		{Tool{Name: nameGetTrace, Description: descGetTrace, InputSchema: json.RawMessage(schemaGetTrace), ReadOnly: true, MinRole: auth.RoleEditor}, r.getTrace},
		{Tool{Name: nameTraceStats, Description: descTraceStats, InputSchema: json.RawMessage(schemaTraceStats), ReadOnly: true, MinRole: auth.RoleEditor}, r.traceStats},
	}
}

type traceOut struct {
	TraceID       string        `json:"trace_id"`
	Spans         int           `json:"spans"`
	SpansReturned int           `json:"spans_returned"`
	Truncated     bool          `json:"truncated"`
	Services      []serviceOut  `json:"services"`
	DurationMS    float64       `json:"duration_ms"`
	Root          *spanRefOut   `json:"root"`
	Errors        []spanRefOut  `json:"errors"`
	Slowest       []slowSpanOut `json:"slowest"`
}

type serviceOut struct {
	Name   string `json:"name"`
	Spans  int    `json:"spans"`
	Errors int    `json:"errors"`
}

type spanRefOut struct {
	SpanID        string `json:"span_id"`
	Name          string `json:"name"`
	Service       string `json:"service"`
	StatusMessage string `json:"status_message,omitempty"`
}

type slowSpanOut struct {
	SpanID     string  `json:"span_id"`
	Name       string  `json:"name"`
	Service    string  `json:"service"`
	DurationMS float64 `json:"duration_ms"`
	Depth      int     `json:"depth"`
}

type traceSpan struct {
	id, parent, name, service, status string
	start                             time.Time
	duration                          float64
}

func (r *Registry) getTrace(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource string `json:"datasource"`
		TraceID    string `json:"trace_id"`
		From       string `json:"from"`
		To         string `json:"to"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if !datasource.ValidTraceID(in.TraceID) {
		return invalid("trace_id must be 16 or 32 hex characters"), nil
	}
	source, ok := inst.Datasource.(datasource.TraceSource)
	if !ok {
		return invalid("%s", traceSourcesMessage), nil
	}
	rng, res, ok := resolveRange(cmp.Or(in.From, traceDefaultFrom), in.To, time.Now())
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	step := "Read trace " + in.TraceID[:traceIDStepChars] + " on " + inst.UID
	release, err := r.acquireUpstream(ctx)
	if err != nil {
		return datasourceFailure(step, err)
	}
	defer release()
	f, err := source.Trace(ctx, in.TraceID, rng, datasource.TraceSpanBudget)
	if err != nil {
		return datasourceFailure(step, err)
	}
	out := summarizeTrace(in.TraceID, f)
	return success(fmt.Sprintf("%s: %s, %s, %s", step, plural(out.Spans, "span", "spans"), plural(len(out.Services), "service", "services"), plural(len(out.Errors), "error", "errors")), out), nil
}

func (r *Registry) acquireUpstream(ctx context.Context) (release func(), err error) {
	waitCtx, cancel := context.WithTimeout(ctx, upstreamWait)
	defer cancel()
	release, err = r.deps.Runner.AcquireUpstream(waitCtx)
	switch {
	case err == nil:
		return release, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	}
	return nil, queryproxy.ErrUpstreamBusy
}

func summarizeTrace(id string, f datasource.Frame) traceOut {
	spans := spansOf(f)
	out := traceOut{TraceID: id, Spans: len(spans), SpansReturned: len(spans), Services: []serviceOut{}, Errors: []spanRefOut{}, Slowest: []slowSpanOut{}}
	if f.Meta != nil && f.Meta.Truncated {
		out.Truncated = true
		out.Spans = max(f.Meta.RowsTotal, len(spans))
	}
	if len(spans) == 0 {
		return out
	}
	byID := make(map[string]*traceSpan, len(spans))
	services := map[string]*serviceOut{}
	var root *traceSpan
	start, end := spans[0].start, spans[0].start
	for i := range spans {
		s := &spans[i]
		byID[s.id] = s
		svc, ok := services[s.service]
		if !ok {
			svc = &serviceOut{Name: s.service}
			services[s.service] = svc
		}
		svc.Spans++
		if datasource.IsErrorStatus(s.status) {
			svc.Errors++
			if len(out.Errors) < traceListLimit {
				out.Errors = append(out.Errors, spanRefOut{SpanID: s.id, Name: s.name, Service: s.service})
			}
		}
		if s.parent == "" && (root == nil || s.start.Before(root.start)) {
			root = s
		}
		start = minTime(start, s.start)
		end = maxTime(end, s.start.Add(time.Duration(s.duration)))
	}
	if root == nil {
		root = &spans[0]
	}
	out.Root = &spanRefOut{SpanID: root.id, Name: root.name, Service: root.service}
	out.DurationMS = millisOf(float64(end.Sub(start)))
	for _, svc := range services {
		out.Services = append(out.Services, *svc)
	}
	slices.SortFunc(out.Services, func(a, b serviceOut) int {
		return cmp.Or(cmp.Compare(b.Spans, a.Spans), cmp.Compare(a.Name, b.Name))
	})
	slowest := slices.Clone(spans)
	slices.SortStableFunc(slowest, func(a, b traceSpan) int { return cmp.Compare(b.duration, a.duration) })
	for _, s := range slowest[:min(len(slowest), traceListLimit)] {
		out.Slowest = append(out.Slowest, slowSpanOut{SpanID: s.id, Name: s.name, Service: s.service, DurationMS: millisOf(s.duration), Depth: depthOf(s, byID)})
	}
	return out
}

func spansOf(f datasource.Frame) []traceSpan {
	column := func(name string) []any {
		for _, field := range f.Fields {
			if field.Name == name {
				return field.Values
			}
		}
		return nil
	}
	ids, parents, names, services := column("SpanId"), column("ParentSpanId"), column("SpanName"), column("ServiceName")
	starts, durations, statuses := column("StartTime"), column("Duration"), column("StatusCode")
	spans := make([]traceSpan, 0, len(ids))
	for i := range ids {
		s := traceSpan{id: stringAt(ids, i), parent: stringAt(parents, i), name: stringAt(names, i), service: stringAt(services, i), status: stringAt(statuses, i), duration: numberAt(durations, i)}
		if at, ok := valueAt(starts, i).(time.Time); ok {
			s.start = at
		}
		spans = append(spans, s)
	}
	slices.SortStableFunc(spans, func(a, b traceSpan) int {
		return cmp.Or(a.start.Compare(b.start), cmp.Compare(a.id, b.id))
	})
	return spans
}

func depthOf(s traceSpan, byID map[string]*traceSpan) int {
	depth := 0
	seen := map[string]bool{s.id: true}
	for parent := s.parent; parent != "" && !seen[parent]; depth++ {
		seen[parent] = true
		p, ok := byID[parent]
		if !ok {
			return depth + 1
		}
		parent = p.parent
	}
	return depth
}

func valueAt(values []any, i int) any {
	if i < len(values) {
		return values[i]
	}
	return nil
}

func stringAt(values []any, i int) string {
	s, _ := valueAt(values, i).(string)
	return s
}

func numberAt(values []any, i int) float64 {
	switch v := valueAt(values, i).(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	case uint64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

func millisOf(nanos float64) float64 {
	return nanos / nanosPerMilli
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

type traceStatsOut struct {
	StepMS    int64               `json:"step_ms"`
	Partial   bool                `json:"partial"`
	Spans     int64               `json:"spans"`
	Errors    int64               `json:"errors"`
	Buckets   []traceBucketOut    `json:"buckets"`
	Histogram []durationBucketOut `json:"histogram"`
}

type traceBucketOut struct {
	Start  string   `json:"start"`
	Spans  int64    `json:"spans"`
	Errors int64    `json:"errors"`
	P50MS  *float64 `json:"p50_ms"`
	P95MS  *float64 `json:"p95_ms"`
	P99MS  *float64 `json:"p99_ms"`
}

type durationBucketOut struct {
	LeMS  float64 `json:"le_ms"`
	Count int64   `json:"count"`
}

func (r *Registry) traceStats(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource string  `json:"datasource"`
		Query      *string `json:"query"`
		From       string  `json:"from"`
		To         string  `json:"to"`
		Buckets    *int    `json:"buckets"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if in.Query == nil {
		return invalid("query is required"), nil
	}
	if len(*in.Query) > maxQueryBytes {
		return invalid("query must be at most %d bytes", maxQueryBytes), nil
	}
	source, ok := inst.Datasource.(datasource.TraceStatsSource)
	if !ok {
		return invalid("%s", traceStatsSourcesText), nil
	}
	buckets, res, ok := boundedInt(in.Buckets, traceStatsMaxBuckets, 1, traceStatsMaxBuckets, "buckets")
	if !ok {
		return res, nil
	}
	rng, res, ok := resolveRange(in.From, in.To, time.Now())
	if !ok {
		return res, nil
	}
	step := queryproxy.StepFor(rng, buckets)
	rng = datasource.TimeRange{From: queryproxy.Floor(rng.From, step), To: queryproxy.Floor(rng.To, step)}
	if !rng.To.After(rng.From) {
		rng.To = rng.From.Add(step)
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	stepName := "Read trace stats on " + inst.UID
	release, err := r.acquireUpstream(ctx)
	if err != nil {
		return datasourceFailure(stepName, err)
	}
	defer release()
	stats, err := source.TraceStats(ctx, datasource.TraceStatsRequest{Query: *in.Query, Range: rng, Step: step, Limits: queryproxy.DraftLimits, User: queryproxy.UserFor(principal)})
	if err != nil {
		return datasourceFailure(stepName, err)
	}
	out := newTraceStatsOut(stats)
	return success(fmt.Sprintf("%s: %s, p95 %s", stepName, plural(int(out.Spans), "span", "spans"), describeP95(stats)), out), nil
}

func newTraceStatsOut(s datasource.TraceStats) traceStatsOut {
	out := traceStatsOut{StepMS: s.Step.Milliseconds(), Partial: s.Partial, Spans: s.Spans, Errors: s.Errors, Buckets: make([]traceBucketOut, len(s.Buckets)), Histogram: make([]durationBucketOut, len(s.Histogram))}
	for i, b := range s.Buckets {
		out.Buckets[i] = traceBucketOut{Start: b.Start.UTC().Format(time.RFC3339), Spans: b.Spans, Errors: b.Errors, P50MS: b.P50, P95MS: b.P95, P99MS: b.P99}
	}
	for i, b := range s.Histogram {
		out.Histogram[i] = durationBucketOut{LeMS: b.LeMS, Count: b.Count}
	}
	return out
}

func describeP95(s datasource.TraceStats) string {
	var worst *float64
	for _, b := range s.Buckets {
		if b.P95 != nil && (worst == nil || *b.P95 > *worst) {
			worst = b.P95
		}
	}
	if worst == nil {
		return "n/a"
	}
	return strconv.FormatFloat(*worst, 'f', 1, 64) + " ms"
}
