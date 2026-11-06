package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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
	traceID = "0af7651916cd43dd8448eb211c80319c"
	spanID  = "b7ad6b7169203331"
)

type traceLogsDatasource struct {
	*fakeDatasource
	kind    spec.DatasourceType
	answer  func(req datasource.TraceLogsRequest) ([]datasource.Frame, error)
	mu      sync.Mutex
	asked   []datasource.TraceLogsRequest
	panicky bool
}

func (d *traceLogsDatasource) Type() string { return string(d.kind) }

func (d *traceLogsDatasource) TraceLogs(_ context.Context, req datasource.TraceLogsRequest) ([]datasource.Frame, error) {
	d.mu.Lock()
	d.asked = append(d.asked, req)
	d.mu.Unlock()
	if d.panicky {
		panic("trace logs exploded")
	}
	if d.answer != nil {
		return d.answer(req)
	}
	return nil, nil
}

func (d *traceLogsDatasource) requests() []datasource.TraceLogsRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.asked)
}

func newTraceLogsHarness(t *testing.T) (hn *harness, lokiDS, clickhouseDS *traceLogsDatasource) {
	t.Helper()
	hn = newHarness(t)
	lokiDS = &traceLogsDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.Loki}
	clickhouseDS = &traceLogsDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.ClickHouse}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "logs", Datasource: lokiDS},
		datasource.Instance{UID: "ch", Datasource: clickhouseDS},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, lokiDS, clickhouseDS
}

func (hn *harness) traceLogs(uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal trace logs body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/trace-logs", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(auth.KindServiceAccount))
	return hn.serve(req, "alice")
}

func traceLogsBody() map[string]any {
	return map[string]any{"trace_id": traceID, "from": exploreFrom, "to": exploreTo}
}

