package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
	traceSourcesMessage   = "get_trace reads Tempo, Jaeger and ClickHouse datasources, and Elastic or OpenSearch datasources with spans: jaeger"
	traceStatsSourcesText = "trace_stats reads Tempo and ClickHouse datasources, and Elastic or OpenSearch datasources with spans: jaeger"
	nanosPerMilli         = 1e6
	minHTTPStatus         = 100
	maxHTTPStatus         = 599
	minServerErrorStatus  = 500
	traceRelayLimit       = 5

	approximateQuantilesNote = "Tempo quantiles are interpolated from its log-scale duration buckets, so treat them as approximate. The histogram lists bucket upper bounds, not quantiles."
	bucketQuantilesNote      = "Per-bucket quantiles are interpolated from Tempo's log-scale duration buckets, so they are approximate, and the histogram lists bucket upper bounds, not quantiles."
	quantilesExact           = "exact"
	quantilesSample          = "sample"
	quantilesApproximate     = "approximate"

	descGetTrace   = "Read one trace by id from a Tempo, Jaeger, ClickHouse or spans: jaeger datasource: its services, root span, error spans with the message each set and the HTTP response status, their parent and depth with each error that has no error below it marked origin (only on a complete trace), and slowest spans with their self time (duration less the children's). Relays lists server spans with an HTTP status of 500 or more above a failing span: they passed the failure upstream and are not counted in errors. Use it after listing traces, or when a log line carries a trace_id, to see where the time and the errors are. A trace over 20,000 spans is truncated to its earliest spans and says so. Spans that link to other traces are counted and their trace ids listed. The origin flag marks where an error chain starts inside this one trace; it does not by itself say which service an incident started in, because a failing dependency is the deepest error of every trace that calls it. To name the service an incident began in, compare per-service error counts or shares over the whole window across many traces, with trace_stats filtered by service or a metrics read. Span durations, and the trace duration of a trace that is not truncated, are exact, in milliseconds with sub-millisecond precision; a Tempo trace listing shows durations rounded to whole milliseconds, so take any duration you state for a trace from here. When the result says the trace was truncated, state no total duration from it."
	descTraceStats = "Count spans and errors per time bucket and get p50, p95 and p99 durations and a duration histogram over a filter: TraceQL on Tempo, a SQL condition over otel_traces on ClickHouse such as ServiceName = 'checkout' AND StatusCode = 'Error' (StatusCode is Unset, Ok or Error), or KQL over the span documents of spans: jaeger such as process.serviceName:checkout. Also gives them over the whole range (window) and, with group_by, per value of one attribute (top 20 by spans). Window and group quantiles are exact on ClickHouse, and elsewhere when every matching span is read (up to 20,000); else quantiles says sample and quantile_spans how many, and you state it as a sample. Per-bucket quantiles outside ClickHouse and the histogram's bucket bounds are approximate: never report one as a window quantile. Use it for a per-service or per-route quantile, or to see if latency or errors moved, before single traces."

	propTraceFrom    = `"from":{"type":"string","description":"Start time such as now-1h or an RFC 3339 time. Default now-24h."}`
	schemaGetTrace   = `{"type":"object","properties":{` + propDatasource + `,"trace_id":{"type":"string","description":"The trace id as 16 or 32 hex characters."},` + propTraceFrom + `,` + propTo + `},"required":["datasource","trace_id"],"additionalProperties":false}`
	propTraceFilter  = `"query":{"type":"string","description":"A TraceQL spanset filter such as { resource.service.name = \"checkout\" } on Tempo, {} for every span; a SQL condition over otel_traces on ClickHouse such as Duration > 1000000, not a SELECT; KQL on spans: jaeger, duration in microseconds; empty for every span."}`
	schemaTraceStats = `{"type":"object","properties":{` + propDatasource + `,` + propTraceFilter + `,` + propFrom + `,` + propTo + `,"buckets":{"type":"integer","minimum":1,"maximum":60,"description":"The most time buckets wanted. Default 60."},"group_by":{"type":"string","maxLength":128,"description":"One attribute to group counts, errors and quantiles by over the whole range: a TraceQL attribute such as resource.service.name, span.http.route or .http.method, or name, status or kind, which every store maps to its own fields."}},"required":["datasource","query"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).traceEntries) }

func (r *Registry) traceEntries() []entry {
	return []entry{
		{Tool{Name: nameGetTrace, Description: descGetTrace, InputSchema: json.RawMessage(schemaGetTrace), ReadOnly: true, MinRole: auth.RoleViewer}, r.getTrace},
		{Tool{Name: nameTraceStats, Description: descTraceStats, InputSchema: json.RawMessage(schemaTraceStats), ReadOnly: true, MinRole: auth.RoleViewer}, r.traceStats},
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
	Relays        []relayOut    `json:"relays,omitempty"`
	Slowest       []slowSpanOut `json:"slowest"`
	Links         int           `json:"links"`
	LinkedTraces  []string      `json:"linked_traces,omitempty"`
}

type serviceOut struct {
	Name   string `json:"name"`
	Spans  int    `json:"spans"`
	Errors int    `json:"errors"`
}

type spanRefOut struct {
	SpanID        string `json:"span_id"`
	ParentSpanID  string `json:"parent_span_id,omitempty"`
	Name          string `json:"name"`
	Service       string `json:"service"`
	Depth         int    `json:"depth"`
	Origin        bool   `json:"origin,omitempty"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	StatusMessage string `json:"status_message,omitempty"`
}

