package elasticsearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const searchPath = "/logs-*/_search"

var queryRange = datasource.TimeRange{From: rangeEnd.Add(-time.Hour), To: rangeEnd}

func searchServer(t *testing.T, body string) (*fakeServer, *elasticsearch.Datasource) {
	t.Helper()
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, body)
	return f, serve(t, f)
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want %q: %v", want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("body mismatch\n got: %s\nwant: %s", got, want)
	}
}

func hitsOf(n int) string {
	hits := make([]string, n)
	for i := range hits {
		hits[i] = fmt.Sprintf(`{"_index":"logs-shop-2026.11","_id":"id%d","_source":{"@timestamp":"2026-11-06T11:59:59.%09dZ","message":"line %d"}}`, i, i, i)
	}
	return `{"timed_out":false,"_shards":{"failed":0},"hits":{"total":{"value":` + fmt.Sprint(n) + `,"relation":"eq"},"hits":[` + strings.Join(hits, ",") + `]}}`
}

const (
	rangeFilter   = `{"range":{"@timestamp":{"gte":"2026-11-06T11:00:00Z","lt":"2026-11-06T12:00:00Z","format":"strict_date_optional_time_nanos"}}}`
	matchAll      = `{"match_all":{}}`
	defaultSort   = `[{"@timestamp":{"order":"desc","numeric_type":"date_nanos","unmapped_type":"date"}},{"_doc":"desc"}]`
	histogramBody = `{"count":{"date_histogram":{"field":"@timestamp","fixed_interval":"1m","min_doc_count":0,"extended_bounds":{"min":1793962800000,"max":1793966400000}}}}`
)

func TestBuildBodyForKQLInstant(t *testing.T) {
	f, ds := searchServer(t, hitsOf(1))
	q := datasource.Query{Text: "service.name:checkout", Limits: datasource.Limits{MaxRows: 1000}}
	if _, err := ds.QueryInstant(context.Background(), q, queryRange); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"size": 1000, "track_total_hits": 10000, "timeout": "30000ms", "sort": `+defaultSort+`,
		"query": {"bool": {"filter": [`+rangeFilter+`, {"match": {"service.name": "checkout"}}]}}
	}`)
}

func TestBuildBodyCapsTheLinesAtTheLogLineLimit(t *testing.T) {
	f, ds := searchServer(t, hitsOf(1))
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: ""}, queryRange); err != nil {
		t.Fatal(err)
	}
	body := searchBody(t, f.last(t))
	if body["size"] != 5000.0 {
		t.Fatalf("size = %v, want 5000", body["size"])
	}
}

func TestBuildBodyForEmptyBodiesIsMatchAll(t *testing.T) {
	for _, text := range []string{"", "   ", "lucene:", "kql:  ", `{}`, `{"size": 5}`} {
		f, ds := searchServer(t, hitsOf(0))
		if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, queryRange); err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		filter := searchBody(t, f.last(t))["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any)
		if len(filter) != 2 || !reflect.DeepEqual(filter[1], map[string]any{"match_all": map[string]any{}}) {
			t.Errorf("%q: filter = %v, want the time range and match_all", text, filter)
		}
	}
}

func TestBuildBodyForLuceneRangeIsADateHistogram(t *testing.T) {
	f, ds := searchServer(t, `{"hits":{"hits":[]},"aggregations":{"count":{"buckets":[]}}}`)
	if _, err := ds.QueryRange(context.Background(), datasource.Query{Text: "lucene:log.level:error"}, queryRange, time.Minute); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"size": 0, "track_total_hits": false, "timeout": "30000ms", "aggs": `+histogramBody+`,
		"query": {"bool": {"filter": [`+rangeFilter+`, {"query_string": {"query": "log.level:error", "analyze_wildcard": true}}]}}
	}`)
}

func TestBuildBodyForDSLWithAggregationsForcesSizeZero(t *testing.T) {
	f, ds := searchServer(t, `{"hits":{"total":{"value":0,"relation":"eq"},"hits":[]},"aggregations":{}}`)
	text := `{"size": 50, "sort": [{"x": "asc"}], "query": {"term": {"service.name": "checkout"}}, "aggs": {"by_service": {"terms": {"field": "service.name"}}}}`
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, queryRange); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"size": 0, "track_total_hits": true, "timeout": "30000ms",
		"aggs": {"by_service": {"terms": {"field": "service.name"}}},
		"query": {"bool": {"filter": [`+rangeFilter+`, {"term": {"service.name": "checkout"}}]}}
	}`)
}

func TestBuildBodyForDSLWithoutAggregationsKeepsSortAndCapsSize(t *testing.T) {
	f, ds := searchServer(t, hitsOf(1))
	text := `{"size": 99999, "sort": [{"x": "asc"}], "_source": ["message"], "fields": ["a"], "runtime_mappings": {"r": {"type": "keyword"}}}`
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text, Limits: datasource.Limits{MaxRows: 300}}, queryRange); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"size": 300, "track_total_hits": 10000, "timeout": "30000ms", "sort": [{"x": "asc"}],
		"_source": ["message"], "fields": ["a"], "runtime_mappings": {"r": {"type": "keyword"}},
		"query": {"bool": {"filter": [`+rangeFilter+`, `+matchAll+`]}}
	}`)
}

