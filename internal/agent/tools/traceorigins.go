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
	nameTraceErrorOrigins = "trace_error_origins"
	maxOriginGroups       = 20
	maxRelayServices      = 5
	maxOriginMessageBytes = 200

	descTraceErrorOrigins = "Find where failing requests start: over a window, count each failing trace once at its deepest erroring span with no error below it (the origin; failing spans above it only relay the failure), grouped by that span's service, operation and status message, with the trace count, share and one full example trace id per group, and relayed_by naming the services above it that also failed. by_route splits the groups by route (the root span name), each share then being of that route's examined traces. Reads Tempo, Jaeger, ClickHouse and an Elastic or OpenSearch datasource with spans: jaeger. Up to 200 failing traces are read, spread over the window and across routes: failing_traces is how many the window holds (complete says whether that is all of them), examined how many were read, and sample whether the figures rest on fewer than all. Use it before naming the service an error began in, instead of reading single traces by hand."

	schemaTraceErrorOrigins = `{"type":"object","properties":{` + propDatasource + `,` + propTraceService + `,` + propTraceRoute + `,` + propTraceOperation + `,"by_route":{"type":"boolean","description":"Split the groups by route. Default false."},` + propFrom + `,` + propTo + `},"required":["datasource"],"additionalProperties":false}`

	originsNote = "Each failing trace is counted once, at its deepest erroring span with no error below it; failing spans above it relayed the error and are counted in relayed_by, not as origins."
)

func init() { registerEntries((*Registry).originEntries) }

func (r *Registry) originEntries() []entry {
	return []entry{
		{Tool{Name: nameTraceErrorOrigins, Description: descTraceErrorOrigins, InputSchema: json.RawMessage(schemaTraceErrorOrigins), ReadOnly: true, MinRole: auth.RoleViewer}, r.traceErrorOrigins},
	}
}

type errorOriginsOut struct {
	From         string           `json:"from"`
	To           string           `json:"to"`
	Failing      int              `json:"failing_traces"`
	Complete     bool             `json:"complete"`
	Examined     int              `json:"examined"`
	Sample       bool             `json:"sample"`
	Unattributed int              `json:"unattributed,omitempty"`
	Routes       []originRouteOut `json:"routes,omitempty"`
	Groups       []originGroupOut `json:"groups"`
	Truncated    bool             `json:"groups_truncated,omitempty"`
	Note         string           `json:"note"`
}

type originRouteOut struct {
	Route    string `json:"route"`
	Failing  int    `json:"failing_traces"`
	Examined int    `json:"examined"`
}

type originGroupOut struct {
	Route          string          `json:"route,omitempty"`
	Service        string          `json:"service"`
	Operation      string          `json:"operation"`
	StatusMessage  string          `json:"status_message,omitempty"`
	Traces         int             `json:"traces"`
	Share          float64         `json:"share"`
	ExampleTraceID string          `json:"example_trace_id"`
	RelayedBy      []relayCountOut `json:"relayed_by,omitempty"`
}

type relayCountOut struct {
	Service string `json:"service"`
	Traces  int    `json:"traces"`
}

type originKey struct{ route, service, operation, message string }

func (r *Registry) traceErrorOrigins(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource string `json:"datasource"`
		Service    string `json:"service"`
		Route      string `json:"route"`
		Operation  string `json:"operation"`
		ByRoute    bool   `json:"by_route"`
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
	source, ok := inst.Datasource.(traceSampler)
	if !ok {
		return invalid("%s %s", nameTraceErrorOrigins, traceAnalyticsText), nil
	}
	rng, res, ok := resolveRunRange(ctx, in.From, in.To)
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	step := "Find error origins on " + inst.UID
	release, err := r.acquireUpstream(ctx, inst.UID)
	if err != nil {
		return datasourceFailure(step, err)
	}
	defer release()
	filter := traceFilterOf(in.Service, in.Route, in.Operation)
	list, err := source.ListTraces(ctx, datasource.TraceListRequest{Filter: filter, Range: rng, Errors: true, Limit: datasource.TraceListBudget, Limits: queryproxy.DraftLimits})
	datasource.NoteQuery(ctx, inst.UID, "failing traces "+filterText(filter), rng, len(list.Traces))
	if err != nil {
		return datasourceFailure(step, err)
	}
	picks := pickSample(list.Traces, traceSampleLimit, in.ByRoute)
	read, cut, err := r.readSample(ctx, inst.UID, source, picks, rng)
	if err != nil {
		return datasourceFailure(step, err)
	}
	out := foldOrigins(list, read, in.ByRoute)
	out.From, out.To = rng.From.UTC().Format(time.RFC3339), rng.To.UTC().Format(time.RFC3339)
	out.Sample = out.Sample || cut
	out.Note = originsNote + sampleNote(out.Sample, out.Examined, out.Failing, out.Complete, "failing")
	text := fmt.Sprintf("%s: %s examined of %d failing, %s", step, plural(out.Examined, "trace", "traces"), out.Failing, plural(len(out.Groups), "origin", "origins"))
	return success(text, out), nil
}