type relayOut struct {
	Service    string `json:"service"`
	Name       string `json:"name"`
	HTTPStatus int    `json:"http_status"`
}

type slowSpanOut struct {
	SpanID     string  `json:"span_id"`
	Name       string  `json:"name"`
	Service    string  `json:"service"`
	DurationMS float64 `json:"duration_ms"`
	SelfTimeMS float64 `json:"self_time_ms"`
	Depth      int     `json:"depth"`
	HTTPStatus int     `json:"http_status,omitempty"`
}

type traceSpan struct {
	id, parent, name, service, status string
	statusMessage, kind               string
	httpStatus                        int
	start                             time.Time
	duration                          float64
	links                             []datasource.SpanLink
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
	inst, res, ok := r.instance(ctx, in.Datasource)
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
	rng, res, ok := resolveRunRange(ctx, cmp.Or(in.From, traceDefaultFrom), in.To)
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	step := "Read trace " + in.TraceID[:traceIDStepChars] + " on " + inst.UID
	release, err := r.acquireUpstream(ctx, inst.UID)
	if err != nil {
		return datasourceFailure(step, err)
	}
	defer release()
	f, err := source.Trace(ctx, in.TraceID, rng, datasource.TraceSpanBudget)
	datasource.NoteQuery(ctx, inst.UID, "trace "+in.TraceID, rng, f.Rows())
	if err != nil {
		return datasourceFailure(step, err)
	}
	out := summarizeTrace(in.TraceID, f)
	return success(fmt.Sprintf("%s: %s, %s, %s", step, plural(out.Spans, "span", "spans"), plural(len(out.Services), "service", "services"), plural(len(out.Errors), "error", "errors")), out), nil
}

func (r *Registry) acquireUpstream(ctx context.Context, datasourceUID string) (release func(), err error) {
	waitCtx, cancel := context.WithTimeout(ctx, r.deps.Runner.UpstreamWait())
	defer cancel()
	release, err = r.deps.Runner.AcquireUpstream(waitCtx, datasourceUID)
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
	out.Links, out.LinkedTraces = linkSummary(spans)
	byID := make(map[string]*traceSpan, len(spans))
	services := map[string]*serviceOut{}
	var root *traceSpan
	var failed []*traceSpan
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
			failed = append(failed, s)
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
	out.Root = &spanRefOut{SpanID: root.id, Name: root.name, Service: root.service, HTTPStatus: root.httpStatus}
	out.Errors = errorRefs(failed, byID, !out.Truncated)
	out.Relays = relayRefs(spans, ancestorsOf(failed, byID))
	out.DurationMS = millisOf(float64(end.Sub(start)))
	for _, svc := range services {
		out.Services = append(out.Services, *svc)
	}
	slices.SortFunc(out.Services, func(a, b serviceOut) int {
		return cmp.Or(cmp.Compare(b.Spans, a.Spans), cmp.Compare(a.Name, b.Name))
	})
	slowest := slices.Clone(spans)
	slices.SortStableFunc(slowest, func(a, b traceSpan) int { return cmp.Compare(b.duration, a.duration) })
	children := childrenOf(spans)
	for _, s := range slowest[:min(len(slowest), traceListLimit)] {
		out.Slowest = append(out.Slowest, slowSpanOut{SpanID: s.id, Name: s.name, Service: s.service, DurationMS: millisOf(s.duration), SelfTimeMS: millisOf(selfTime(s, children[s.id])), Depth: depthOf(s, byID), HTTPStatus: s.httpStatus})
	}
	return out
}