func TestBuildBodyRefusesInvalidBodiesBeforeAnyRequest(t *testing.T) {
	for _, tc := range []struct{ name, text, message string }{
		{"unknown key", `{"query": {}, "post_filter": {}}`, `unsupported key "post_filter" in the query body`},
		{"unknown key after nesting", `{"query": {"bool": {"x": [1, 2]}}, "from": 3}`, `unsupported key "from" in the query body`},
		{"syntax error", `{"query": }`, "query body is not valid JSON"},
		{"truncated", `{"query": {"match_all": {}}`, "query body is not valid JSON"},
		{"trailing data", `{} {}`, "unexpected data after the query body"},
		{"kql syntax", `service.name:(a or`, "at offset"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := searchServer(t, hitsOf(0))
			_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: tc.text}, queryRange)
			if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error = %v, want ErrInvalidQuery containing %q", err, tc.message)
			}
			if n := f.count(searchPath); n != 0 {
				t.Fatalf("%d search requests, want none", n)
			}
		})
	}
}

func TestQueryInstantReturnsLinesWithNanosecondTime(t *testing.T) {
	_, ds := searchServer(t, hitsReply)
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "message:payment"}, queryRange)
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames = %v, err = %v", frames, err)
	}
	f := frames[0]
	names := make([]string, len(f.Fields))
	for i, field := range f.Fields {
		names[i] = field.Name
	}
	want := []string{"Time", "TimeNs", "Body", "level", "ID", "Index", "service", "trace_id", "span_id", "Fields"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	wantNs := fmt.Sprint(time.Date(2026, 11, 6, 11, 59, 59, 123456789, time.UTC).UnixNano())
	if f.Fields[1].Values[0] != wantNs {
		t.Fatalf("TimeNs = %v, want %s", f.Fields[1].Values[0], wantNs)
	}
}

func TestQueryInstantRoundsTimeToMillisecondsOnADateField(t *testing.T) {
	reply := `{"hits":{"hits":[{"_index":"logs-shop-2026.11","_id":"a","_source":{"event":{"ingested":"2026-11-06T11:59:59.123456789Z"},"message":"m"}}]}}`
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, reply)
	ds := serve(t, f, func(o *elasticsearch.Options) { o.TimeField = "event.ingested" })
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{}, queryRange)
	if err != nil {
		t.Fatal(err)
	}
	wantNs := fmt.Sprint(time.Date(2026, 11, 6, 11, 59, 59, 123000000, time.UTC).UnixNano())
	if got := frames[0].Fields[1].Values[0]; got != wantNs {
		t.Fatalf("TimeNs = %v, want %s", got, wantNs)
	}
}

func TestQueryInstantMeta(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		limit int
		want  datasource.FrameMeta
	}{
		{"fewer rows than the limit", hitsOf(3), 1000, datasource.FrameMeta{RowsTotal: 3}},
		{"rows equal the limit", hitsOf(1000), 1000, datasource.FrameMeta{Truncated: true, RowsTotal: 1000}},
		{"timed out", strings.Replace(hitsOf(2), `"timed_out":false`, `"timed_out":true`, 1), 1000, datasource.FrameMeta{Partial: true, RowsTotal: 2}},
		{"failed shards", strings.Replace(hitsOf(2), `"failed":0`, `"failed":2`, 1), 1000, datasource.FrameMeta{Partial: true, RowsTotal: 2}},
		{"capped total", strings.Replace(strings.Replace(hitsOf(2), `"eq"`, `"gte"`, 1), `"value":2`, `"value":10000`, 1), 1000, datasource.FrameMeta{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ds := searchServer(t, tc.reply)
			frames, err := ds.QueryInstant(context.Background(), datasource.Query{Limits: datasource.Limits{MaxRows: tc.limit}}, queryRange)
			if err != nil {
				t.Fatal(err)
			}
			if frames[0].Meta == nil || *frames[0].Meta != tc.want {
				t.Fatalf("meta = %+v, want %+v", frames[0].Meta, tc.want)
			}
		})
	}
}

