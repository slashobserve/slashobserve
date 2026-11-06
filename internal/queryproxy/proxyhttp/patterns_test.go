package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
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

type patternSource struct {
	*lineSource
	onPatterns func(req datasource.PatternRequest) ([]datasource.NativePattern, error)

	mu       sync.Mutex
	patterns []datasource.PatternRequest
}

func (d *patternSource) LogPatterns(_ context.Context, req datasource.PatternRequest) ([]datasource.NativePattern, error) {
	d.mu.Lock()
	d.patterns = append(d.patterns, req)
	d.mu.Unlock()
	if d.onPatterns == nil {
		return nil, nil
	}
	return d.onPatterns(req)
}

func (d *patternSource) patternRequests() []datasource.PatternRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.patterns)
}

func newPatternsHarness(t *testing.T) (*harness, *patternSource, *lineSource) {
	t.Helper()
	hn := newHarness(t)
	logs := &patternSource{lineSource: &lineSource{typedDatasource: &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.Loki}}}
	ch := &lineSource{typedDatasource: &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.ClickHouse}}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "logs", Datasource: logs},
		datasource.Instance{UID: "ch", Datasource: ch},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, logs, ch
}

func (hn *harness) logPatterns(uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal log-patterns body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/log-patterns", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(auth.KindUser))
	return hn.serve(req, "alice")
}

func patternsBody(query string) map[string]any {
	return map[string]any{"query": query, "from": volumeFrom, "to": volumeTo}
}

func decodePatterns(t *testing.T, rec *httptest.ResponseRecorder) queryproxy.LogPatterns {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body queryproxy.LogPatterns
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("patterns body: %v", err)
	}
	return body
}

func patternSampleFrames(texts ...string) []datasource.Frame {
	f := datasource.Frame{Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime},
		{Name: "Body", Type: datasource.FieldString},
	}}
	for i, text := range texts {
		f.Fields[0].Values = append(f.Fields[0].Values, volumeStart.Add(time.Hour-time.Duration(i)*time.Second))
		f.Fields[1].Values = append(f.Fields[1].Values, text)
	}
	return []datasource.Frame{f}
}

func TestLogPatternsComeFromLokiWhenItsPatternAPIAnswers(t *testing.T) {
	hn, logs, _ := newPatternsHarness(t)
	logs.onPatterns = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		return []datasource.NativePattern{
			{Pattern: "GET /a status=<*>", Samples: []datasource.PatternSample{{At: volumeStart.Add(2 * time.Minute), Count: 5}, {At: volumeStart, Count: 7}}},
			{Pattern: "no samples in range"},
			{Pattern: "POST /checkout status=<*> duration_ms=<*>", Samples: []datasource.PatternSample{{At: volumeStart.Add(time.Minute), Count: 31}}},
			{Pattern: "a tie with GET", Samples: []datasource.PatternSample{{At: volumeStart, Count: 12}}},
		}, nil
	}
	body := patternsBody(`{app="shop"}`)
	body["limit"] = 2
	got := decodePatterns(t, hn.logPatterns("logs", body))
	if got.Source != "loki" || got.LinesSampled != 0 || got.LinesMatched != 55 || !got.Truncated || len(got.Patterns) != 2 {
		t.Fatalf("patterns = %+v", got)
	}
	first := got.Patterns[0]
	if first.Pattern != "POST /checkout status=<*> duration_ms=<*>" || first.Count != 31 || first.Sample != "" || first.First != "2025-10-09T08:55:00Z" || first.Last != "2025-10-09T08:55:00Z" {
		t.Fatalf("first = %+v", first)
	}
	if second := got.Patterns[1]; second.Pattern != "GET /a status=<*>" || second.Count != 12 || second.First != "2025-10-09T08:54:00Z" || second.Last != "2025-10-09T08:56:00Z" {
		t.Fatalf("second = %+v, want the pattern that sorts first among equal counts", second)
	}
	calls := logs.patternRequests()
	if len(calls) != 1 {
		t.Fatalf("%d pattern calls, want 1", len(calls))
	}
	req := calls[0]
	if req.Query.Text != `{app="shop"}` || req.Query.Limits != queryproxy.DraftLimits || req.Query.User.ID != "alice" || req.Step != time.Minute {
		t.Fatalf("request = %+v", req)
	}
	if !req.Range.From.Equal(volumeStart) || !req.Range.To.Equal(volumeStart.Add(time.Hour)) {
		t.Fatalf("range = %v to %v", req.Range.From, req.Range.To)
	}
	if logs.calls() != 0 {
		t.Fatalf("%d line reads beside a native answer", logs.calls())
	}
}