func sampleNote(sample bool, examined, listed int, complete bool, kind string) string {
	if !sample {
		return fmt.Sprintf(" The figures cover all %d %s traces in the window.", listed, kind)
	}
	of := fmt.Sprintf("the %d %s traces in the window", listed, kind)
	if !complete {
		of = fmt.Sprintf("at least %d %s traces (the newest listed)", listed, kind)
	}
	return fmt.Sprintf(" The figures rest on %d of %s, read spread over the window, so counts and shares are a sample: state them as one.", examined, of)
}

func foldOrigins(list datasource.TraceList, read []sampledTrace, byRoute bool) errorOriginsOut {
	out := errorOriginsOut{Failing: len(list.Traces), Complete: list.Complete, Examined: len(read), Groups: []originGroupOut{}}
	out.Sample = !list.Complete || len(read) < len(list.Traces)
	examinedIn := map[string]int{}
	groups := map[originKey]*originGroupOut{}
	relays := map[originKey]map[string]int{}
	for _, t := range read {
		scope := routeScope(t.summary.Route, byRoute)
		examinedIn[scope]++
		origin, ok := originOf(t.spans)
		if !ok || t.truncated {
			out.Unattributed++
			continue
		}
		key := originKey{route: scope, service: origin.span.service, operation: origin.span.name, message: clip(origin.span.statusMessage, maxOriginMessageBytes)}
		g, ok := groups[key]
		if !ok {
			g = &originGroupOut{Route: key.route, Service: key.service, Operation: key.operation, StatusMessage: key.message, ExampleTraceID: t.summary.ID}
			groups[key], relays[key] = g, map[string]int{}
		}
		g.Traces++
		seen := map[string]bool{}
		for _, relay := range origin.relays {
			if relay.service != origin.span.service && !seen[relay.service] {
				seen[relay.service] = true
				relays[key][relay.service]++
			}
		}
	}
	for key, g := range groups {
		g.Share = float64(g.Traces) / float64(examinedIn[key.route])
		g.RelayedBy = relayCounts(relays[key])
		out.Groups = append(out.Groups, *g)
	}
	slices.SortFunc(out.Groups, func(a, b originGroupOut) int {
		return cmp.Or(cmp.Compare(examinedIn[b.Route], examinedIn[a.Route]), cmp.Compare(a.Route, b.Route), cmp.Compare(b.Traces, a.Traces), cmp.Compare(a.Service, b.Service), cmp.Compare(a.Operation, b.Operation), cmp.Compare(a.StatusMessage, b.StatusMessage))
	})
	if len(out.Groups) > maxOriginGroups {
		out.Groups, out.Truncated = out.Groups[:maxOriginGroups], true
	}
	if byRoute {
		out.Routes = routeCounts(list.Traces, examinedIn)
	}
	return out
}

func routeScope(route string, byRoute bool) string {
	if byRoute {
		return route
	}
	return ""
}

func relayCounts(counts map[string]int) []relayCountOut {
	out := make([]relayCountOut, 0, len(counts))
	for service, n := range counts {
		out = append(out, relayCountOut{Service: service, Traces: n})
	}
	slices.SortFunc(out, func(a, b relayCountOut) int {
		return cmp.Or(cmp.Compare(b.Traces, a.Traces), cmp.Compare(a.Service, b.Service))
	})
	return out[:min(len(out), maxRelayServices)]
}

func routeCounts(listed []datasource.TraceSummary, examined map[string]int) []originRouteOut {
	var out []originRouteOut
	for _, r := range routesOf(listed) {
		if len(out) < maxSampledRoutes {
			out = append(out, originRouteOut{Route: r.name, Failing: len(r.traces), Examined: examined[r.name]})
		}
	}
	return out
}
