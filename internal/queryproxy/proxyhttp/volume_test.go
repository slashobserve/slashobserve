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
	"github.com/slashobserve/slashobserve/internal/logvolume"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	volumeFromMS = 1760000040000
	volumeFrom   = "1760000040000"
	volumeTo     = "1760003640000"
)

var volumeStart = time.UnixMilli(volumeFromMS).UTC()

type typedDatasource struct {
	*fakeDatasource
	kind spec.DatasourceType

	mu       sync.Mutex
	instants []instantCall
}

func (d *typedDatasource) Type() string { return string(d.kind) }

func (d *typedDatasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	d.mu.Lock()
	d.instants = append(d.instants, instantCall{query: q, rng: r})
	d.mu.Unlock()
	return d.fakeDatasource.QueryInstant(ctx, q, r)
}

func (d *typedDatasource) instantCalls() []instantCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.instants)
}

func newVolumeHarness(t *testing.T) (hn *harness, lokiDS, clickhouseDS *typedDatasource) {
	t.Helper()
	hn = newHarness(t)
	lokiDS = &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.Loki}
	clickhouseDS = &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.ClickHouse}
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

func (hn *harness) volume(uid string, body map[string]any) *httptest.ResponseRecorder {
	hn.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		hn.t.Fatalf("marshal volume body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasources/"+uid+"/log-volume", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kind", string(auth.KindUser))
	return hn.serve(req, "alice")
}

func volumeBody(query string) map[string]any {
	return map[string]any{"query": query, "from": volumeFrom, "to": volumeTo}
}

func decodeVolume(t *testing.T, rec *httptest.ResponseRecorder) volumeJSON {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body volumeJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("volume body: %v", err)
	}
	return body
}

func series(labels map[string]string, at []time.Time, values []float64) datasource.Frame {
	f := datasource.Frame{Labels: labels, Fields: []datasource.Field{
		{Name: "Time", Type: datasource.FieldTime},
		{Name: "Value", Type: datasource.FieldNumber},
	}}
	for i := range at {
		f.Fields[0].Values = append(f.Fields[0].Values, at[i])
		f.Fields[1].Values = append(f.Fields[1].Values, datasource.Number(values[i]))
	}
	return f
}

func TestVolumeRejectsInvalidRequests(t *testing.T) {
	hn, lokiDS, _ := newVolumeHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := volumeBody(`{app="shop"}`)
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
		{"unknown field", with(func(b map[string]any) { b["level_column"] = "level" }), "invalid_request"},
		{"missing query", with(func(b map[string]any) { delete(b, "query") }), "invalid_request"},
		{"query over 64 KiB", with(func(b map[string]any) { b["query"] = strings.Repeat("x", 64<<10+1) }), "query_too_long"},
		{"missing from", with(func(b map[string]any) { delete(b, "from") }), "invalid_request"},
		{"from after to", with(func(b map[string]any) { b["from"], b["to"] = volumeTo, volumeFrom }), "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectError(t, hn.volume("logs", tt.body), http.StatusBadRequest, tt.code)
		})
	}
	if ranges, _, _, _ := lokiDS.counts(); ranges != 0 {
		t.Fatalf("%d upstream calls for invalid requests", ranges)
	}
}

