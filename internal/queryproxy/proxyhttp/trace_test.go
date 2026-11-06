package proxyhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	traceFixtureID    = "0af7651916cd43dd8448eb211c80319c"
	traceFixtureSpans = 25000
)

var traceFixtureStart = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

type traceCall struct {
	id       string
	rng      datasource.TimeRange
	maxSpans int
}

type traceDatasource struct {
	*fakeDatasource
	kind    spec.DatasourceType
	onTrace func(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error)
	onStats func(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error)

	mu     sync.Mutex
	traces []traceCall
	stats  []datasource.TraceStatsRequest
}

func (d *traceDatasource) Type() string { return string(d.kind) }

func (d *traceDatasource) Trace(ctx context.Context, id string, r datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
	d.mu.Lock()
	d.traces = append(d.traces, traceCall{id: id, rng: r, maxSpans: maxSpans})
	d.mu.Unlock()
	return d.onTrace(ctx, id, r, maxSpans)
}

func (d *traceDatasource) TraceStats(ctx context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
	d.mu.Lock()
	d.stats = append(d.stats, req)
	d.mu.Unlock()
	return d.onStats(ctx, req)
}

func (d *traceDatasource) traceCalls() []traceCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]traceCall(nil), d.traces...)
}

func (d *traceDatasource) statsCalls() []datasource.TraceStatsRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]datasource.TraceStatsRequest(nil), d.stats...)
}

func newTraceHarness(t *testing.T) (hn *harness, tempoDS *traceDatasource) {
	t.Helper()
	hn = newHarness(t)
	tempoDS = &traceDatasource{
		fakeDatasource: &fakeDatasource{},
		kind:           spec.Tempo,
		onTrace: func(_ context.Context, _ string, _ datasource.TimeRange, maxSpans int) (datasource.Frame, error) {
			return spanFixture(traceFixtureSpans, maxSpans), nil
		},
		onStats: func(context.Context, datasource.TraceStatsRequest) (datasource.TraceStats, error) {
			return datasource.TraceStats{}, nil
		},
	}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "tempo", Datasource: tempoDS},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, tempoDS
}

func spanFixture(total, budget int) datasource.Frame {
	n := min(total, budget)
	names := []string{"TraceId", "SpanId", "ParentSpanId", "SpanName", "ServiceName", "StartTime", "Duration", "StatusCode", "SpanAttributes", "ResourceAttributes", "SpanKind", "SpanEvents"}
	f := datasource.Frame{Fields: make([]datasource.Field, len(names))}
	for i, name := range names {
		typ := datasource.FieldString
		switch name {
		case "StartTime":
			typ = datasource.FieldTime
		case "Duration":
			typ = datasource.FieldNumber
		}
		f.Fields[i] = datasource.Field{Name: name, Type: typ, Values: make([]any, n)}
	}
	for i := range n {
		row := []any{
			traceFixtureID, fmt.Sprintf("%016x", i+1), "", "GET /checkout", "checkout",
			traceFixtureStart.Add(time.Duration(i) * time.Microsecond), float64(1_000_000), "Unset", "{}", "{}", "Server", "[]",
		}
		for c := range row {
			f.Fields[c].Values[i] = row[c]
		}
	}
	if n < total {
		f.Meta = &datasource.FrameMeta{Truncated: true, RowsTotal: total}
	}
	return f
}

func (hn *harness) trace(kind auth.PrincipalKind, uid, id, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasources/"+uid+"/traces/"+id+query, nil)
	req.Header.Set("X-Kind", string(kind))
	return hn.serve(req, "alice")
}