func TestLogPatternsAreComputedFromTheNewestLinesWhenLokiAnswersNotFound(t *testing.T) {
	hn, logs, _ := newPatternsHarness(t)
	logs.onPatterns = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		return nil, datasource.Errorf(datasource.ErrNotFound, "the pattern API is not enabled")
	}
	logs.onLines = func(datasource.LinesRequest) ([]datasource.Frame, error) {
		return patternSampleFrames("POST /checkout status=200 duration_ms=31", "POST /checkout status=500 duration_ms=900", "GET /health ok"), nil
	}
	body := patternsBody(`{app="shop"} |= "x"`)
	body["sample"] = 500
	got := decodePatterns(t, hn.logPatterns("logs", body))
	if got.Source != "server" || got.LinesSampled != 3 || got.LinesMatched != 3 || got.Truncated || len(got.Patterns) != 2 {
		t.Fatalf("patterns = %+v", got)
	}
	first := got.Patterns[0]
	if first.Pattern != "POST /checkout status=<*> duration_ms=<*>" || first.Count != 2 || first.Sample != "POST /checkout status=200 duration_ms=31" {
		t.Fatalf("first = %+v", first)
	}
	if first.Last != "2025-10-09T09:54:00Z" || first.First != "2025-10-09T09:53:59Z" {
		t.Fatalf("first = %+v, want the times of its newest and oldest line", first)
	}
	read := logs.side(datasource.Backward)
	if read.Limit != 500 || read.Query.Limits.MaxRows != 500 || read.Query.Text != `{app="shop"} |= "x"` || read.Direction != datasource.Backward {
		t.Fatalf("line read = %+v", read)
	}
	if read.Query.Limits.Timeout != queryproxy.DraftLimits.Timeout || read.Query.Limits.MaxSeries != queryproxy.DraftLimits.MaxSeries {
		t.Fatalf("line read limits = %+v, want the draft limits with only the rows lowered", read.Query.Limits)
	}
}

func TestLogPatternsOnClickHouseReadTheBodyColumnOfTheSample(t *testing.T) {
	hn, _, ch := newPatternsHarness(t)
	ch.onLines = func(datasource.LinesRequest) ([]datasource.Frame, error) {
		return []datasource.Frame{{Fields: []datasource.Field{
			{Name: "Timestamp", Type: datasource.FieldTime, Values: []any{volumeStart, volumeStart.Add(time.Second)}},
			{Name: "ServiceName", Type: datasource.FieldString, Values: []any{"checkout", "checkout"}},
			{Name: "Body", Type: datasource.FieldString, Values: []any{"payment failed for order 11", "payment failed for order 12"}},
		}}}, nil
	}
	got := decodePatterns(t, hn.logPatterns("ch", patternsBody("SELECT Timestamp, ServiceName, Body FROM otel_logs")))
	if got.Source != "server" || got.LinesSampled != 2 || len(got.Patterns) != 1 || got.Patterns[0].Pattern != "payment failed for order <*>" || got.Patterns[0].Count != 2 {
		t.Fatalf("patterns = %+v", got)
	}
	if got.Patterns[0].Sample != "payment failed for order 12" || got.Patterns[0].Last != "2025-10-09T08:54:01Z" {
		t.Fatalf("pattern = %+v, want the newest row as its sample", got.Patterns[0])
	}
	if read := ch.side(datasource.Backward); read.Limit != 2000 || read.Query.Limits.MaxRows != 2000 {
		t.Fatalf("line read = %+v, want the default sample of 2000", read)
	}
	ch.onLines = func(datasource.LinesRequest) ([]datasource.Frame, error) {
		return []datasource.Frame{{Fields: []datasource.Field{
			{Name: "Timestamp", Type: datasource.FieldTime, Values: []any{volumeStart}},
			{Name: "ServiceName", Type: datasource.FieldString, Values: []any{"checkout"}},
		}}}, nil
	}
	rec := hn.logPatterns("ch", patternsBody("SELECT Timestamp, ServiceName FROM otel_logs"))
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	if got := decodeError(t, rec).Error; got != "invalid query: patterns need a query that selects Body" {
		t.Fatalf("error = %q", got)
	}
	ch.onLines = nil
	if got := decodePatterns(t, hn.logPatterns("ch", patternsBody("SELECT 1"))); got.Source != "server" || got.LinesSampled != 0 || got.Patterns == nil || len(got.Patterns) != 0 {
		t.Fatalf("an empty sample = %+v, want no patterns as []", got)
	}
}