func TestVolumeRefusesOtherDatasourceTypesAndUnknownUIDs(t *testing.T) {
	hn, _, _ := newVolumeHarness(t)
	rec := hn.volume("prom", volumeBody("up"))
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := decodeError(t, rec).Error; got != "volume reads Loki and ClickHouse datasources" {
		t.Fatalf("error = %q", got)
	}
	expectError(t, hn.volume("missing", volumeBody("up")), http.StatusNotFound, "not_found")
	if ranges, instants, _, _ := hn.ds.counts(); ranges+instants != 0 {
		t.Fatalf("%d upstream calls", ranges+instants)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestVolumeCountsLokiSeriesByLevelOverTheAlignedRange(t *testing.T) {
	hn, lokiDS, _ := newVolumeHarness(t)
	lokiDS.onRange = func(_ context.Context, r datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
		at := []time.Time{r.From, r.From.Add(step), r.From.Add(2 * step), r.To}
		return []datasource.Frame{
			series(map[string]string{"level": "error"}, at, []float64{9, 2, 3, 4}),
			series(map[string]string{"detected_level": "info"}, at, []float64{9, 10, 20, 30}),
			series(nil, at[1:2], []float64{1}),
		}, nil
	}
	body := decodeVolume(t, hn.volume("logs", volumeBody(`{app="shop"} |= "x"`)))
	text := "sum by (level, lvl, severity_text, severity, detected_level) (count_over_time(\n{app=\"shop\"} |= \"x\"\n[60000ms]))"
	if step := lokiDS.stepFor(text); step != time.Minute {
		t.Fatalf("step for the derived query = %v, want 1m (saw %+v)", step, lokiDS.lastQuery())
	}
	lokiDS.mu.Lock()
	rng := lokiDS.rangeRange
	lokiDS.mu.Unlock()
	if !rng.From.Equal(volumeStart) || !rng.To.Equal(volumeStart.Add(time.Hour)) {
		t.Fatalf("range = %v to %v", rng.From, rng.To)
	}
	if lokiDS.lastQuery().Limits != queryproxy.DraftLimits {
		t.Fatalf("limits = %+v", lokiDS.lastQuery().Limits)
	}
	if body.StepMS != 60000 || len(body.Buckets) != 60 {
		t.Fatalf("step_ms = %d, %d buckets", body.StepMS, len(body.Buckets))
	}
	want := map[int]bucketJSON{
		0:  {Start: "2025-10-09T08:54:00Z", Error: 2, Info: 10, Other: 1},
		1:  {Start: "2025-10-09T08:55:00Z", Error: 3, Info: 20},
		59: {Start: "2025-10-09T09:53:00Z", Error: 4, Info: 30},
	}
	for i, b := range body.Buckets {
		w, ok := want[i]
		if !ok {
			w = bucketJSON{Start: volumeStart.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)}
		}
		if b != w {
			t.Errorf("bucket %d = %+v, want %+v", i, b, w)
		}
	}
	if body.Total != 70 {
		t.Fatalf("total = %d, want 70", body.Total)
	}
}

func TestVolumeBucketsCapTheBucketCount(t *testing.T) {
	hn, lokiDS, _ := newVolumeHarness(t)
	lokiDS.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return nil, nil
	}
	body := volumeBody(`{app="shop"}`)
	body["buckets"] = 200
	got := decodeVolume(t, hn.volume("logs", body))
	if got.StepMS != 18000 || len(got.Buckets) != 200 || got.Total != 0 {
		t.Fatalf("step_ms = %d, %d buckets, total %d", got.StepMS, len(got.Buckets), got.Total)
	}
}

func TestVolumeRefusesALokiMetricQuery(t *testing.T) {
	hn, lokiDS, _ := newVolumeHarness(t)
	rec := hn.volume("logs", volumeBody(`rate({app="shop"}[1m])`))
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	if got := decodeError(t, rec).Error; !strings.Contains(got, "the volume needs a log query that starts with a stream selector") {
		t.Fatalf("error = %q", got)
	}
	if ranges, _, _, _ := lokiDS.counts(); ranges != 0 {
		t.Fatalf("%d upstream calls", ranges)
	}
}

