package tools

import (
	"cmp"
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

const fixtureTraceID = "0af7651916cd43dd8448eb211c80319c"

var fixtureStart = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

type traceFakeDS struct {
	*fakeDS
	traceFn func(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error)
	statsFn func(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error)

	mu    sync.Mutex
	stats []datasource.TraceStatsRequest
}

func (d *traceFakeDS) Trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	return d.traceFn(ctx, id, r, maxSpans)
}

func (d *traceFakeDS) TraceStats(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
	d.mu.Lock()
	d.stats = append(d.stats, req)
	d.mu.Unlock()
	return d.statsFn(ctx, req)
}

func newTraceRig(t *testing.T) (*Registry, *traceFakeDS) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	ds := &traceFakeDS{
		fakeDS: &fakeDS{typ: "tempo"},
		traceFn: func(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
			return thirtySpans(), nil
		},
		statsFn: func(context.Context, datasource.TraceStatsRequest) (datasource.TraceStats, error) {
			return datasource.TraceStats{}, nil
		},
	}
	reg, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: &fakeDS{}},
		datasource.Instance{UID: "tempo", Name: "Tempo", Datasource: ds},
	)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger, queryproxy.WithUpstreamWait(30*time.Millisecond)), Logger: logger}), ds
}

const linkedTraceID = "1b2c3d4e5f60718293a4b5c6d7e8f901"

func thirtySpans() datasource.Frame {
	names := []string{"TraceId", "SpanId", "ParentSpanId", "SpanName", "ServiceName", "StartTime", "Duration", "StatusCode", "SpanAttributes", "ResourceAttributes", "SpanKind", "SpanEvents", "SpanLinks"}
	f := datasource.Frame{Fields: make([]datasource.Field, len(names))}
	for i, name := range names {
		f.Fields[i] = datasource.Field{Name: name, Type: datasource.FieldString}
	}
	add := func(id, parent, name, service string, offset, duration time.Duration, status string) {
		row := []any{fixtureTraceID, id, parent, name, service, fixtureStart.Add(offset), int64(duration), status, "{}", "{}", "Server", "[]", "[]"}
		for c := range row {
			f.Fields[c].Values = append(f.Fields[c].Values, row[c])
		}
	}
	add("root000000000000", "", "POST /checkout", "gateway", 0, 900*time.Millisecond, "Unset")
	add("chk0000000000000", "root000000000000", "checkout", "checkout", 10*time.Millisecond, 800*time.Millisecond, "Unset")
	add("pay0000000000000", "chk0000000000000", "charge", "payments", 20*time.Millisecond, 700*time.Millisecond, "STATUS_CODE_ERROR")
	add("db00000000000000", "pay0000000000000", "SELECT cards", "payments", 30*time.Millisecond, 650*time.Millisecond, "Error")
	f.Fields[12].Values[3] = `[{"trace_id":"` + linkedTraceID + `","span_id":"b7ad6b7169203331","attributes":{}},{"trace_id":"` + linkedTraceID + `","span_id":"00f067aa0ba902b7","attributes":{}}]`
	for i := range 26 {
		add(fmt.Sprintf("inv%013d", i), "chk0000000000000", "reserve", "inventory", time.Duration(40+i)*time.Millisecond, time.Duration(i+1)*time.Millisecond, "Ok")
	}
	return f
}

func decodeTrace(t *testing.T, res Result) traceOut {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	out, ok := res.Content.(traceOut)
	if !ok {
		t.Fatalf("content is %T", res.Content)
	}
	return out
}

