package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func (hn *harness) traceStats(uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal trace stats body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/trace-stats", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(auth.KindServiceAccount))
	return hn.serve(req, "alice")
}

func traceStatsBody(query string) map[string]any {
	return map[string]any{"query": query, "from": volumeFrom, "to": volumeTo}
}

func TestTraceStatsAnswersTheDocumentedShape(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	p50, p95, p99 := 33.1, 120.4, 410.0
	tempoDS.onStats = func(_ context.Context, req datasource.TraceStatsRequest) (datasource.TraceStats, error) {
		return datasource.TraceStats{
			Step: req.Step, Partial: true, Spans: 42, Errors: 1,
			Buckets: []datasource.TraceBucket{
				{Start: req.Range.From, Spans: 40, Errors: 1, P50: &p50, P95: &p95, P99: &p99},
				{Start: req.Range.From.Add(req.Step)},
			},
			Histogram: []datasource.DurationBucket{{LeMS: 0.131072, Count: 5}},
		}, nil
	}
	rec := hn.traceStats("tempo", map[string]any{"query": `{ resource.service.name = "checkout" }`, "from": volumeFrom, "to": volumeTo, "buckets": 30})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	want := `{"step_ms":120000,"partial":true,"spans":42,"errors":1,"buckets":[{"start":"2025-10-09T08:54:00Z","spans":40,"errors":1,"p50_ms":33.1,"p95_ms":120.4,"p99_ms":410},{"start":"2025-10-09T08:56:00Z","spans":0,"errors":0,"p50_ms":null,"p95_ms":null,"p99_ms":null}],"histogram":[{"le_ms":0.131072,"count":5}]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
	calls := tempoDS.statsCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	call := calls[0]
	if call.Query != `{ resource.service.name = "checkout" }` || call.Step != 2*time.Minute || call.Limits != queryproxy.DraftLimits || call.User.ID != "alice" {
		t.Fatalf("call = %+v", call)
	}
	if !call.Range.From.Equal(volumeStart) || call.Range.To.Sub(call.Range.From) != time.Hour {
		t.Fatalf("range = %v..%v, want the aligned hour from %v", call.Range.From, call.Range.To, volumeStart)
	}
	if hn.heldSlots() != 0 {
		t.Fatal("a slot stayed held")
	}
}

func TestTraceStatsWithNothingMatchedKeepsEmptyArrays(t *testing.T) {
	hn, _ := newTraceHarness(t)
	rec := hn.traceStats("tempo", traceStatsBody(""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	want := `{"step_ms":0,"partial":false,"spans":0,"errors":0,"buckets":[],"histogram":[]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestTraceStatsRejectsInvalidRequests(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := traceStatsBody("{}")
		edit(body)
		return body
	}
	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{"buckets zero", with(func(b map[string]any) { b["buckets"] = 0 }), "invalid_request"},
		{"buckets over 200", with(func(b map[string]any) { b["buckets"] = 201 }), "invalid_request"},
		{"unknown field", with(func(b map[string]any) { b["step"] = "1m" }), "invalid_request"},
		{"missing query", with(func(b map[string]any) { delete(b, "query") }), "invalid_request"},
		{"query over 64 KiB", with(func(b map[string]any) { b["query"] = strings.Repeat("x", 64<<10+1) }), "query_too_long"},
		{"missing to", with(func(b map[string]any) { delete(b, "to") }), "invalid_request"},
		{"from after to", with(func(b map[string]any) { b["from"], b["to"] = volumeTo, volumeFrom }), "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := hn.traceStats("tempo", tt.body)
			expectError(t, rec, http.StatusBadRequest, tt.code)
			if ref := decodeError(t, rec).Ref; ref != "" {
				t.Fatalf("error carries ref %q", ref)
			}
		})
	}
	if calls := len(tempoDS.statsCalls()); calls != 0 {
		t.Fatalf("%d upstream calls for invalid requests", calls)
	}
}

func TestTraceStatsRefusesOtherDatasourceTypesAndUnknownUIDs(t *testing.T) {
	hn, _ := newTraceHarness(t)
	rec := hn.traceStats("prom", traceStatsBody("{}"))
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := decodeError(t, rec).Error; got != traceStatsSourcesMessage {
		t.Fatalf("error = %q", got)
	}
	rec = hn.traceStats("missing", traceStatsBody("{}"))
	expectError(t, rec, http.StatusNotFound, "not_found")
	if got := decodeError(t, rec).Error; got != "datasource not found" {
		t.Fatalf("error = %q", got)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestTraceStatsMapsUpstreamErrorsLikeExplore(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	tests := []struct {
		kind   error
		status int
		code   string
	}{
		{datasource.ErrInvalidQuery, http.StatusUnprocessableEntity, "query_invalid"},
		{datasource.ErrUnavailable, http.StatusBadGateway, "datasource_unavailable"},
		{datasource.ErrTimeout, http.StatusGatewayTimeout, "query_timeout"},
	}
	for _, tc := range tests {
		tempoDS.onStats = func(context.Context, datasource.TraceStatsRequest) (datasource.TraceStats, error) {
			return datasource.TraceStats{}, datasource.Errorf(tc.kind, "upstream said no")
		}
		rec := hn.traceStats("tempo", traceStatsBody("{}"))
		expectError(t, rec, tc.status, tc.code)
		if ref := decodeError(t, rec).Ref; ref != "" {
			t.Fatalf("error carries ref %q", ref)
		}
	}
}

func TestTraceStatsAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, tempoDS := newTraceHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	expectError(t, hn.traceStats("tempo", traceStatsBody("{}")), http.StatusTooManyRequests, "too_many_requests")
	if calls := len(tempoDS.statsCalls()); calls != 0 {
		t.Fatalf("%d upstream calls while every slot is held", calls)
	}
}
