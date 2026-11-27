package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

type sampleFakeDS struct {
	*fakeDS
	list   datasource.TraceList
	traces map[string][]traceSpan

	mu       sync.Mutex
	listReqs []datasource.TraceListRequest
	read     []string
}

func (d *sampleFakeDS) ListTraces(_ context.Context, req datasource.TraceListRequest) (datasource.TraceList, error) {
	d.mu.Lock()
	d.listReqs = append(d.listReqs, req)
	d.mu.Unlock()
	return d.list, nil
}

func (d *sampleFakeDS) Trace(_ context.Context, id string, _ datasource.TimeRange, _ int) (datasource.Frame, error) {
	d.mu.Lock()
	d.read = append(d.read, id)
	d.mu.Unlock()
	spans, ok := d.traces[id]
	if !ok {
		return datasource.Frame{}, datasource.Errorf(datasource.ErrNotFound, "no trace")
	}
	return spanFrameOf(id, spans), nil
}

func spanFrameOf(id string, spans []traceSpan) datasource.Frame {
	names := []string{"TraceId", "SpanId", "ParentSpanId", "SpanName", "ServiceName", "StartTime", "Duration", "StatusCode", "StatusMessage"}
	f := datasource.Frame{Fields: make([]datasource.Field, len(names))}
	for i, name := range names {
		f.Fields[i] = datasource.Field{Name: name}
	}
	for _, s := range spans {
		row := []any{id, s.id, s.parent, s.name, s.service, s.start, int64(s.duration), s.status, s.statusMessage}
		for c := range row {
			f.Fields[c].Values = append(f.Fields[c].Values, row[c])
		}
	}
	return f
}

func newSampleRig(t *testing.T, ds *sampleFakeDS) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	ds.fakeDS = &fakeDS{typ: "tempo"}
	reg, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: "traces", Name: "Traces", Datasource: ds},
		datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: &fakeDS{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger, queryproxy.WithUpstreamWait(30*time.Millisecond)), Logger: logger})
}

func sampleID(n int) string { return fmt.Sprintf("%032x", n) }

func failingTrace(origin, message string, relayed bool) []traceSpan {
	rootStatus := "Unset"
	if relayed {
		rootStatus = "Error"
	}
	deep := span("p", "c", origin, "Charge", "Error", 2*time.Millisecond, 10*time.Millisecond)
	deep.statusMessage = message
	return []traceSpan{
		span("r", "", "frontend", "GET /checkout", rootStatus, 0, 100*time.Millisecond),
		span("c", "r", "checkout", "PlaceOrder", "Error", time.Millisecond, 50*time.Millisecond),
		deep,
	}
}

func TestTraceErrorOriginsCountsEachTraceOnceAtItsDeepestError(t *testing.T) {
	ds := &sampleFakeDS{traces: map[string][]traceSpan{}}
	for i := range 4 {
		ds.list.Traces = append(ds.list.Traces, datasource.TraceSummary{ID: sampleID(i), Route: "GET /checkout"})
		ds.traces[sampleID(i)] = failingTrace("payment", "card declined", i%2 == 0)
	}
	ds.list.Traces = append(ds.list.Traces, datasource.TraceSummary{ID: sampleID(9), Route: "GET /cart"})
	ds.traces[sampleID(9)] = []traceSpan{span("r", "", "frontend", "GET /cart", "Error", 0, time.Millisecond)}
	ds.list.Complete = true
	r := newSampleRig(t, ds)
	res, err := call(t, r, nameTraceErrorOrigins, `{"datasource":"traces","service":"payment","from":"2026-11-06T10:00:00Z","to":"2026-11-06T11:00:00Z"}`)
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
	if req := ds.listReqs[0]; !req.Errors || req.Filter.Service != "payment" || req.Limit != datasource.TraceListBudget {
		t.Fatalf("list request = %+v", req)
	}
	out := res.Content.(errorOriginsOut)
	if out.Failing != 5 || !out.Complete || out.Examined != 5 || out.Sample {
		t.Fatalf("counts = %+v", out)
	}
	if len(out.Groups) != 2 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	pay := out.Groups[0]
	if pay.Service != "payment" || pay.Operation != "Charge" || pay.StatusMessage != "card declined" || pay.Traces != 4 || pay.Share != 0.8 || pay.ExampleTraceID != sampleID(0) {
		t.Fatalf("payment group = %+v", pay)
	}
	if !slices.Equal(pay.RelayedBy, []relayCountOut{{Service: "checkout", Traces: 4}, {Service: "frontend", Traces: 2}}) {
		t.Fatalf("relayed by = %+v", pay.RelayedBy)
	}
	if front := out.Groups[1]; front.Service != "frontend" || front.Traces != 1 {
		t.Fatalf("frontend group = %+v", front)
	}
	if !strings.Contains(out.Note, "cover all 5 failing traces") {
		t.Fatalf("note = %q", out.Note)
	}
}