func TestVolumeFillsClickHouseBucketsFromTheGroupedTable(t *testing.T) {
	hn, _, clickhouseDS := newVolumeHarness(t)
	clickhouseDS.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return []datasource.Frame{{Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{volumeStart, volumeStart, volumeStart.Add(2 * time.Minute), volumeStart.Add(time.Hour)}},
			{Name: "Level", Type: datasource.FieldString, Values: []any{"error", "warning", "info", "info"}},
			{Name: "count", Type: datasource.FieldNumber, Values: []any{datasource.Number(2), datasource.Number(1), datasource.Number(40), datasource.Number(99)}},
		}}}, nil
	}
	body := decodeVolume(t, hn.volume("ch", volumeBody("SELECT Timestamp, SeverityText FROM otel_logs WHERE $__timeFilter(Timestamp) ORDER BY Timestamp DESC LIMIT 1000")))
	calls := clickhouseDS.instantCalls()
	if len(calls) != 1 {
		t.Fatalf("%d instant calls, want 1", len(calls))
	}
	if q := calls[0].query.Text; !strings.HasPrefix(q, "SELECT toStartOfInterval(`Timestamp`, INTERVAL 60 SECOND) AS Time") || strings.Contains(q, "LIMIT 1000") {
		t.Fatalf("query = %q", q)
	}
	if r := calls[0].rng; !r.From.Equal(volumeStart) || !r.To.Equal(volumeStart.Add(time.Hour)) {
		t.Fatalf("range = %v to %v", r.From, r.To)
	}
	if body.Total != 43 || len(body.Buckets) != 60 {
		t.Fatalf("total = %d, %d buckets", body.Total, len(body.Buckets))
	}
	if b := body.Buckets[0]; b.Error != 2 || b.Warning != 1 || b.Info != 0 {
		t.Fatalf("bucket 0 = %+v", b)
	}
	if b := body.Buckets[2]; b.Info != 40 {
		t.Fatalf("bucket 2 = %+v", b)
	}
}

func TestVolumeAnswersInternalWhenTheClickHouseTableLacksItsFields(t *testing.T) {
	hn, _, clickhouseDS := newVolumeHarness(t)
	clickhouseDS.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return []datasource.Frame{{Fields: []datasource.Field{{Name: "Time", Type: datasource.FieldTime, Values: []any{}}}}}, nil
	}
	expectError(t, hn.volume("ch", volumeBody("SELECT 1")), http.StatusInternalServerError, "internal")
}

func TestVolumeJSONHasExactlyTheContractKeys(t *testing.T) {
	hn, lokiDS, _ := newVolumeHarness(t)
	lokiDS.onRange = func(context.Context, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return nil, nil
	}
	rec := hn.volume("logs", volumeBody(`{app="shop"}`))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if got := slices.Sorted(maps.Keys(raw)); !slices.Equal(got, []string{"buckets", "step_ms", "total"}) {
		t.Fatalf("keys = %v", got)
	}
	var buckets []map[string]any
	if err := json.Unmarshal(raw["buckets"], &buckets); err != nil || len(buckets) == 0 {
		t.Fatalf("buckets %s: %v", raw["buckets"], err)
	}
	if got := slices.Sorted(maps.Keys(buckets[0])); !slices.Equal(got, []string{"error", "info", "other", "start", "warning"}) {
		t.Fatalf("bucket keys = %v", got)
	}
}

func TestVolumeAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, lokiDS, _ := newVolumeHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.volume("logs", volumeBody(`{app="shop"}`))
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if ranges, _, _, _ := lokiDS.counts(); ranges != 0 {
		t.Fatalf("%d upstream calls while every slot is held", ranges)
	}
}

type volumeCapabilityDatasource struct {
	*fakeDatasource
	onVolume func(req datasource.VolumeRequest) (*logvolume.Histogram, error)

	mu       sync.Mutex
	requests []datasource.VolumeRequest
}

func (d *volumeCapabilityDatasource) Type() string { return string(spec.Elasticsearch) }

func (d *volumeCapabilityDatasource) LogVolume(_ context.Context, req datasource.VolumeRequest) (*logvolume.Histogram, error) {
	d.mu.Lock()
	d.requests = append(d.requests, req)
	d.mu.Unlock()
	return d.onVolume(req)
}

