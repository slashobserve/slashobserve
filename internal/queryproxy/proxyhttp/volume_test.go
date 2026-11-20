package proxyhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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

func newVolumeHarness(t *testing.T) (hn *harness, clickhouseDS *typedDatasource) {
	t.Helper()
	hn = newHarness(t)
	clickhouseDS = &typedDatasource{fakeDatasource: &fakeDatasource{}, kind: spec.ClickHouse}
	registry, err := datasource.NewRegistry(hn.h.logger,
		datasource.Instance{UID: "prom", Datasource: hn.ds},
		datasource.Instance{UID: "ch", Datasource: clickhouseDS},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	hn.runner = queryproxy.NewRunner(registry, hn.h.logger)
	hn.h.runner, hn.h.registry = hn.runner, registry
	return hn, clickhouseDS
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

func TestVolumeRejectsInvalidRequests(t *testing.T) {
	hn, clickhouseDS := newVolumeHarness(t)
	with := func(edit func(map[string]any)) map[string]any {
		body := volumeBody("SELECT 1")
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
			expectError(t, hn.volume("ch", tt.body), http.StatusBadRequest, tt.code)
		})
	}
	if calls := clickhouseDS.instantCalls(); len(calls) != 0 {
		t.Fatalf("%d upstream calls for invalid requests", len(calls))
	}
}

