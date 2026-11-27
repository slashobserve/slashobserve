package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	nameTraceSlowPath = "trace_slow_path"
	slowMultiple      = 2
	maxSlowSpans      = 3
	maxThresholdMS    = 86_400_000

	descTraceSlowPath = "Find where slow requests spend their time, per route: for the traces over a threshold on a route or service (threshold_ms, else twice each route's p50 over the listed traces of the window), report for each route (the root span name) separately the spans with the most self time across its slow traces (service, operation, self time and share), how many slow traces it has, every service that appears in them with its trace count, and one full example trace id with its exact duration. Reads Tempo, Jaeger, ClickHouse and an Elastic or OpenSearch datasource with spans: jaeger. Up to 2,000 traces are listed and up to 200 slow ones read, spread over the window and taken from every route in turn: complete and sample say whether the figures cover all of them. Use it for a latency question about each route it names, instead of reading the newest few slow traces."

	schemaTraceSlowPath = `{"type":"object","properties":{` + propDatasource + `,` + propTraceService + `,` + propTraceRoute + `,` + propTraceOperation + `,"threshold_ms":{"type":"number","exclusiveMinimum":0,"maximum":86400000,"description":"A trace slower than this is slow. Default twice its route's p50."},` + propFrom + `,` + propTo + `},"required":["datasource"],"additionalProperties":false}`

	slowPathNote   = "Self time is a span's duration less the time its children cover; the example duration is exact, read from the whole trace."
	tempoListNote  = " Tempo lists trace durations in whole milliseconds, so the threshold and p50 rest on those."
	thresholdGiven = "given"
	thresholdP50   = "twice the route p50"
)

func init() { registerEntries((*Registry).slowEntries) }

func (r *Registry) slowEntries() []entry {
	return []entry{
		{Tool{Name: nameTraceSlowPath, Description: descTraceSlowPath, InputSchema: json.RawMessage(schemaTraceSlowPath), ReadOnly: true, MinRole: auth.RoleViewer}, r.traceSlowPath},
	}
}

type slowPathOut struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Threshold string         `json:"threshold"`
	Listed    int            `json:"listed_traces"`
	Complete  bool           `json:"complete"`
	Sample    bool           `json:"sample"`
	Routes    []slowRouteOut `json:"routes"`
	Truncated bool           `json:"routes_truncated,omitempty"`
	Note      string         `json:"note"`
}

type slowRouteOut struct {
	Route       string            `json:"route"`
	ThresholdMS float64           `json:"threshold_ms"`
	P50MS       *float64          `json:"p50_ms,omitempty"`
	SlowTraces  int               `json:"slow_traces"`
	Examined    int               `json:"examined"`
	TopSelfTime []selfTimeOut     `json:"top_self_time"`
	Services    []serviceCountOut `json:"services"`
	Example     *exampleTraceOut  `json:"example,omitempty"`
}

type selfTimeOut struct {
	Service    string  `json:"service"`
	Operation  string  `json:"operation"`
	SelfTimeMS float64 `json:"self_time_ms"`
	Share      float64 `json:"share"`
	Traces     int     `json:"traces"`
}

type serviceCountOut struct {
	Service string `json:"service"`
	Traces  int    `json:"traces"`
}

type exampleTraceOut struct {
	TraceID    string  `json:"trace_id"`
	DurationMS float64 `json:"duration_ms"`
}

type slowSet struct {
	traces     []datasource.TraceSummary
	thresholds map[string]float64
	p50s       map[string]float64
}

