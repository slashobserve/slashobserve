package elasticsearch_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

func categorizeFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/categorize.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func patternRequest() datasource.PatternRequest {
	return datasource.PatternRequest{Query: datasource.Query{Text: "service.name:checkout"}, Range: queryRange, Step: time.Minute}
}

func TestLogPatternsAsksForCategorizedTextOverTime(t *testing.T) {
	f, ds := searchServer(t, categorizeFixture(t))
	if _, err := ds.LogPatterns(context.Background(), patternRequest()); err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"size": 0, "track_total_hits": false, "timeout": "30000ms",
		"query": {"bool": {"filter": [`+rangeFilter+`, {"match": {"service.name": "checkout"}}]}},
		"aggs": {"patterns": {
			"categorize_text": {"field": "message", "size": 100, "similarity_threshold": 70, "max_matched_tokens": 5},
			"aggs": {"over_time": {"date_histogram": {"field": "@timestamp", "fixed_interval": "1m"}}}
		}}
	}`)
}

func TestLogPatternsMapsBucketsToPatternsWithTheirNonEmptySamples(t *testing.T) {
	_, ds := searchServer(t, categorizeFixture(t))
	got, err := ds.LogPatterns(context.Background(), patternRequest())
	if err != nil {
		t.Fatal(err)
	}
	at := func(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
	want := []datasource.NativePattern{
		{Pattern: "connection to refused after retry", Samples: []datasource.PatternSample{{At: at(1793959200000), Count: 3}, {At: at(1793960400000), Count: 7}}},
		{Pattern: "request handled in ms", Samples: []datasource.PatternSample{{At: at(1793959200000), Count: 4}, {At: at(1793959800000), Count: 6}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("patterns = %+v\nwant %+v", got, want)
	}
}

func TestLogPatternsAnswersNothingForAnEmptyResult(t *testing.T) {
	_, ds := searchServer(t, `{"hits":{"total":{"value":0,"relation":"eq"},"hits":[]},"aggregations":{"patterns":{"buckets":[]}}}`)
	got, err := ds.LogPatterns(context.Background(), patternRequest())
	if err != nil || len(got) != 0 {
		t.Fatalf("patterns = %v, err = %v", got, err)
	}
}

func TestLogPatternsFallsBackWhereTheClusterCannotCategorize(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"a build without the machine learning module", http.StatusBadRequest, esError("parsing_exception", "Unknown aggregation type [categorize_text]")},
		{"a license below platinum", http.StatusForbidden, `{"error":{"root_cause":[{"type":"security_exception","reason":"current license is non-compliant for [categorize-text-agg]","license.expired.feature":"categorize-text-agg"}],"type":"security_exception","reason":"current license is non-compliant for [categorize-text-agg]"},"status":403}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := searchServer(t, categorizeFixture(t))
			f.fixed(http.MethodPost, searchPath, tc.status, tc.body)
			if _, err := ds.LogPatterns(context.Background(), patternRequest()); !errors.Is(err, datasource.ErrNotFound) {
				t.Fatalf("error = %v, want not found", err)
			}
		})
	}
}

func TestLogPatternsKeepsOtherFailuresAsTheyAre(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"a rejected query", http.StatusBadRequest, esError("parsing_exception", "unknown query [bogus]"), datasource.ErrInvalidQuery},
		{"a bad credential", http.StatusForbidden, `{"error":{"root_cause":[{"type":"security_exception","reason":"action [indices:data/read/search] is unauthorized"}],"type":"security_exception","reason":"x"},"status":403}`, datasource.ErrAuthFailed},
		{"an unavailable cluster", http.StatusServiceUnavailable, `{}`, datasource.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := searchServer(t, categorizeFixture(t))
			f.fixed(http.MethodPost, searchPath, tc.status, tc.body)
			if _, err := ds.LogPatterns(context.Background(), patternRequest()); !errors.Is(err, tc.want) || errors.Is(err, datasource.ErrNotFound) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestLogPatternsOnOpenSearchFallsBackWithoutSearching(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodGet, "/", http.StatusOK, osRoot)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, categorizeFixture(t))
	ds := serve(t, f, func(o *elasticsearch.Options) { o.Flavor = elasticsearch.OpenSearch })
	if _, err := ds.LogPatterns(context.Background(), patternRequest()); !errors.Is(err, datasource.ErrNotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
	if f.count(searchPath) != 0 {
		t.Fatal("OpenSearch was asked to categorize")
	}
}

func TestLogPatternsRefusesWhatItCannotReadBeforeSearching(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  datasource.PatternRequest
	}{
		{"a body with aggregations", datasource.PatternRequest{Query: datasource.Query{Text: `{"aggs": {"x": {"terms": {"field": "service.name"}}}}`}, Range: queryRange, Step: time.Minute}},
		{"an empty range", datasource.PatternRequest{Range: datasource.TimeRange{From: queryRange.To, To: queryRange.From}, Step: time.Minute}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := searchServer(t, categorizeFixture(t))
			if _, err := ds.LogPatterns(context.Background(), tc.req); err == nil || errors.Is(err, datasource.ErrNotFound) {
				t.Fatalf("error = %v, want a refusal", err)
			}
			if f.count(searchPath) != 0 {
				t.Fatal("a refused request reached the search endpoint")
			}
		})
	}
}