func relayRefs(spans []traceSpan, aboveFailure map[string]bool) []relayOut {
	var relays []relayOut
	for _, s := range spans {
		relay := aboveFailure[s.id] && datasource.SpanKindName(s.kind) == "Server" && !datasource.IsErrorStatus(s.status) &&
			datasource.SpanStatusName(s.status) != "Ok" && s.httpStatus >= minServerErrorStatus && s.httpStatus <= maxHTTPStatus
		if relay && len(relays) < traceRelayLimit {
			relays = append(relays, relayOut{Service: s.service, Name: s.name, HTTPStatus: s.httpStatus})
		}
	}
	return relays
}

func errorRefs(failed []*traceSpan, byID map[string]*traceSpan, complete bool) []spanRefOut {
	origins := errorOriginIDs(failed, byID)
	origin := func(s *traceSpan) bool { return complete && origins[s.id] }
	listed := make([]*traceSpan, 0, min(len(failed), traceListLimit))
	for _, wantOrigin := range []bool{true, false} {
		for _, s := range failed {
			if len(listed) < traceListLimit && origin(s) == wantOrigin {
				listed = append(listed, s)
			}
		}
	}
	slices.SortStableFunc(listed, func(a, b *traceSpan) int { return slices.Index(failed, a) - slices.Index(failed, b) })
	refs := make([]spanRefOut, 0, len(listed))
	for _, s := range listed {
		refs = append(refs, spanRefOut{SpanID: s.id, ParentSpanID: s.parent, Name: s.name, Service: s.service, Depth: depthOf(*s, byID), Origin: origin(s), HTTPStatus: s.httpStatus, StatusMessage: s.statusMessage})
	}
	return refs
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
	starts, durations, statuses, links := column("StartTime"), column("Duration"), column("StatusCode"), column("SpanLinks")
	messages, attributes, kinds := column("StatusMessage"), column("SpanAttributes"), column("SpanKind")
	spans := make([]traceSpan, 0, len(ids))
	for i := range ids {
		s := traceSpan{id: stringAt(ids, i), parent: stringAt(parents, i), name: stringAt(names, i), service: stringAt(services, i), status: stringAt(statuses, i), statusMessage: stringAt(messages, i), kind: stringAt(kinds, i), httpStatus: httpStatusOf(stringAt(attributes, i)), duration: numberAt(durations, i)}
		if at, ok := valueAt(starts, i).(time.Time); ok {
			s.start = at
		}
		s.links = linksAt(links, i)
		spans = append(spans, s)
	}
	slices.SortStableFunc(spans, func(a, b traceSpan) int {
		return cmp.Or(a.start.Compare(b.start), cmp.Compare(a.id, b.id))
	})
	return spans
}

func httpStatusOf(attributes string) int {
	if !strings.Contains(attributes, "status_code") {
		return 0
	}
	var attrs struct {
		Response json.RawMessage `json:"http.response.status_code"`
		Legacy   json.RawMessage `json:"http.status_code"`
	}
	if json.Unmarshal([]byte(attributes), &attrs) != nil {
		return 0
	}
	for _, raw := range []json.RawMessage{attrs.Response, attrs.Legacy} {
		if n, err := strconv.Atoi(strings.Trim(string(raw), `" `)); err == nil && n >= minHTTPStatus && n <= maxHTTPStatus {
			return n
		}
	}
	return 0
}