func TestGetTraceSummarizesServicesErrorsAndTheSlowestSpans(t *testing.T) {
	r, _ := newTraceRig(t)
	res, err := call(t, r, nameGetTrace, `{"datasource":"tempo","trace_id":"`+strings.ToUpper(fixtureTraceID)+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	out := decodeTrace(t, res)
	if want := "Read trace 0AF76519 on tempo: 30 spans, 4 services, 2 errors"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	if out.TraceID != strings.ToUpper(fixtureTraceID) || out.Spans != 30 || out.SpansReturned != 30 || out.Truncated || out.DurationMS != 900 {
		t.Fatalf("out = %+v", out)
	}
	if out.Root == nil || out.Root.SpanID != "root000000000000" || out.Root.Service != "gateway" || out.Root.Name != "POST /checkout" {
		t.Fatalf("root = %+v", out.Root)
	}
	wantServices := []serviceOut{{"inventory", 26, 0}, {"payments", 2, 2}, {"checkout", 1, 0}, {"gateway", 1, 0}}
	if fmt.Sprint(out.Services) != fmt.Sprint(wantServices) {
		t.Fatalf("services = %+v, want %+v", out.Services, wantServices)
	}
	wantErrors := []spanRefOut{
		{SpanID: "pay0000000000000", ParentSpanID: "chk0000000000000", Name: "charge", Service: "payments", Depth: 2},
		{SpanID: "db00000000000000", ParentSpanID: "pay0000000000000", Name: "SELECT cards", Service: "payments", Depth: 3, Origin: true},
	}
	if fmt.Sprint(out.Errors) != fmt.Sprint(wantErrors) {
		t.Fatalf("errors = %+v, want %+v", out.Errors, wantErrors)
	}
	if len(out.Slowest) != traceListLimit {
		t.Fatalf("slowest has %d spans, want %d", len(out.Slowest), traceListLimit)
	}
	if out.Links != 1 || !slices.Equal(out.LinkedTraces, []string{linkedTraceID}) {
		t.Fatalf("links = %d to %v, want one linked span to %s", out.Links, out.LinkedTraces, linkedTraceID)
	}
	wantSlowest := []slowSpanOut{
		{"root000000000000", "POST /checkout", "gateway", 900, 100, 0, 0},
		{"chk0000000000000", "checkout", "checkout", 800, 100, 1, 0},
		{"pay0000000000000", "charge", "payments", 700, 50, 2, 0},
		{"db00000000000000", "SELECT cards", "payments", 650, 650, 3, 0},
		{"inv0000000000025", "reserve", "inventory", 26, 26, 2, 0},
	}
	if fmt.Sprint(out.Slowest[:5]) != fmt.Sprint(wantSlowest) {
		t.Fatalf("slowest = %+v, want %+v", out.Slowest[:5], wantSlowest)
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"trace_id"`, `"spans"`, `"spans_returned"`, `"truncated"`, `"services"`, `"duration_ms"`, `"root"`, `"errors"`, `"slowest"`, `"depth"`, `"parent_span_id"`, `"origin"`, `"self_time_ms"`, `"links"`, `"linked_traces"`} {
		if !strings.Contains(string(body), key) {
			t.Errorf("JSON lacks %s: %s", key, body)
		}
	}
	if strings.Contains(string(body), "status_message") {
		t.Errorf("JSON carries an empty status_message: %s", body)
	}
}

func TestGetTraceNamesTheMessageAnErrorSpanSet(t *testing.T) {
	r, ds := newTraceRig(t)
	ds.traceFn = func(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
		f := thirtySpans()
		messages := make([]any, len(f.Fields[0].Values))
		for i, id := range f.Fields[1].Values {
			if id == "pay0000000000000" {
				messages[i] = "card declined"
			} else {
				messages[i] = ""
			}
		}
		f.Fields = append(f.Fields, datasource.Field{Name: "StatusMessage", Type: datasource.FieldString, Values: messages})
		return f, nil
	}
	res, err := call(t, r, nameGetTrace, `{"datasource":"tempo","trace_id":"`+fixtureTraceID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	out := decodeTrace(t, res)
	if len(out.Errors) != 2 || out.Errors[0].StatusMessage != "card declined" || out.Errors[1].StatusMessage != "" {
		t.Fatalf("errors = %+v, want the first to carry its message", out.Errors)
	}
}

func TestGetTracePassesTruncationThroughAndSendsTheBudget(t *testing.T) {
	r, ds := newTraceRig(t)
	var got struct {
		id       string
		maxSpans int
		span     time.Duration
	}
	ds.traceFn = func(_ context.Context, id string, rng datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
		got.id, got.maxSpans, got.span = id, maxSpans, rng.To.Sub(rng.From)
		f := thirtySpans()
		f.Meta = &datasource.FrameMeta{Truncated: true, RowsTotal: 25000}
		return f, nil
	}
	res, err := call(t, r, nameGetTrace, `{"datasource":"tempo","trace_id":"`+fixtureTraceID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	out := decodeTrace(t, res)
	if !out.Truncated || out.Spans != 25000 || out.SpansReturned != 30 {
		t.Fatalf("out = spans %d, returned %d, truncated %t", out.Spans, out.SpansReturned, out.Truncated)
	}
	if got.id != fixtureTraceID || got.maxSpans != datasource.TraceSpanBudget || got.span < 24*time.Hour-time.Minute || got.span > 24*time.Hour+time.Minute {
		t.Fatalf("call = %+v", got)
	}
	if want := "Read trace 0af76519 on tempo: 25000 spans, 4 services, 2 errors"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}

func TestGetTraceRefusesBadInputAndOtherDatasources(t *testing.T) {
	r, ds := newTraceRig(t)
	ds.traceFn = func(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
		return datasource.Frame{}, datasource.Errorf(datasource.ErrNotFound, "trace not found")
	}
	cases := []struct {
		name, input, code, message string
	}{
		{"short id", `{"datasource":"tempo","trace_id":"abc"}`, codeInvalidRequest, "trace_id must be 16 or 32 hex characters"},
		{"a Prometheus datasource", `{"datasource":"fake","trace_id":"` + fixtureTraceID + `"}`, codeInvalidRequest, traceSourcesMessage},
		{"an unknown datasource", `{"datasource":"nope","trace_id":"` + fixtureTraceID + `"}`, "unknown_datasource", ""},
		{"a bad time", `{"datasource":"tempo","trace_id":"` + fixtureTraceID + `","from":"yesterday-ish"}`, codeInvalidRequest, ""},
		{"an unknown field", `{"datasource":"tempo","trace_id":"` + fixtureTraceID + `","limit":5}`, codeInvalidRequest, ""},
		{"a missing trace", `{"datasource":"tempo","trace_id":"` + fixtureTraceID + `"}`, "not_found", "not found: trace not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := call(t, r, nameGetTrace, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != tc.code {
				t.Fatalf("code = %q, want %q (%#v)", got, tc.code, res.Content)
			}
			if tc.message != "" && res.Content.(errorBody).Message != tc.message {
				t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, tc.message)
			}
		})
	}
}

func TestGetTraceAnswersTooManyRequestsWhenUpstreamIsBusy(t *testing.T) {
	r, _ := newTraceRig(t)
	for range 32 {
		release, err := r.deps.Runner.AcquireUpstream(reading(context.Background()), "tempo")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
	}
	res, err := call(t, r, nameGetTrace, `{"datasource":"tempo","trace_id":"`+fixtureTraceID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != "too_many_requests" {
		t.Fatalf("code = %q", got)
	}
}

func TestSummarizeTraceWithNoSpansAndAnOrphanRoot(t *testing.T) {
	empty := summarizeTrace(fixtureTraceID, datasource.Frame{})
	if empty.Spans != 0 || empty.Root != nil || empty.Services == nil || empty.Errors == nil || empty.Slowest == nil {
		t.Fatalf("empty = %+v", empty)
	}
	f := thirtySpans()
	f.Fields[2].Values[0] = "missing000000000"
	out := summarizeTrace(fixtureTraceID, f)
	if out.Root == nil || out.Root.SpanID != "root000000000000" {
		t.Fatalf("root = %+v, want the earliest span when no span has an empty parent", out.Root)
	}
	if out.Slowest[0].Depth != 1 || out.Slowest[1].Depth != 2 {
		t.Fatalf("depths = %d, %d, want an orphan parent to count as one level", out.Slowest[0].Depth, out.Slowest[1].Depth)
	}
}

func TestTraceStatsAlignsTheRangeAndCapsTheBuckets(t *testing.T) {
	r, ds := newTraceRig(t)
	p95 := 120.4
	ds.statsFn = func(_ context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
		return datasource.TraceStats{
			Step: req.Step, Partial: true, Spans: 42, Errors: 1,
			Buckets:   []datasource.TraceBucket{{Start: req.Range.From, Spans: 42, Errors: 1, P95: &p95}, {Start: req.Range.From.Add(req.Step)}},
			Histogram: []datasource.DurationBucket{{LeMS: 0.131072, Count: 5}},
		}, nil
	}
	res, err := call(t, r, nameTraceStats, `{"datasource":"tempo","query":"{}","from":"2026-11-06T10:00:30Z","to":"2026-11-06T11:00:30Z","buckets":30}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	if want := "Read trace stats on tempo: 42 spans, p95 120.4 ms in the peak bucket"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"step_ms":120000,"partial":true,"spans":42,"errors":1,"buckets":[{"start":"2026-11-06T10:00:00Z","spans":42,"errors":1,"p50_ms":null,"p95_ms":120.4,"p99_ms":null},{"start":"2026-11-06T10:02:00Z","spans":0,"errors":0,"p50_ms":null,"p95_ms":null,"p99_ms":null}],"histogram":[{"le_ms":0.131072,"count":5}]}`
	if string(body) != want {
		t.Fatalf("body =\n%s\nwant\n%s", body, want)
	}
	calls := ds.stats
	if len(calls) != 1 || calls[0].Query != "{}" || calls[0].Step != 2*time.Minute || calls[0].Limits != queryproxy.DraftLimits {
		t.Fatalf("calls = %+v", calls)
	}
	if from, to := calls[0].Range.From, calls[0].Range.To; !from.Equal(time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 11, 6, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("range = %v..%v, want the range floored to the step", from, to)
	}
	res, err = call(t, r, nameTraceStats, `{"datasource":"tempo","query":"{}","buckets":61}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != codeInvalidRequest {
		t.Fatalf("code = %q", got)
	}
}

func TestTraceStatsReportsTheWholeWindowAndGroups(t *testing.T) {
	r, ds := newTraceRig(t)
	bucketP95, windowP95, p50, p99 := 900.0, 410.5, 33.1, 1200.0
	ds.statsFn = func(_ context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
		return datasource.TraceStats{
			Step: req.Step, Spans: 10, Errors: 2,
			Buckets: []datasource.TraceBucket{{Start: req.Range.From, Spans: 10, Errors: 2, P95: &bucketP95}},
			Window:  &datasource.TraceQuantiles{P50: &p50, P95: &windowP95, P99: &p99},
			GroupBy: req.GroupBy,
			Groups: []datasource.TraceGroup{
				{Value: "checkout", Spans: 7, Errors: 2, TraceQuantiles: datasource.TraceQuantiles{P99: &p99}},
				{Value: "cart", Spans: 3},
			},
			GroupsTruncated: true,
		}, nil
	}
	res, err := call(t, r, nameTraceStats, `{"datasource":"tempo","query":"{}","group_by":"resource.service.name"}`)
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
	if want := "Read trace stats on tempo: 10 spans, p95 410.5 ms"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
	if got := ds.stats[0].GroupBy; got != "resource.service.name" {
		t.Fatalf("group by = %q", got)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"window":{"p50_ms":33.1,"p95_ms":410.5,"p99_ms":1200}`,
		`"group_by":"resource.service.name"`,
		`"groups":[{"value":"checkout","spans":7,"errors":2,"p50_ms":null,"p95_ms":null,"p99_ms":1200},{"value":"cart","spans":3,"errors":0,"p50_ms":null,"p95_ms":null,"p99_ms":null}]`,
		`"groups_truncated":true`,
		`"note":"Tempo quantiles are interpolated from its log-scale duration buckets, so treat them as approximate.`,
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("body = %s\nmissing %s", body, want)
		}
	}
}

func TestTraceStatsKeepsADatasourceNoteWithoutAWindow(t *testing.T) {
	r, ds := newTraceRig(t)
	ds.statsFn = func(context.Context, datasource.TraceStatsRequest) (datasource.TraceStats, error) {
		return datasource.TraceStats{Note: "per bucket only"}, nil
	}
	res, err := call(t, r, nameTraceStats, `{"datasource":"tempo","query":"x","group_by":"service"}`)
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %#v", err, res.Content)
	}
	body, _ := json.Marshal(res.Content)
	if !strings.Contains(string(body), `"note":"per bucket only"`) || strings.Contains(string(body), `"window"`) || res.Step != "Read trace stats on tempo: 0 spans, p95 n/a" {
		t.Fatalf("step = %q, body = %s", res.Step, body)
	}
}

func TestTraceStatsRefusesBadInputAndOtherDatasources(t *testing.T) {
	r, ds := newTraceRig(t)
	ds.statsFn = func(context.Context, datasource.TraceStatsRequest) (datasource.TraceStats, error) {
		return datasource.TraceStats{}, datasource.Errorf(datasource.ErrInvalidQuery, "bad filter")
	}
	cases := []struct {
		name, input, code, message string
	}{
		{"a missing query", `{"datasource":"tempo"}`, codeInvalidRequest, "query is required"},
		{"a Prometheus datasource", `{"datasource":"fake","query":"{}"}`, codeInvalidRequest, traceStatsSourcesText},
		{"an upstream rejection", `{"datasource":"tempo","query":"{"}`, "query_invalid", "invalid query: bad filter"},
		{"a long query", `{"datasource":"tempo","query":"` + strings.Repeat("x", maxQueryBytes+1) + `"}`, codeInvalidRequest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := call(t, r, nameTraceStats, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != tc.code {
				t.Fatalf("code = %q, want %q (%#v)", got, tc.code, res.Content)
			}
			if tc.message != "" && res.Content.(errorBody).Message != tc.message {
				t.Fatalf("message = %q, want %q", res.Content.(errorBody).Message, tc.message)
			}
		})
	}
	ds.statsFn = func(context.Context, datasource.TraceStatsRequest) (datasource.TraceStats, error) {
		return datasource.TraceStats{}, nil
	}
	res, err := call(t, r, nameTraceStats, `{"datasource":"tempo","query":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("an empty query is every span on ClickHouse: %#v", res.Content)
	}
	if want := "Read trace stats on tempo: 0 spans, p95 n/a"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}

type shapedSpan struct {
	id, parent, status, message, attributes, kind string
	offset, duration                              time.Duration
}

func shapedTrace(spans ...shapedSpan) datasource.Frame {
	names := []string{"SpanId", "ParentSpanId", "SpanName", "ServiceName", "StartTime", "Duration", "StatusCode", "StatusMessage", "SpanAttributes", "SpanKind"}
	f := datasource.Frame{Fields: make([]datasource.Field, len(names))}
	for i, name := range names {
		f.Fields[i] = datasource.Field{Name: name, Type: datasource.FieldString}
	}
	for _, s := range spans {
		row := []any{s.id, s.parent, "op-" + s.id, "svc", fixtureStart.Add(s.offset), int64(s.duration), s.status, s.message, s.attributes, s.kind}
		for c := range row {
			f.Fields[c].Values = append(f.Fields[c].Values, row[c])
		}
	}
	return f
}

func originsOf(refs []spanRefOut) []string {
	var ids []string
	for _, r := range refs {
		if r.Origin {
			ids = append(ids, r.SpanID)
		}
	}
	return ids
}

func TestSummarizeTraceFlagsTheDeepestErrorOfAChainAsTheOrigin(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", status: "Unset", duration: 100 * ms},
		shapedSpan{id: "b", parent: "a", status: "Error", offset: ms, duration: 90 * ms},
		shapedSpan{id: "c", parent: "b", status: "Unset", offset: 2 * ms, duration: 80 * ms},
		shapedSpan{id: "d", parent: "c", status: "Error", offset: 3 * ms, duration: 70 * ms},
	))
	if got := originsOf(out.Errors); !slices.Equal(got, []string{"d"}) {
		t.Fatalf("origins = %v, want only the deepest error across a clean span", got)
	}
	if b, d := out.Errors[0], out.Errors[1]; b.SpanID != "b" || b.ParentSpanID != "a" || b.Depth != 1 || d.ParentSpanID != "c" || d.Depth != 3 {
		t.Fatalf("errors = %+v", out.Errors)
	}
}

func TestSummarizeTraceFlagsOnlyTheDeepestSpanOfAWrappingChain(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", status: "Unset", duration: 100 * ms},
		shapedSpan{id: "outer", parent: "a", status: "Error", message: "upstream returned 500", offset: ms, duration: 90 * ms},
		shapedSpan{id: "mid", parent: "outer", status: "Error", message: "card service: timeout", offset: 2 * ms, duration: 80 * ms},
		shapedSpan{id: "leaf", parent: "mid", status: "Error", message: "timeout", offset: 3 * ms, duration: 70 * ms},
	))
	if got := originsOf(out.Errors); !slices.Equal(got, []string{"leaf"}) {
		t.Fatalf("origins = %v, want only the span with no error below it", got)
	}
}

func TestSummarizeTraceKeepsOriginsWhenTheErrorListIsCapped(t *testing.T) {
	const ms = time.Millisecond
	spans := []shapedSpan{{id: "root", status: "Unset", duration: 1000 * ms}}
	for i := range traceListLimit + 5 {
		spans = append(spans, shapedSpan{id: fmt.Sprintf("e%02d", i), parent: "root", status: "Error", offset: time.Duration(i+1) * ms, duration: ms})
	}
	spans = append(spans, shapedSpan{id: "late-child", parent: "e00", status: "Error", offset: 100 * ms, duration: ms})
	out := summarizeTrace(fixtureTraceID, shapedTrace(spans...))
	if len(out.Errors) != traceListLimit {
		t.Fatalf("errors = %d, want %d", len(out.Errors), traceListLimit)
	}
	for _, e := range out.Errors {
		if e.SpanID == "e00" {
			t.Fatalf("e00 has an error below it and should be dropped before any origin: %+v", out.Errors)
		}
		if !e.Origin {
			t.Fatalf("%s is not an origin but the list holds origins that were cut", e.SpanID)
		}
	}
	if !slices.IsSortedFunc(out.Errors, func(a, b spanRefOut) int { return cmp.Compare(a.SpanID, b.SpanID) }) {
		t.Fatalf("errors lost their start order: %+v", out.Errors)
	}
}

func TestSummarizeTraceSetsNoOriginOnATruncatedTrace(t *testing.T) {
	const ms = time.Millisecond
	f := shapedTrace(
		shapedSpan{id: "a", status: "Unset", duration: 100 * ms},
		shapedSpan{id: "b", parent: "a", status: "Error", offset: ms, duration: 90 * ms},
	)
	f.Meta = &datasource.FrameMeta{Truncated: true, RowsTotal: 50}
	out := summarizeTrace(fixtureTraceID, f)
	if len(out.Errors) != 1 || out.Errors[0].Origin || len(originsOf(out.Errors)) != 0 {
		t.Fatalf("errors = %+v, want no origin when children may be missing", out.Errors)
	}
}

func TestSummarizeTraceIgnoresASpanThatIsItsOwnParent(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", status: "Unset", duration: 100 * ms},
		shapedSpan{id: "loop", parent: "loop", status: "Error", offset: ms, duration: 50 * ms},
	))
	if got := originsOf(out.Errors); !slices.Equal(got, []string{"loop"}) {
		t.Fatalf("origins = %v", got)
	}
	for _, s := range out.Slowest {
		if s.SpanID == "loop" && s.SelfTimeMS != 50 {
			t.Fatalf("self time = %v, want the whole duration", s.SelfTimeMS)
		}
	}
}

func TestSummarizeTraceFlagsTheDeepestErrorOfEachIndependentChain(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", status: "Unset", duration: 100 * ms},
		shapedSpan{id: "left", parent: "a", status: "Error", offset: ms, duration: 40 * ms},
		shapedSpan{id: "left-leaf", parent: "left", status: "Error", offset: 2 * ms, duration: 30 * ms},
		shapedSpan{id: "right", parent: "a", status: "Error", offset: 50 * ms, duration: 40 * ms},
		shapedSpan{id: "right-leaf", parent: "right", status: "Error", offset: 51 * ms, duration: 30 * ms},
	))
	if got := originsOf(out.Errors); !slices.Equal(got, []string{"left-leaf", "right-leaf"}) {
		t.Fatalf("origins = %v, want one per chain", got)
	}
}

func TestSummarizeTraceSelfTimeSubtractsTheUnionOfOverlappingChildren(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", status: "Unset", duration: 100 * ms},
		shapedSpan{id: "x", parent: "a", status: "Unset", offset: 10 * ms, duration: 40 * ms},
		shapedSpan{id: "y", parent: "a", status: "Unset", offset: 30 * ms, duration: 40 * ms},
		shapedSpan{id: "z", parent: "a", status: "Unset", offset: 90 * ms, duration: 50 * ms},
		shapedSpan{id: "w", parent: "x", status: "Unset", offset: 10 * ms, duration: 60 * ms},
	))
	selfByID := map[string]float64{}
	for _, s := range out.Slowest {
		selfByID[s.SpanID] = s.SelfTimeMS
	}
	want := map[string]float64{"a": 100 - 60 - 10, "x": 0, "y": 40, "z": 50, "w": 60}
	for id, w := range want {
		if selfByID[id] != w {
			t.Errorf("self time of %s = %v, want %v (all: %v)", id, selfByID[id], w, selfByID)
		}
	}
}

func TestGetTraceDescriptionScopesOriginToOneTrace(t *testing.T) {
	for _, want := range []string{"Relays", "not counted in errors", "inside this one trace", "does not by itself say which service an incident started in", "across many traces", "trace_stats"} {
		if !strings.Contains(descGetTrace, want) {
			t.Errorf("description lacks %q", want)
		}
	}
	if n := len(descGetTrace); n > 1500 {
		t.Errorf("description is %d bytes, want at most 1500", n)
	}
}

func relayNames(relays []relayOut) []string {
	var names []string
	for _, r := range relays {
		names = append(names, fmt.Sprintf("%s:%d", r.Name, r.HTTPStatus))
	}
	return names
}

func TestSummarizeTraceListsServerSpansThatRelayA5xxAboveAFailureAsRelaysNotErrors(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "web", kind: "Server", status: "Unset", attributes: `{"http.response.status_code":500}`, duration: 100 * ms},
		shapedSpan{id: "gateway", parent: "web", kind: "SPAN_KIND_SERVER", status: "Unset", attributes: `{"http.status_code":"502"}`, offset: ms, duration: 90 * ms},
		shapedSpan{id: "api", parent: "gateway", kind: "Server", status: "Unset", attributes: `{"http.response.status_code":200}`, offset: 2 * ms, duration: 80 * ms},
		shapedSpan{id: "db", parent: "gateway", kind: "Server", status: "Error", message: "connection refused", attributes: `{"http.response.status_code":503}`, offset: 3 * ms, duration: 70 * ms},
	))
	if got := relayNames(out.Relays); !slices.Equal(got, []string{"op-web:500", "op-gateway:502"}) {
		t.Fatalf("relays = %v", got)
	}
	if out.Relays[0].Service != "svc" {
		t.Fatalf("relays = %+v", out.Relays)
	}
	if len(out.Errors) != 1 || out.Errors[0].SpanID != "db" || !out.Errors[0].Origin || out.Errors[0].HTTPStatus != 503 {
		t.Fatalf("errors = %+v, want only the span with an error status", out.Errors)
	}
	if out.Services[0].Errors != 1 {
		t.Fatalf("services = %+v, want error counts by status only", out.Services)
	}
	if out.Root.HTTPStatus != 500 {
		t.Fatalf("root = %+v", out.Root)
	}
}

func TestSummarizeTraceLeavesOutSpansThatAreNotRelays(t *testing.T) {
	const ms = time.Millisecond
	five := `{"http.response.status_code":503}`
	for name, above := range map[string]shapedSpan{
		"an Ok span":          {id: "a", kind: "Server", status: "Ok", attributes: five},
		"a client span":       {id: "a", kind: "Client", status: "Unset", attributes: five},
		"an unspecified kind": {id: "a", status: "Unset", attributes: five},
		"an error status":     {id: "a", kind: "Server", status: "Error", attributes: five},
		"a 4xx status":        {id: "a", kind: "Server", status: "Unset", attributes: `{"http.response.status_code":429}`},
		"an out of range one": {id: "a", kind: "Server", status: "Unset", attributes: `{"http.response.status_code":600}`},
		"no status":           {id: "a", kind: "Server", status: "Unset"},
	} {
		above.duration = 100 * ms
		out := summarizeTrace(fixtureTraceID, shapedTrace(above, shapedSpan{id: "b", parent: "a", status: "Error", offset: ms, duration: 90 * ms}))
		if len(out.Relays) != 0 {
			t.Errorf("%s: relays = %+v, want none", name, out.Relays)
		}
	}
}

func TestSummarizeTraceListsNoRelayWithoutAFailureBelow(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", kind: "Server", status: "Unset", attributes: `{"http.response.status_code":500}`, duration: 100 * ms},
		shapedSpan{id: "b", parent: "a", kind: "Server", status: "Unset", offset: ms, duration: 90 * ms},
	))
	if len(out.Relays) != 0 || len(out.Errors) != 0 {
		t.Fatalf("relays = %+v errors = %+v", out.Relays, out.Errors)
	}
}

func TestSummarizeTraceCapsTheRelayList(t *testing.T) {
	const ms = time.Millisecond
	spans := []shapedSpan{}
	parent := ""
	for i := range traceRelayLimit + 3 {
		id := fmt.Sprintf("r%d", i)
		spans = append(spans, shapedSpan{id: id, parent: parent, kind: "Server", status: "Unset", attributes: `{"http.response.status_code":500}`, offset: time.Duration(i) * ms, duration: 100 * ms})
		parent = id
	}
	spans = append(spans, shapedSpan{id: "leaf", parent: parent, status: "Error", offset: 50 * ms, duration: ms})
	out := summarizeTrace(fixtureTraceID, shapedTrace(spans...))
	if len(out.Relays) != traceRelayLimit {
		t.Fatalf("relays = %d, want %d", len(out.Relays), traceRelayLimit)
	}
}

func TestSummarizeTraceErrorCountsIgnoreHTTPStatus(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", kind: "Server", status: "Unset", attributes: `{"http.response.status_code":500}`, duration: 100 * ms},
		shapedSpan{id: "b", parent: "a", kind: "Client", status: "Unset", attributes: `{"http.response.status_code":503}`, offset: ms, duration: 90 * ms},
	))
	if len(out.Errors) != 0 || out.Services[0].Errors != 0 || len(out.Relays) != 0 {
		t.Fatalf("errors = %+v services = %+v relays = %+v", out.Errors, out.Services, out.Relays)
	}
}

func TestHTTPStatusOfReadsTheStandardAndTheOlderAttribute(t *testing.T) {
	for _, tc := range []struct {
		attributes string
		want       int
	}{
		{`{"http.response.status_code":500}`, 500},
		{`{"http.status_code":503}`, 503},
		{`{"http.status_code":"502"}`, 502},
		{`{"http.response.status_code":"201","http.status_code":500}`, 201},
		{`{"http.response.status_code":"x","http.status_code":500}`, 500},
		{`{"http.response.status_code":99}`, 0},
		{`{"http.response.status_code":600}`, 0},
		{`{"http.status_code":500.5}`, 0},
		{`{"http.status_code":"5 00"}`, 0},
		{`{"http.status_code":[500]}`, 0},
		{`{"http.status_code":null}`, 0},
		{`{"http.status_code":true}`, 0},
		{`{"http.method":"GET"}`, 0},
		{`{"other.status_code":500}`, 0},
		{`not json status_code`, 0},
		{``, 0},
	} {
		if got := httpStatusOf(tc.attributes); got != tc.want {
			t.Errorf("httpStatusOf(%q) = %d, want %d", tc.attributes, got, tc.want)
		}
	}
}

func TestGetTraceOutputPrintsTheHTTPStatusAndRelaysOnlyWhenPresent(t *testing.T) {
	const ms = time.Millisecond
	out := summarizeTrace(fixtureTraceID, shapedTrace(
		shapedSpan{id: "a", kind: "Server", status: "Unset", attributes: `{"http.status_code":500}`, duration: 100 * ms},
		shapedSpan{id: "b", parent: "a", status: "Error", offset: ms, duration: 90 * ms},
	))
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(body), `"http_status"`); n != 3 || !strings.Contains(string(body), `"relays":[{"service":"svc","name":"op-a","http_status":500}]`) {
		t.Fatalf("http_status appears %d times, want root, one relay and one slowest span: %s", n, body)
	}
	plain, err := json.Marshal(summarizeTrace(fixtureTraceID, shapedTrace(shapedSpan{id: "a", status: "Error", duration: 100 * ms})))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "http_status") || strings.Contains(string(plain), "relays") {
		t.Fatalf("http_status or relays printed without an HTTP status: %s", plain)
	}
}