func TestTraceErrorOriginsSplitsByRouteAndSaysWhenItSampled(t *testing.T) {
	ds := &sampleFakeDS{traces: map[string][]traceSpan{}}
	for i := range traceSampleLimit + 50 {
		ds.list.Traces = append(ds.list.Traces, datasource.TraceSummary{ID: sampleID(i), Route: "GET /checkout"})
		ds.traces[sampleID(i)] = failingTrace("payment", "declined", false)
	}
	ds.list.Traces = append(ds.list.Traces, datasource.TraceSummary{ID: sampleID(999), Route: "GET /cart"})
	ds.traces[sampleID(999)] = failingTrace("cart", "redis down", false)
	r := newSampleRig(t, ds)
	res, err := call(t, r, nameTraceErrorOrigins, `{"datasource":"traces","by_route":true}`)
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
	out := res.Content.(errorOriginsOut)
	if out.Examined != traceSampleLimit || !out.Sample || out.Complete {
		t.Fatalf("counts = %+v", out)
	}
	if !slices.Contains(ds.read, sampleID(999)) {
		t.Fatal("the rare route was not read")
	}
	if len(out.Routes) != 2 || out.Routes[0] != (originRouteOut{Route: "GET /checkout", Failing: 250, Examined: 199}) || out.Routes[1] != (originRouteOut{Route: "GET /cart", Failing: 1, Examined: 1}) {
		t.Fatalf("routes = %+v", out.Routes)
	}
	cart := out.Groups[1]
	if cart.Route != "GET /cart" || cart.Service != "cart" || cart.Share != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	if !strings.Contains(out.Note, "rest on 200 of at least 251 failing traces") {
		t.Fatalf("note = %q", out.Note)
	}
	body, _ := json.Marshal(out)
	if !strings.Contains(string(body), `"example_trace_id":"`+sampleID(999)+`"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestTraceErrorOriginsRefusesADatasourceWithoutTraces(t *testing.T) {
	r := newSampleRig(t, &sampleFakeDS{})
	res, err := call(t, r, nameTraceErrorOrigins, `{"datasource":"fake"}`)
	if err != nil || !res.IsError || !strings.Contains(res.Content.(errorBody).Message, "reads Tempo, Jaeger and ClickHouse") {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
}

func TestPickSampleSpreadsAndTakesEveryRoute(t *testing.T) {
	var traces []datasource.TraceSummary
	for i := range 10 {
		traces = append(traces, datasource.TraceSummary{ID: sampleID(i), Route: "a"})
	}
	traces = append(traces, datasource.TraceSummary{ID: "b1", Route: "b"}, datasource.TraceSummary{ID: "c1", Route: "c"})
	ids := func(picks []datasource.TraceSummary) []string {
		out := []string{}
		for _, p := range picks {
			out = append(out, p.ID)
		}
		return out
	}
	for _, tc := range []struct {
		name    string
		limit   int
		byRoute bool
		want    []string
	}{
		{"under the limit keeps all", 20, false, ids(traces)},
		{"spread over the listing", 3, false, []string{sampleID(0), sampleID(4), sampleID(8)}},
		{"by route takes each route", 4, true, []string{sampleID(0), sampleID(5), "b1", "c1"}},
		{"by route with a limit under the routes keeps the biggest", 2, true, []string{sampleID(0), "b1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ids(pickSample(traces, tc.limit, tc.byRoute)); !slices.Equal(got, tc.want) {
				t.Fatalf("picks = %v, want %v", got, tc.want)
			}
		})
	}
}