func TestTraceLogsAnswerTheLinesOfATraceAsPanelData(t *testing.T) {
	hn, lokiDS, clickhouseDS := newTraceLogsHarness(t)
	for _, ds := range []*traceLogsDatasource{lokiDS, clickhouseDS} {
		ds.answer = func(datasource.TraceLogsRequest) ([]datasource.Frame, error) {
			frames := logFrames()
			frames[0].Meta = &datasource.FrameMeta{Truncated: true}
			return frames, nil
		}
	}
	for _, uid := range []string{"logs", "ch"} {
		body := traceLogsBody()
		body["span_id"] = spanID
		body["service"] = "checkout"
		body["limit"] = 2
		got := decodeData(t, hn.traceLogs(uid, body))
		if got.Meta.Kind != queryproxy.KindInstant || len(got.Meta.Queries) != 1 || got.Meta.Queries[0].Ref != "A" || got.Meta.StepMS != 0 || got.Meta.Cached {
			t.Fatalf("%s: meta = %+v, want an uncached instant result with one query A", uid, got.Meta)
		}
		if !got.Meta.Truncated || got.Meta.Rows != 3 || got.Meta.Oldest != "2026-11-06T11:59:58.5Z" || got.Meta.Newest != "2026-11-06T12:00:00.000000001Z" {
			t.Fatalf("%s: meta = %+v, want truncated with 3 rows and the line edges", uid, got.Meta)
		}
		if got.Meta.From != "2025-10-09T08:53:20.123Z" || got.Meta.To != "2025-10-09T08:54:20.456Z" {
			t.Fatalf("%s: range = %s to %s, want the exact bounds", uid, got.Meta.From, got.Meta.To)
		}
		var frames []struct {
			Ref    string            `json:"ref"`
			Labels map[string]string `json:"labels"`
		}
		if err := json.Unmarshal(got.Frames, &frames); err != nil {
			t.Fatal(err)
		}
		if len(frames) != 2 || frames[0].Ref != "A" || frames[0].Labels["service_name"] != "checkout" {
			t.Fatalf("%s: frames = %+v, want two stream frames under ref A", uid, frames)
		}
	}
	for _, ds := range []*traceLogsDatasource{lokiDS, clickhouseDS} {
		asked := ds.requests()
		if len(asked) != 1 {
			t.Fatalf("%s asked %d times, want once", ds.kind, len(asked))
		}
		req := asked[0]
		if req.TraceID != traceID || req.SpanID != spanID || req.Service != "checkout" || req.Limit != 2 {
			t.Fatalf("%s: request = %+v", ds.kind, req)
		}
		if req.Range.From.UnixMilli() != 1760000000123 || req.Range.To.UnixMilli() != 1760000060456 {
			t.Fatalf("%s: range = %v to %v, want the exact bounds", ds.kind, req.Range.From, req.Range.To)
		}
		if req.User.ID != "alice" {
			t.Fatalf("%s: user = %+v, want the caller", ds.kind, req.User)
		}
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestTraceLogsDefaultTheLimitAndAnswerAnEmptyResultWithNoFrames(t *testing.T) {
	hn, lokiDS, _ := newTraceLogsHarness(t)
	got := decodeData(t, hn.traceLogs("logs", traceLogsBody()))
	if string(got.Frames) != "[]" {
		t.Fatalf("frames = %s, want []", got.Frames)
	}
	if got.Meta.Truncated || got.Meta.Rows != 0 || got.Meta.Oldest != "" || got.Meta.Newest != "" || len(got.Meta.Queries) != 1 {
		t.Fatalf("meta = %+v, want an empty result with one query", got.Meta)
	}
	asked := lokiDS.requests()
	if len(asked) != 1 || asked[0].Limit != 500 || asked[0].SpanID != "" || asked[0].Service != "" {
		t.Fatalf("request = %+v, want the 500 line default with no span or service", asked)
	}
}

func TestTraceLogsRejectInvalidBodies(t *testing.T) {
	hn, lokiDS, _ := newTraceLogsHarness(t)
	long := strings.Repeat("s", 201)
	tests := []struct {
		name string
		body map[string]any
		want string
	}{
		{"missing trace id", map[string]any{"from": exploreFrom, "to": exploreTo}, "trace_id must be 16 or 32 hex characters"},
		{"short trace id", map[string]any{"trace_id": "abc", "from": exploreFrom, "to": exploreTo}, "trace_id must be 16 or 32 hex characters"},
		{"non-hex trace id", map[string]any{"trace_id": "zz" + traceID[2:], "from": exploreFrom, "to": exploreTo}, "trace_id must be 16 or 32 hex characters"},
		{"short span id", map[string]any{"trace_id": traceID, "span_id": "b7ad", "from": exploreFrom, "to": exploreTo}, "span_id must be 16 hex characters"},
		{"empty service", map[string]any{"trace_id": traceID, "service": "", "from": exploreFrom, "to": exploreTo}, "service must be from 1 to 200 bytes"},
		{"long service", map[string]any{"trace_id": traceID, "service": long, "from": exploreFrom, "to": exploreTo}, "service must be from 1 to 200 bytes"},
		{"missing to", map[string]any{"trace_id": traceID, "from": exploreFrom}, "from and to are required"},
		{"limit 0", map[string]any{"trace_id": traceID, "from": exploreFrom, "to": exploreTo, "limit": 0}, "limit must be from 1 to 1000"},
		{"limit 1001", map[string]any{"trace_id": traceID, "from": exploreFrom, "to": exploreTo, "limit": 1001}, "limit must be from 1 to 1000"},
		{"from after to", map[string]any{"trace_id": traceID, "from": exploreTo, "to": exploreFrom}, "invalid request: from must be before to"},
		{"unknown field", map[string]any{"trace_id": traceID, "from": exploreFrom, "to": exploreTo, "query": "x"}, "request body must be valid JSON with known fields"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := hn.traceLogs("logs", tt.body)
			expectError(t, rec, http.StatusBadRequest, "invalid_request")
			if got := decodeError(t, rec); got.Error != tt.want || got.Ref != "" {
				t.Fatalf("error = %+v, want %q with no ref", got, tt.want)
			}
		})
	}
	if asked := lokiDS.requests(); len(asked) != 0 {
		t.Fatalf("the datasource was asked for a rejected body: %+v", asked)
	}
}

func TestTraceLogsRefuseOtherDatasourceTypesAndUnknownUIDs(t *testing.T) {
	hn, _, _ := newTraceLogsHarness(t)
	rec := hn.traceLogs("prom", traceLogsBody())
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := decodeError(t, rec).Error; got != "trace logs read Loki and ClickHouse datasources" {
		t.Fatalf("error = %q", got)
	}
	expectError(t, hn.traceLogs("missing", traceLogsBody()), http.StatusNotFound, "not_found")
	if ranges, instants, _, _ := hn.ds.counts(); ranges+instants != 0 {
		t.Fatalf("%d upstream calls", ranges+instants)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestTraceLogsMapDatasourceErrorsLikeExplore(t *testing.T) {
	hn, lokiDS, _ := newTraceLogsHarness(t)
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid query", datasource.Errorf(datasource.ErrInvalidQuery, "bad selector"), http.StatusUnprocessableEntity, "query_invalid"},
		{"limit exceeded", datasource.Errorf(datasource.ErrLimitExceeded, "too many"), http.StatusUnprocessableEntity, "limit_exceeded"},
		{"unavailable", datasource.Errorf(datasource.ErrUnavailable, "down"), http.StatusBadGateway, "datasource_unavailable"},
		{"timeout", datasource.Errorf(datasource.ErrTimeout, "slow"), http.StatusGatewayTimeout, "query_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lokiDS.answer = func(datasource.TraceLogsRequest) ([]datasource.Frame, error) { return nil, tt.err }
			rec := hn.traceLogs("logs", traceLogsBody())
			expectError(t, rec, tt.status, tt.code)
			if got := decodeError(t, rec).Ref; got != "" {
				t.Fatalf("ref = %q, want none", got)
			}
		})
	}
	lokiDS.answer, lokiDS.panicky = nil, true
	expectError(t, hn.traceLogs("logs", traceLogsBody()), http.StatusInternalServerError, "internal")
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestTraceLogsAnswerTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, lokiDS, _ := newTraceLogsHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.traceLogs("logs", traceLogsBody())
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if asked := lokiDS.requests(); len(asked) != 0 {
		t.Fatalf("%d upstream calls while every slot is held", len(asked))
	}

	hn, lokiDS, _ = newTraceLogsHarness(t)
	hn.h.lintTimeout = 20 * time.Millisecond
	holdUpstreamSlots(t, hn)
	rec = hn.traceLogs("logs", traceLogsBody())
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if asked := lokiDS.requests(); len(asked) != 0 {
		t.Fatalf("%d upstream calls while every upstream slot is held", len(asked))
	}
}