func TestQueryRangeCountsOverTime(t *testing.T) {
	reply := `{"hits":{"hits":[]},"aggregations":{"count":{"buckets":[
		{"key_as_string":"2026-11-06T11:00:00.000Z","key":1793962800000,"doc_count":2},
		{"key_as_string":"2026-11-06T11:01:00.000Z","key":1793962860000,"doc_count":0},
		{"key_as_string":"2026-11-06T11:02:00.000Z","key":1793962920000,"doc_count":5}]}}}`
	f, ds := searchServer(t, reply)
	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: "service.name:checkout"}, queryRange, time.Minute)
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames = %v, err = %v", frames, err)
	}
	frame := frames[0]
	if frame.Name != "count" || frame.Fields[0].Name != "Time" || frame.Fields[1].Name != "Value" {
		t.Fatalf("frame = %+v", frame)
	}
	if !reflect.DeepEqual(frame.Fields[1].Values, []any{2.0, 0.0, 5.0}) {
		t.Fatalf("values = %v", frame.Fields[1].Values)
	}
	if frame.Fields[0].Values[2] != time.UnixMilli(1793962920000).UTC() {
		t.Fatalf("times = %v", frame.Fields[0].Values)
	}
	aggs := searchBody(t, f.last(t))["aggs"]
	assertJSONEqual(t, mustJSON(t, aggs), histogramBody)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestQueryRangeStepBecomesAnElasticsearchInterval(t *testing.T) {
	for _, tc := range []struct {
		step time.Duration
		want string
	}{
		{500 * time.Millisecond, "1s"},
		{1500 * time.Millisecond, "2s"},
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Second, "90s"},
		{2 * time.Hour, "2h"},
		{24 * time.Hour, "1d"},
	} {
		f, ds := searchServer(t, `{"hits":{"hits":[]},"aggregations":{"count":{"buckets":[]}}}`)
		r := datasource.TimeRange{From: rangeEnd.Add(-48 * time.Hour), To: rangeEnd}
		if tc.step < time.Minute {
			r.From = rangeEnd.Add(-time.Hour)
		}
		if _, err := ds.QueryRange(context.Background(), datasource.Query{}, r, tc.step); err != nil {
			t.Fatalf("%v: %v", tc.step, err)
		}
		hist := searchBody(t, f.last(t))["aggs"].(map[string]any)["count"].(map[string]any)["date_histogram"].(map[string]any)
		if hist["fixed_interval"] != tc.want {
			t.Errorf("step %v: fixed_interval = %v, want %s", tc.step, hist["fixed_interval"], tc.want)
		}
	}
}

func TestQueryRangeRefusesTooManyBucketsAndTinySteps(t *testing.T) {
	f, ds := searchServer(t, `{}`)
	long := datasource.TimeRange{From: rangeEnd.Add(-7 * 24 * time.Hour), To: rangeEnd}
	if _, err := ds.QueryRange(context.Background(), datasource.Query{}, long, time.Second); !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("too many buckets: %v", err)
	}
	if _, err := ds.QueryRange(context.Background(), datasource.Query{Limits: datasource.Limits{MaxPoints: 10}}, queryRange, time.Minute); !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("max points: %v", err)
	}
	if _, err := ds.QueryRange(context.Background(), datasource.Query{}, queryRange, time.Microsecond); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("tiny step: %v", err)
	}
	if _, err := ds.QueryRange(context.Background(), datasource.Query{}, datasource.TimeRange{}, time.Minute); err == nil {
		t.Fatal("empty range was accepted")
	}
	if n := f.count(searchPath); n != 0 {
		t.Fatalf("%d search requests, want none", n)
	}
}

func TestQueryRangeOnADSLWithoutAggregationsReturnsLines(t *testing.T) {
	_, ds := searchServer(t, hitsOf(2))
	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: `{"query": {"match_all": {}}}`}, queryRange, time.Minute)
	if err != nil || len(frames) != 1 || frames[0].Fields[0].Name != "Time" || frames[0].Rows() != 2 {
		t.Fatalf("frames = %+v, err = %v", frames, err)
	}
}

func TestQueryExpandsMacrosInEveryForm(t *testing.T) {
	f, ds := searchServer(t, `{"hits":{"hits":[]},"aggregations":{"h":{"buckets":[]}}}`)
	vars := []datasource.Variable{{Name: "svc", Values: []string{"checkout", `pay"ment`}}}
	dsl := `{"aggs": {"h": {"date_histogram": {"field": "@timestamp", "fixed_interval": "$__interval"}}}, "query": {"query_string": {"query": "service.name:${svc:pipe}"}}}`
	if _, err := ds.QueryRange(context.Background(), datasource.Query{Text: dsl, Variables: vars}, queryRange, 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	body := f.last(t).body
	if !strings.Contains(body, `"fixed_interval":"300s"`) || !strings.Contains(body, `service.name:checkout|pay\"ment`) {
		t.Fatalf("macros not expanded: %s", body)
	}
}

func TestQueryUpstreamErrorsKeepTheirKind(t *testing.T) {
	f, ds := searchServer(t, `{}`)
	f.fixed(http.MethodPost, searchPath, http.StatusBadRequest, esError("query_shard_exception", "failed to create query"))
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "lucene:a:["}, queryRange); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("error = %v", err)
	}
}

func TestQueryOnAnIndexWithoutTheTimeFieldAsADateIsRefused(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, hitsOf(0))
	ds := serve(t, f, func(o *elasticsearch.Options) { o.TimeField = "message" })
	if _, err := ds.QueryInstant(context.Background(), datasource.Query{}, queryRange); !errors.Is(err, datasource.ErrNoTimeField) {
		t.Fatalf("error = %v, want ErrNoTimeField", err)
	}
}