func TestTraceAnswersTheWholeTraceUnderTheSpanBudget(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	rec := hn.trace(auth.KindServiceAccount, "tempo", strings.ToUpper(traceFixtureID), "?from=now-6h&to=now&timezone=Asia/Kolkata")
	body := decodeData(t, rec)
	meta := body.Meta
	if meta.Kind != queryproxy.KindInstant || meta.Cached || meta.Rows != datasource.TraceSpanBudget || meta.RowsTotal != traceFixtureSpans || !meta.Truncated || meta.Partial {
		t.Fatalf("meta = %+v", meta)
	}
	if len(meta.Queries) != 1 || meta.Queries[0].Ref != "A" || meta.Queries[0].Kind != queryproxy.KindInstant {
		t.Fatalf("queries = %+v", meta.Queries)
	}
	var frames []struct {
		Ref    string `json:"ref"`
		Fields []struct {
			Name   string `json:"name"`
			Type   string `json:"type"`
			Values []any  `json:"values"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(body.Frames, &frames); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Ref != "A" || len(frames[0].Fields) != 12 {
		t.Fatalf("frames = %d, ref %q, fields %d", len(frames), frames[0].Ref, len(frames[0].Fields))
	}
	if got := frames[0].Fields[10]; got.Name != "SpanKind" || len(got.Values) != datasource.TraceSpanBudget || got.Values[0] != "Server" {
		t.Fatalf("SpanKind field = %q with %d values", got.Name, len(got.Values))
	}
	calls := tempoDS.traceCalls()
	if len(calls) != 1 || calls[0].id != traceFixtureID || calls[0].maxSpans != datasource.TraceSpanBudget {
		t.Fatalf("calls = %+v", calls)
	}
	if span := calls[0].rng.To.Sub(calls[0].rng.From); span < 6*time.Hour-time.Minute || span > 6*time.Hour+time.Minute {
		t.Fatalf("range spans %s, want about 6h", span)
	}
	if hn.heldSlots() != 0 {
		t.Fatal("a slot stayed held")
	}
}

func TestTraceDefaultsToTheLastDay(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	tempoDS.onTrace = func(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
		return spanFixture(3, datasource.TraceSpanBudget), nil
	}
	body := decodeData(t, hn.trace(auth.KindUser, "tempo", traceFixtureID[:16], ""))
	if body.Meta.Rows != 3 || body.Meta.RowsTotal != 0 || body.Meta.Truncated {
		t.Fatalf("meta = %+v", body.Meta)
	}
	calls := tempoDS.traceCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	if span := calls[0].rng.To.Sub(calls[0].rng.From); span < 24*time.Hour-time.Minute || span > 24*time.Hour+time.Minute {
		t.Fatalf("range spans %s, want about 24h", span)
	}
}

func TestTraceRejectsBadRequests(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	tempoDS.onTrace = func(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
		return datasource.Frame{}, datasource.Errorf(datasource.ErrNotFound, "trace not found")
	}
	tests := []struct {
		name    string
		uid, id string
		query   string
		status  int
		code    string
		message string
	}{
		{name: "short id", uid: "tempo", id: "abc", status: http.StatusBadRequest, code: "invalid_request", message: traceIDMessage},
		{name: "non hex id", uid: "tempo", id: strings.Repeat("g", 32), status: http.StatusBadRequest, code: "invalid_request"},
		{name: "unknown datasource", uid: "nope", id: traceFixtureID, status: http.StatusNotFound, code: "not_found", message: "datasource not found"},
		{name: "a Prometheus datasource", uid: "prom", id: traceFixtureID, status: http.StatusBadRequest, code: "invalid_request", message: traceSourcesMessage},
		{name: "a bad time", uid: "tempo", id: traceFixtureID, query: "?from=yesterday-ish", status: http.StatusBadRequest, code: "invalid_request"},
		{name: "an unknown parameter", uid: "tempo", id: traceFixtureID, query: "?limit=5", status: http.StatusBadRequest, code: "invalid_request"},
		{name: "a missing trace", uid: "tempo", id: traceFixtureID, status: http.StatusNotFound, code: "not_found", message: traceNotFoundMessage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := hn.trace(auth.KindUser, tc.uid, tc.id, tc.query)
			expectError(t, rec, tc.status, tc.code)
			body := decodeError(t, rec)
			if body.Ref != "" {
				t.Fatalf("error carries ref %q", body.Ref)
			}
			if tc.message != "" && body.Error != tc.message {
				t.Fatalf("error = %q, want %q", body.Error, tc.message)
			}
		})
	}
	if hn.heldSlots() != 0 {
		t.Fatal("a slot stayed held")
	}
}

func TestTraceMapsUpstreamErrorsLikeExplore(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	tests := []struct {
		kind   error
		status int
		code   string
	}{
		{datasource.ErrLimitExceeded, http.StatusUnprocessableEntity, "limit_exceeded"},
		{datasource.ErrUnavailable, http.StatusBadGateway, "datasource_unavailable"},
		{datasource.ErrTimeout, http.StatusGatewayTimeout, "query_timeout"},
	}
	for _, tc := range tests {
		tempoDS.onTrace = func(context.Context, string, datasource.TimeRange, int) (datasource.Frame, error) {
			return datasource.Frame{}, datasource.Errorf(tc.kind, "upstream said no")
		}
		expectError(t, hn.trace(auth.KindUser, "tempo", traceFixtureID, ""), tc.status, tc.code)
	}
}

func TestTraceAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.trace(auth.KindUser, "tempo", traceFixtureID, "")
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if calls := len(tempoDS.traceCalls()); calls != 0 {
		t.Fatalf("%d upstream calls while every slot is held", calls)
	}
}