func TestLogPatternsJSONKeys(t *testing.T) {
	hn, logs, _ := newPatternsHarness(t)
	logs.onPatterns = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		return []datasource.NativePattern{{Pattern: "x", Samples: []datasource.PatternSample{{At: volumeStart, Count: 1}}}}, nil
	}
	rec := hn.logPatterns("logs", patternsBody(`{app="shop"}`))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if got := slices.Sorted(maps.Keys(raw)); !slices.Equal(got, []string{"lines_matched", "lines_sampled", "patterns", "source", "truncated"}) {
		t.Fatalf("keys = %v", got)
	}
	var patterns []map[string]json.RawMessage
	if err := json.Unmarshal(raw["patterns"], &patterns); err != nil || len(patterns) != 1 {
		t.Fatalf("patterns %s: %v", raw["patterns"], err)
	}
	if got := slices.Sorted(maps.Keys(patterns[0])); !slices.Equal(got, []string{"count", "first", "last", "pattern", "sample"}) {
		t.Fatalf("pattern keys = %v", got)
	}
}

func TestLogPatternsRejectsInvalidRequests(t *testing.T) {
	hn, logs, _ := newPatternsHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := patternsBody(`{app="shop"}`)
		edit(body)
		return body
	}
	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{"missing query", with(func(b map[string]any) { delete(b, "query") }), "invalid_request"},
		{"query over 64 KiB", with(func(b map[string]any) { b["query"] = strings.Repeat("x", 64<<10+1) }), "query_too_long"},
		{"missing to", with(func(b map[string]any) { delete(b, "to") }), "invalid_request"},
		{"from after to", with(func(b map[string]any) { b["from"], b["to"] = volumeTo, volumeFrom }), "invalid_request"},
		{"limit zero", with(func(b map[string]any) { b["limit"] = 0 }), "invalid_request"},
		{"limit over 100", with(func(b map[string]any) { b["limit"] = 101 }), "invalid_request"},
		{"sample under 100", with(func(b map[string]any) { b["sample"] = 99 }), "invalid_request"},
		{"sample over 5000", with(func(b map[string]any) { b["sample"] = 5001 }), "invalid_request"},
		{"unknown field", with(func(b map[string]any) { b["fields"] = []string{"a"} }), "invalid_request"},
		{"unknown timezone", with(func(b map[string]any) { b["timezone"] = "Mars/Olympus" }), "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectError(t, hn.logPatterns("logs", tt.body), http.StatusBadRequest, tt.code)
		})
	}
	if n := len(logs.patternRequests()) + logs.calls(); n != 0 {
		t.Fatalf("%d datasource calls for invalid requests", n)
	}
}

func TestLogPatternsRefusesOtherDatasourceTypesAndUnknownUIDs(t *testing.T) {
	hn, _, _ := newPatternsHarness(t)
	rec := hn.logPatterns("prom", patternsBody("up"))
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := decodeError(t, rec).Error; got != "patterns read Loki and ClickHouse datasources" {
		t.Fatalf("error = %q", got)
	}
	expectError(t, hn.logPatterns("missing", patternsBody("up")), http.StatusNotFound, "not_found")
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestLogPatternsErrorsCarryNoRef(t *testing.T) {
	hn, logs, _ := newPatternsHarness(t)
	logs.onPatterns = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "patterns need a log query that starts with a stream selector")
	}
	rec := hn.logPatterns("logs", patternsBody(`rate({app="shop"}[1m])`))
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, has := raw["ref"]; has {
		t.Fatalf("error body carries ref: %s", rec.Body.String())
	}
	if logs.calls() != 0 {
		t.Fatalf("%d line reads after a query the pattern API refused", logs.calls())
	}
	logs.onPatterns = func(datasource.PatternRequest) ([]datasource.NativePattern, error) {
		return nil, datasource.Errorf(datasource.ErrNotFound, "the pattern API is not enabled")
	}
	logs.onLines = func(datasource.LinesRequest) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	expectError(t, hn.logPatterns("logs", patternsBody(`{app="shop"}`)), http.StatusBadGateway, "datasource_unavailable")
}

func TestLogPatternsAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, logs, _ := newPatternsHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.logPatterns("logs", patternsBody(`{app="shop"}`))
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if n := len(logs.patternRequests()); n != 0 {
		t.Fatalf("%d pattern calls while every slot is held", n)
	}
}