func TestVolumeRefusesOtherDatasourceTypesAndUnknownUIDs(t *testing.T) {
	hn, _ := newVolumeHarness(t)
	rec := hn.volume("prom", volumeBody("up"))
	expectError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := decodeError(t, rec).Error; got != "volume reads Loki, ClickHouse, Elasticsearch and OpenSearch datasources" {
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

func TestVolumeFillsClickHouseBucketsFromTheGroupedTable(t *testing.T) {
	hn, clickhouseDS := newVolumeHarness(t)
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

func TestVolumeCountsClickHouseOverTheRequestedRangeOnly(t *testing.T) {
	hn, clickhouseDS := newVolumeHarness(t)
	clickhouseDS.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) { return nil, nil }
	body := volumeBody("SELECT Timestamp, SeverityText FROM otel_logs WHERE $__timeFilter(Timestamp)")
	from, to := volumeStart.Add(20*time.Second), volumeStart.Add(time.Hour+20*time.Second)
	body["from"], body["to"] = strconv.FormatInt(from.UnixMilli(), 10), strconv.FormatInt(to.UnixMilli(), 10)
	got := decodeVolume(t, hn.volume("ch", body))
	calls := clickhouseDS.instantCalls()
	if len(calls) != 1 || !calls[0].rng.From.Equal(from) || !calls[0].rng.To.Equal(to) {
		t.Fatalf("instant calls %+v, want one over %v to %v", calls, from, to)
	}
	if len(got.Buckets) != 61 || got.Buckets[0].Start != "2025-10-09T08:54:00Z" {
		t.Fatalf("%d buckets from %s", len(got.Buckets), got.Buckets[0].Start)
	}
}

func TestVolumeAnswersInternalWhenTheClickHouseTableLacksItsFields(t *testing.T) {
	hn, clickhouseDS := newVolumeHarness(t)
	clickhouseDS.onInstant = func(context.Context, datasource.TimeRange) ([]datasource.Frame, error) {
		return []datasource.Frame{{Fields: []datasource.Field{{Name: "Time", Type: datasource.FieldTime, Values: []any{}}}}}, nil
	}
	expectError(t, hn.volume("ch", volumeBody("SELECT 1")), http.StatusInternalServerError, "internal")
}

func TestVolumeAnswersTooManyRequestsWhenEverySlotIsHeld(t *testing.T) {
	hn, clickhouseDS := newVolumeHarness(t)
	holdPrincipalSlots(t, hn, "alice")
	rec := hn.volume("ch", volumeBody("SELECT 1"))
	expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if calls := clickhouseDS.instantCalls(); len(calls) != 0 {
		t.Fatalf("%d upstream calls while every slot is held", len(calls))
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

func TestVolumeAsksACapabilityDatasourceToCountOnlyTheRequestedRange(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(req datasource.VolumeRequest) (*logvolume.Histogram, error) {
		return logvolume.New(req.Range.From, req.Step, req.Buckets()), nil
	}
	from, to := volumeStart.Add(20*time.Second), volumeStart.Add(time.Hour+20*time.Second)
	body := volumeBody(`service.name:checkout`)
	body["from"], body["to"] = strconv.FormatInt(from.UnixMilli(), 10), strconv.FormatInt(to.UnixMilli(), 10)
	decodeVolume(t, hn.volume("search", body))
	reqs := search.volumeRequests()
	if len(reqs) != 1 || !reqs[0].Counted.From.Equal(from) || !reqs[0].Counted.To.Equal(to) {
		t.Fatalf("requests %+v, want the count limited to %v to %v", reqs, from, to)
	}
}

func TestVolumeCountsTheTrailingPartialBucket(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(req datasource.VolumeRequest) (*logvolume.Histogram, error) {
		h := logvolume.New(req.Range.From, req.Step, req.Buckets())
		h.Add(req.Range.From.Add(60*req.Step), logvolume.Info, 5)
		return h, nil
	}
	body := volumeBody(`service.name:checkout`)
	body["from"] = strconv.FormatInt(volumeStart.Add(20*time.Second).UnixMilli(), 10)
	body["to"] = strconv.FormatInt(volumeStart.Add(time.Hour+20*time.Second).UnixMilli(), 10)
	got := decodeVolume(t, hn.volume("search", body))
	reqs := search.volumeRequests()
	if len(reqs) != 1 || !reqs[0].Range.To.Equal(volumeStart.Add(61*time.Minute)) {
		t.Fatalf("requests %+v", reqs)
	}
	if len(got.Buckets) != 61 || got.Total != 5 {
		t.Fatalf("%d buckets, total %d", len(got.Buckets), got.Total)
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

func TestVolumeJSONHasExactlyTheContractKeys(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(req datasource.VolumeRequest) (*logvolume.Histogram, error) {
		return logvolume.New(req.Range.From, req.Step, req.Buckets()), nil
	}
	rec := hn.volume("search", volumeBody(`*`))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if got := slices.Sorted(maps.Keys(raw)); !slices.Equal(got, []string{"buckets", "partial", "step_ms", "total"}) {
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

func TestVolumeSaysWhereAPartialCountStarts(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(req datasource.VolumeRequest) (*logvolume.Histogram, error) {
		h := logvolume.New(req.Range.From, req.Step, req.Buckets())
		h.Add(req.Range.From.Add(50*req.Step), logvolume.Info, 3)
		h.CountedOnlyFrom(req.Range.From.Add(40 * req.Step))
		return h, nil
	}
	body := decodeVolume(t, hn.volume("search", volumeBody(`*`)))
	if !body.Partial || body.CountedFrom != "2025-10-09T09:34:00Z" || body.Total != 3 {
		t.Fatalf("volume = %+v, want 3 lines counted from 09:34 and marked partial", body)
	}
}

func TestVolumeSpeaksOfTheQueryWhenACapabilityDatasourceRejectsIt(t *testing.T) {
	hn, search := newVolumeCapabilityHarness(t)
	search.onVolume = func(datasource.VolumeRequest) (*logvolume.Histogram, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "pipeline error: 'JSONParserErr' for series")
	}
	rec := hn.volume("search", volumeBody(`*`))
	expectError(t, rec, http.StatusUnprocessableEntity, "query_invalid")
	if got := decodeError(t, rec).Error; !strings.Contains(got, "add | __error__=\"\" after the parser") {
		t.Fatalf("error = %q", got)
	}
}

func TestVolumeAlignmentCoversTheWholeRangeAtEveryWidth(t *testing.T) {
	now := time.Date(2026, 11, 18, 19, 19, 31, 456_000_000, time.UTC)
	for _, span := range []time.Duration{time.Hour, 3 * time.Hour, 6 * time.Hour, 24 * time.Hour, 48 * time.Hour, 7 * 24 * time.Hour, 14 * 24 * time.Hour, 30 * 24 * time.Hour, 90 * 24 * time.Hour} {
		rng := datasource.TimeRange{From: now.Add(-span), To: now}
		al := volumeAlignment(rng, 60)
		if al.rng.From.After(rng.From) || al.rng.To.Before(rng.To) {
			t.Fatalf("%v: window %v to %v does not cover %v to %v", span, al.rng.From, al.rng.To, rng.From, rng.To)
		}
		if got := int(al.rng.To.Sub(al.rng.From) / al.step); got != al.maxDataPoints {
			t.Fatalf("%v: %d steps in the window but %d buckets", span, got, al.maxDataPoints)
		}
		if got := queryproxy.StepFor(al.rng, al.maxDataPoints); got != al.step {
			t.Fatalf("%v: the runner would step by %v, not %v", span, got, al.step)
		}
	}
}

func TestVolumeProblemSpeaksOfTheQueryNotOfTheWrapper(t *testing.T) {
	invalid := func(message string) error { return datasource.Errorf(datasource.ErrInvalidQuery, "%s", message) }
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"a parser failing on plain text", invalid(`pipeline error: 'JSONParserErr' for series: {app="x"}. Use a label filter to intentionally skip this error.`), "a parser stage failed on some lines (JSONParserErr)"},
		{"a wrapper parse position", invalid("parse error at line 0, col 1: syntax error: unexpected RANGE, expecting } or ,"), "could not read the query as written"},
		{"a ClickHouse wrapper position", invalid("failed at position 443 (*) ) GROUP BY Time"), "could not read the query as written"},
		{"a ClickHouse position in the volume query", invalid(`Syntax error: failed at line 3, column 9 of the query, near "GROUP"`), "could not read the query as written"},
		{"the end of the volume query", invalid("Syntax error: failed at the end of the query (line 4, column 2)"), "could not read the query as written"},
		{"the expanded time macro", invalid("Unknown table in WHERE fromUnixTimestamp64Milli(toInt64(1), 'UTC')"), "could not read the query as written"},
		{"a message that names only the query", invalid("the volume needs the query to select SeverityText"), "the volume needs the query to select SeverityText"},
		{"another kind of error", datasource.Errorf(datasource.ErrTimeout, "parse error at line 1, col 1"), "parse error at line 1, col 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := volumeProblem(tc.in)
			if !strings.Contains(got.Error(), tc.want) || strings.Contains(got.Error(), "line 0, col 1") && tc.name != "another kind of error" {
				t.Fatalf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
}