func linkSummary(spans []traceSpan) (linked int, traces []string) {
	for _, s := range spans {
		if len(s.links) > 0 {
			linked++
		}
		for _, l := range s.links {
			if len(traces) < traceListLimit && !slices.Contains(traces, l.TraceID) {
				traces = append(traces, l.TraceID)
			}
		}
	}
	return linked, traces
}

func linksAt(values []any, i int) []datasource.SpanLink {
	var links []datasource.SpanLink
	if json.Unmarshal([]byte(stringAt(values, i)), &links) != nil {
		return nil
	}
	return links
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
	Window    *traceQuantilesOut  `json:"window,omitempty"`
	GroupBy   string              `json:"group_by,omitempty"`
	Groups    []traceGroupOut     `json:"groups,omitempty"`
	Truncated bool                `json:"groups_truncated,omitempty"`
	Quantiles string              `json:"quantiles,omitempty"`
	Measured  int64               `json:"quantile_spans,omitempty"`
	Note      string              `json:"note,omitempty"`
}

type traceQuantilesOut struct {
	P50MS *float64 `json:"p50_ms"`
	P95MS *float64 `json:"p95_ms"`
	P99MS *float64 `json:"p99_ms"`
}

type traceGroupOut struct {
	Value  string `json:"value"`
	Spans  int64  `json:"spans"`
	Errors int64  `json:"errors"`
	traceQuantilesOut
	Measured *int64 `json:"quantile_spans,omitempty"`
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
		GroupBy    string  `json:"group_by"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(ctx, in.Datasource)
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
	rng, res, ok := resolveRunRange(ctx, in.From, in.To)
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
	release, err := r.acquireUpstream(ctx, inst.UID)
	if err != nil {
		return datasourceFailure(stepName, err)
	}
	defer release()
	stats, err := source.TraceStats(ctx, datasource.TraceStatsRequest{Query: *in.Query, GroupBy: in.GroupBy, Range: rng, Step: step, Limits: queryproxy.DraftLimits, User: queryproxy.UserFor(principal)})
	datasource.NoteQuery(ctx, inst.UID, *in.Query, rng, len(stats.Buckets))
	if err != nil {
		return datasourceFailure(stepName, err)
	}
	out := newTraceStatsOut(stats)
	if durations, ok := inst.Datasource.(datasource.TraceDurationSource); ok {
		req := datasource.TraceDurationRequest{Query: *in.Query, GroupBy: in.GroupBy, Range: rng, Limits: queryproxy.DraftLimits, User: queryproxy.UserFor(principal), MaxSpans: datasource.TraceDurationSpanBudget}
		d, err := durations.TraceDurations(ctx, req)
		datasource.NoteQuery(ctx, inst.UID, *in.Query, rng, len(d.Groups)+1)
		out.applyDurations(d, err, in.GroupBy, bucketNoteOf(inst.Datasource.Type(), stats.Note))
	}
	return success(fmt.Sprintf("%s: %s, p95 %s", stepName, plural(int(out.Spans), "span", "spans"), describeP95(out)), out), nil
}

func bucketNoteOf(datasourceType, storeNote string) string {
	if datasourceType == "tempo" {
		return bucketQuantilesNote
	}
	return storeNote
}

func (out *traceStatsOut) applyDurations(d datasource.TraceDurations, err error, groupBy, bucketNote string) {
	if err != nil {
		if out.Window != nil || len(out.Groups) > 0 {
			out.Note = strings.TrimSpace("The exact quantiles could not be read, so the window and group quantiles are Tempo's. " + out.Note)
		}
		return
	}
	exact := d.Exact && d.Window.Spans == out.Spans
	out.Quantiles, out.Measured, out.Window = quantilesSample, d.Window.Spans, nil
	if exact {
		out.Quantiles = quantilesExact
	}
	if d.Window.Spans > 0 {
		q := exactOut(d.Window)
		out.Window = &q
	}
	out.mergeGroups(d, groupBy, exact)
	note := fmt.Sprintf("Window and group quantiles are exact: they come from all %d matching span durations, each the value at position q times n of the sorted durations.", d.Window.Spans)
	if !exact {
		note = fmt.Sprintf("Window and group quantiles rest on %d of the %d matching spans, the newest a trace search returned within its cap, so they are a sample and not exact; say so when you state one.", d.Window.Spans, out.Spans)
	}
	if bucketNote != "" {
		note += " " + bucketNote
	}
	out.Note = note
}

func (out *traceStatsOut) mergeGroups(d datasource.TraceDurations, groupBy string, exact bool) {
	byValue := make(map[string]datasource.DurationQuantiles, len(d.Groups))
	for _, g := range d.Groups {
		byValue[g.Value] = g
	}
	measured := func(g datasource.DurationQuantiles) *int64 {
		if exact {
			return nil
		}
		return &g.Spans
	}
	if len(out.Groups) == 0 {
		for _, g := range d.Groups {
			out.Groups = append(out.Groups, traceGroupOut{Value: g.Value, Spans: g.Spans, Errors: g.Errors, traceQuantilesOut: exactOut(g), Measured: measured(g)})
		}
		if len(out.Groups) > 0 {
			out.GroupBy, out.Truncated = groupBy, d.GroupsTruncated
		}
		return
	}
	for i := range out.Groups {
		g, ok := byValue[out.Groups[i].Value]
		out.Groups[i].traceQuantilesOut = traceQuantilesOut{}
		if ok && g.Spans > 0 {
			out.Groups[i].traceQuantilesOut, out.Groups[i].Measured = exactOut(g), measured(g)
		}
	}
}

func exactOut(q datasource.DurationQuantiles) traceQuantilesOut {
	if q.Spans == 0 {
		return traceQuantilesOut{}
	}
	return traceQuantilesOut{P50MS: &q.P50, P95MS: &q.P95, P99MS: &q.P99}
}

func newTraceStatsOut(s datasource.TraceStats) traceStatsOut {
	out := traceStatsOut{StepMS: s.Step.Milliseconds(), Partial: s.Partial, Spans: s.Spans, Errors: s.Errors, Buckets: make([]traceBucketOut, len(s.Buckets)), Histogram: make([]durationBucketOut, len(s.Histogram)), GroupBy: s.GroupBy, Truncated: s.GroupsTruncated, Note: s.Note}
	for i, b := range s.Buckets {
		out.Buckets[i] = traceBucketOut{Start: b.Start.UTC().Format(time.RFC3339), Spans: b.Spans, Errors: b.Errors, P50MS: b.P50, P95MS: b.P95, P99MS: b.P99}
	}
	for i, b := range s.Histogram {
		out.Histogram[i] = durationBucketOut{LeMS: b.LeMS, Count: b.Count}
	}
	if s.Window != nil {
		q := quantilesOut(*s.Window)
		out.Window = &q
	}
	for _, g := range s.Groups {
		out.Groups = append(out.Groups, traceGroupOut{Value: g.Value, Spans: g.Spans, Errors: g.Errors, traceQuantilesOut: quantilesOut(g.TraceQuantiles)})
	}
	if s.Window != nil || len(s.Groups) > 0 {
		out.Quantiles = quantilesApproximate
		out.Note = strings.TrimSpace(approximateQuantilesNote + " " + out.Note)
	}
	return out
}

func quantilesOut(q datasource.TraceQuantiles) traceQuantilesOut {
	return traceQuantilesOut{P50MS: q.P50, P95MS: q.P95, P99MS: q.P99}
}

func describeP95(s traceStatsOut) string {
	if s.Window != nil {
		return formatP95(s.Window.P95MS)
	}
	var worst *float64
	for _, b := range s.Buckets {
		if b.P95MS != nil && (worst == nil || *b.P95MS > *worst) {
			worst = b.P95MS
		}
	}
	if worst == nil {
		return "n/a"
	}
	return formatP95(worst) + " in the peak bucket"
}

func formatP95(p *float64) string {
	if p == nil {
		return "n/a"
	}
	return strconv.FormatFloat(*p, 'f', 1, 64) + " ms"
}