func (r *Registry) traceSlowPath(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource  string   `json:"datasource"`
		Service     string   `json:"service"`
		Route       string   `json:"route"`
		Operation   string   `json:"operation"`
		ThresholdMS *float64 `json:"threshold_ms"`
		From        string   `json:"from"`
		To          string   `json:"to"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.ThresholdMS != nil && (*in.ThresholdMS <= 0 || *in.ThresholdMS > maxThresholdMS) {
		return invalid("threshold_ms must be above 0 and at most %d", maxThresholdMS), nil
	}
	inst, res, ok := r.instance(ctx, in.Datasource)
	if !ok {
		return res, nil
	}
	source, ok := inst.Datasource.(traceSampler)
	if !ok {
		return invalid("%s %s", nameTraceSlowPath, traceAnalyticsText), nil
	}
	rng, res, ok := resolveRunRange(ctx, in.From, in.To)
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	step := "Find the slow path on " + inst.UID
	release, err := r.acquireUpstream(ctx, inst.UID)
	if err != nil {
		return datasourceFailure(step, err)
	}
	defer release()
	filter := traceFilterOf(in.Service, in.Route, in.Operation)
	req := datasource.TraceListRequest{Filter: filter, Range: rng, Limit: datasource.TraceListBudget, Limits: queryproxy.DraftLimits}
	if in.ThresholdMS != nil {
		req.MinDuration = time.Duration(*in.ThresholdMS * float64(time.Millisecond))
	}
	list, err := source.ListTraces(ctx, req)
	datasource.NoteQuery(ctx, inst.UID, "traces "+filterText(filter, "min="+req.MinDuration.String()), rng, len(list.Traces))
	if err != nil {
		return datasourceFailure(step, err)
	}
	slow := slowTracesOf(list.Traces, in.ThresholdMS)
	read, cut, err := r.readSample(ctx, inst.UID, source, pickSample(slow.traces, traceSampleLimit, true), rng)
	if err != nil {
		return datasourceFailure(step, err)
	}
	out := foldSlowPath(slow, read)
	out.describe(rng, list, len(slow.traces), len(read), cut, in.ThresholdMS != nil, inst.Datasource.Type() == "tempo")
	text := fmt.Sprintf("%s: %s over the threshold, %s", step, plural(len(slow.traces), "trace", "traces"), plural(len(out.Routes), "route", "routes"))
	return success(text, out), nil
}

func (out *slowPathOut) describe(rng datasource.TimeRange, list datasource.TraceList, slow, read int, cut, given, tempo bool) {
	out.From, out.To, out.Listed, out.Complete = rng.From.UTC().Format(time.RFC3339), rng.To.UTC().Format(time.RFC3339), len(list.Traces), list.Complete
	out.Threshold = thresholdP50
	if given {
		out.Threshold = thresholdGiven
	}
	out.Sample = cut || !list.Complete || read < slow
	out.Note = slowPathNote + sampleNote(out.Sample, read, slow, list.Complete, "slow")
	if tempo {
		out.Note += tempoListNote
	}
}

func slowTracesOf(listed []datasource.TraceSummary, thresholdMS *float64) slowSet {
	set := slowSet{thresholds: map[string]float64{}}
	if thresholdMS != nil {
		for _, t := range listed {
			set.thresholds[t.Route] = *thresholdMS
		}
		set.traces = listed
		return set
	}
	set.p50s = map[string]float64{}
	for _, route := range routesOf(listed) {
		durations := make([]int64, 0, len(route.traces))
		for _, t := range route.traces {
			durations = append(durations, int64(t.Duration))
		}
		slices.Sort(durations)
		p50 := millisOf(float64(datasource.ExactQuantile(durations, 0.5)))
		set.p50s[route.name], set.thresholds[route.name] = p50, slowMultiple*p50
	}
	for _, t := range listed {
		if millisOf(float64(t.Duration)) > set.thresholds[t.Route] {
			set.traces = append(set.traces, t)
		}
	}
	return set
}

type spanKey struct{ service, operation string }

type routeFold struct {
	slow, examined int
	self           map[spanKey]float64
	selfTraces     map[spanKey]int
	services       map[string]int
	example        *exampleTraceOut
	total          float64
}

func foldSlowPath(set slowSet, read []sampledTrace) slowPathOut {
	folds := map[string]*routeFold{}
	at := func(route string) *routeFold {
		if folds[route] == nil {
			folds[route] = &routeFold{self: map[spanKey]float64{}, selfTraces: map[spanKey]int{}, services: map[string]int{}}
		}
		return folds[route]
	}
	for _, t := range set.traces {
		at(t.Route).slow++
	}
	for _, t := range read {
		at(t.summary.Route).add(t)
	}
	out := slowPathOut{Routes: []slowRouteOut{}}
	for route, f := range folds {
		out.Routes = append(out.Routes, f.out(route, set))
	}
	slices.SortFunc(out.Routes, func(a, b slowRouteOut) int {
		return cmp.Or(cmp.Compare(b.SlowTraces, a.SlowTraces), cmp.Compare(a.Route, b.Route))
	})
	if len(out.Routes) > maxSampledRoutes {
		out.Routes, out.Truncated = out.Routes[:maxSampledRoutes], true
	}
	return out
}

func (f *routeFold) add(t sampledTrace) {
	f.examined++
	self := selfTimes(t.spans)
	seenSpan, seenService := map[spanKey]bool{}, map[string]bool{}
	for _, s := range t.spans {
		key := spanKey{s.service, s.name}
		f.self[key] += self[s.id]
		f.total += self[s.id]
		if !seenSpan[key] {
			seenSpan[key] = true
			f.selfTraces[key]++
		}
		if !seenService[s.service] {
			seenService[s.service] = true
			f.services[s.service]++
		}
	}
	if t.truncated || len(t.spans) == 0 {
		return
	}
	if d := millisOf(extentOf(t.spans)); f.example == nil || d > f.example.DurationMS {
		f.example = &exampleTraceOut{TraceID: t.summary.ID, DurationMS: d}
	}
}

func (f *routeFold) out(route string, set slowSet) slowRouteOut {
	r := slowRouteOut{Route: route, ThresholdMS: set.thresholds[route], SlowTraces: f.slow, Examined: f.examined, TopSelfTime: []selfTimeOut{}, Services: []serviceCountOut{}, Example: f.example}
	if p50, ok := set.p50s[route]; ok {
		r.P50MS = &p50
	}
	for key, self := range f.self {
		share := 0.0
		if f.total > 0 {
			share = self / f.total
		}
		r.TopSelfTime = append(r.TopSelfTime, selfTimeOut{Service: key.service, Operation: key.operation, SelfTimeMS: millisOf(self), Share: share, Traces: f.selfTraces[key]})
	}
	slices.SortFunc(r.TopSelfTime, func(a, b selfTimeOut) int {
		return cmp.Or(cmp.Compare(b.SelfTimeMS, a.SelfTimeMS), cmp.Compare(a.Service, b.Service), cmp.Compare(a.Operation, b.Operation))
	})
	r.TopSelfTime = r.TopSelfTime[:min(len(r.TopSelfTime), maxSlowSpans)]
	for service, n := range f.services {
		r.Services = append(r.Services, serviceCountOut{Service: service, Traces: n})
	}
	slices.SortFunc(r.Services, func(a, b serviceCountOut) int {
		return cmp.Or(cmp.Compare(b.Traces, a.Traces), cmp.Compare(a.Service, b.Service))
	})
	return r
}
