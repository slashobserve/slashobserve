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
	wantErrors := []spanRefOut{{SpanID: "pay0000000000000", Name: "charge", Service: "payments"}, {SpanID: "db00000000000000", Name: "SELECT cards", Service: "payments"}}
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
		{"root000000000000", "POST /checkout", "gateway", 900, 0},
		{"chk0000000000000", "checkout", "checkout", 800, 1},
		{"pay0000000000000", "charge", "payments", 700, 2},
		{"db00000000000000", "SELECT cards", "payments", 650, 3},
		{"inv0000000000025", "reserve", "inventory", 26, 2},
	}
	if fmt.Sprint(out.Slowest[:5]) != fmt.Sprint(wantSlowest) {
		t.Fatalf("slowest = %+v, want %+v", out.Slowest[:5], wantSlowest)
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"trace_id"`, `"spans"`, `"spans_returned"`, `"truncated"`, `"services"`, `"duration_ms"`, `"root"`, `"errors"`, `"slowest"`, `"depth"`, `"links"`, `"linked_traces"`} {
		if !strings.Contains(string(body), key) {
			t.Errorf("JSON lacks %s: %s", key, body)
		}
	}
	if strings.Contains(string(body), "status_message") {
		t.Errorf("JSON carries an empty status_message: %s", body)
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
		release, err := r.deps.Runner.AcquireUpstream(context.Background())
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
	if want := "Read trace stats on tempo: 42 spans, p95 120.4 ms"; res.Step != want {
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