func (d *volumeCapabilityDatasource) volumeRequests() []datasource.VolumeRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.requests)
}

func newVolumeCapabilityHarness(t *testing.T) (*harness, *volumeCapabilityDatasource) {
	t.Helper()
	hn := newHarness(t)
	search := &volumeCapabilityDatasource{fakeDatasource: &fakeDatasource{}}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "search", Datasource: search},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, search
}

func TestVolumeAsksACapabilityDatasourceForTheAlignedWindow(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(req datasource.VolumeRequest) (*logvolume.Histogram, error) {
		h := logvolume.New(req.Range.From, req.Step, req.Buckets())
		h.Add(req.Range.From, logvolume.Error, 4)
		h.Add(req.Range.From.Add(2*req.Step), logvolume.Info, 7)
		h.Add(req.Range.From.Add(59*req.Step), logvolume.Other, 1)
		return h, nil
	}
	body := decodeVolume(t, hn.volume("search", volumeBody(`service.name:checkout`)))
	reqs := search.volumeRequests()
	if len(reqs) != 1 {
		t.Fatalf("%d volume calls, want 1", len(reqs))
	}
	got := reqs[0]
	if got.Step != time.Minute || !got.Range.From.Equal(volumeStart) || !got.Range.To.Equal(volumeStart.Add(time.Hour)) {
		t.Fatalf("step = %v, range = %v to %v", got.Step, got.Range.From, got.Range.To)
	}
	if got.Query.Text != "service.name:checkout" || got.Query.Limits != queryproxy.DraftLimits {
		t.Fatalf("query = %+v", got.Query)
	}
	if body.StepMS != 60000 || len(body.Buckets) != 60 || body.Total != 12 {
		t.Fatalf("step_ms = %d, %d buckets, total %d", body.StepMS, len(body.Buckets), body.Total)
	}
	if b := body.Buckets[0]; b.Error != 4 || b.Start != "2025-10-09T08:54:00Z" {
		t.Fatalf("bucket 0 = %+v", b)
	}
	if b := body.Buckets[2]; b.Info != 7 {
		t.Fatalf("bucket 2 = %+v", b)
	}
	if b := body.Buckets[59]; b.Other != 1 {
		t.Fatalf("bucket 59 = %+v", b)
	}
	if ranges, instants, _, _ := search.counts(); ranges+instants != 0 {
		t.Fatalf("%d runner queries on the capability path", ranges+instants)
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
}

func TestVolumeBucketsCapTheBucketCountOnACapabilityDatasource(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(req datasource.VolumeRequest) (*logvolume.Histogram, error) {
		return logvolume.New(req.Range.From, req.Step, req.Buckets()), nil
	}
	body := volumeBody(`*`)
	body["buckets"] = 200
	got := decodeVolume(t, hn.volume("search", body))
	if got.StepMS != 18000 || len(got.Buckets) != 200 {
		t.Fatalf("step_ms = %d, %d buckets", got.StepMS, len(got.Buckets))
	}
}

func TestVolumeMapsCapabilityErrorsAndHoldsOneUpstreamSlot(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(datasource.VolumeRequest) (*logvolume.Histogram, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "the volume needs a KQL, Lucene or filter-only query body")
	}
	expectError(t, hn.volume("search", volumeBody(`{"aggs":{}}`)), http.StatusUnprocessableEntity, "query_invalid")
	search.onVolume = func(datasource.VolumeRequest) (*logvolume.Histogram, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	expectError(t, hn.volume("search", volumeBody(`*`)), http.StatusBadGateway, "datasource_unavailable")
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d slots still held", held)
	}
	holdPrincipalSlots(t, hn, "alice")
	before := len(search.volumeRequests())
	expectError(t, hn.volume("search", volumeBody(`*`)), http.StatusTooManyRequests, "too_many_requests")
	if n := len(search.volumeRequests()); n != before {
		t.Fatalf("%d volume calls while every slot is held", n-before)
	}
}
