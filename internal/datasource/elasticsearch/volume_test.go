package elasticsearch_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/logvolume"
)

const (
	volumeStep   = 20 * time.Minute
	volumeBucket = `{"date_histogram":{"field":"@timestamp","fixed_interval":"20m","min_doc_count":0,"extended_bounds":{"min":1793962800000,"max":1793966400000}}`
)

func volumeFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/volume.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func volumeRequest(text string) datasource.VolumeRequest {
	return datasource.VolumeRequest{Query: datasource.Query{Text: text}, Range: queryRange, Step: volumeStep}
}

func TestLogVolumeCountsEachLevelKindPerBucket(t *testing.T) {
	f, ds := searchServer(t, volumeFixture(t))
	hist, err := ds.LogVolume(context.Background(), volumeRequest("service.name:checkout"))
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"timeout": "30000ms", "size": 0, "track_total_hits": false,
		"query": {"bool": {"filter": [`+rangeFilter+`, {"match": {"service.name": "checkout"}}]}},
		"aggs": {"volume": `+volumeBucket+`, "aggs": {"level": {"terms": {"field": "log.level", "size": 20}}}}}
	}`)
	got := hist.Volume()
	if got.StepMS != volumeStep.Milliseconds() || got.Total != 23 {
		t.Fatalf("step_ms = %d, total = %d, want %d and 23", got.StepMS, got.Total, volumeStep.Milliseconds())
	}
	want := []logvolume.Bucket{
		{Start: rangeEnd.Add(-time.Hour), Error: 3, Warning: 2, Info: 4, Other: 1},
		{Start: rangeEnd.Add(-40 * time.Minute), Info: 5},
		{Start: rangeEnd.Add(-20 * time.Minute), Error: 2, Warning: 1, Info: 1, Other: 4},
	}
	if len(got.Buckets) != len(want) {
		t.Fatalf("%d buckets, want %d", len(got.Buckets), len(want))
	}
	for i, b := range got.Buckets {
		if b != want[i] {
			t.Errorf("bucket %d = %+v, want %+v", i, b, want[i])
		}
	}
}

func TestLogVolumeWithoutALevelFieldCountsEverythingAsOther(t *testing.T) {
	const caps = `{"fields":{
		"@timestamp":{"date_nanos":{"type":"date_nanos","metadata_field":false,"searchable":true,"aggregatable":true}},
		"message":{"text":{"type":"text","metadata_field":false,"searchable":true,"aggregatable":false}}
	}}`
	const reply = `{"timed_out":false,"_shards":{"failed":0},"aggregations":{"volume":{"buckets":[
		{"key":1793962800000,"doc_count":7},{"key":1793964000000,"doc_count":0},{"key":1793965200000,"doc_count":2}
	]}}}`
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_field_caps", http.StatusOK, caps)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, reply)
	hist, err := serve(t, f).LogVolume(context.Background(), volumeRequest(""))
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f.last(t).body, `{
		"timeout": "30000ms", "size": 0, "track_total_hits": false,
		"query": {"bool": {"filter": [`+rangeFilter+`, `+matchAll+`]}},
		"aggs": {"volume": `+volumeBucket+`}}
	}`)
	got := hist.Volume()
	if got.Total != 9 || got.Buckets[0].Other != 7 || got.Buckets[2].Other != 2 {
		t.Fatalf("volume = %+v", got)
	}
	for _, b := range got.Buckets {
		if b.Error+b.Warning+b.Info != 0 {
			t.Fatalf("bucket %+v counts a level", b)
		}
	}
}

func TestLogVolumeUsesTheConfiguredLevelField(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, volumeFixture(t))
	ds := serve(t, f, func(o *elasticsearch.Options) { o.Fields.Level = "service.name" })
	if _, err := ds.LogVolume(context.Background(), volumeRequest("")); err != nil {
		t.Fatal(err)
	}
	aggs := searchBody(t, f.last(t))["aggs"].(map[string]any)["volume"].(map[string]any)["aggs"].(map[string]any)
	terms := aggs["level"].(map[string]any)["terms"].(map[string]any)
	if terms["field"] != "service.name" {
		t.Fatalf("terms = %v", terms)
	}
}

func TestLogVolumeKeepsADSLQueryClauseAndRefusesDSLAggregations(t *testing.T) {
	f, ds := searchServer(t, volumeFixture(t))
	body := `{"query":{"term":{"service.name":"checkout"}},"sort":[{"@timestamp":"asc"}],"size":5}`
	if _, err := ds.LogVolume(context.Background(), volumeRequest(body)); err != nil {
		t.Fatal(err)
	}
	sent := searchBody(t, f.last(t))
	if _, ok := sent["sort"]; ok || sent["size"] != 0.0 {
		t.Fatalf("body = %v, want no sort and size 0", sent)
	}
	assertJSONEqual(t, mustJSON(t, sent["query"]), `{"bool":{"filter":[`+rangeFilter+`,{"term":{"service.name":"checkout"}}]}}`)

	before := f.count(searchPath)
	for _, key := range []string{"aggs", "aggregations"} {
		_, err := ds.LogVolume(context.Background(), volumeRequest(`{"`+key+`":{"by":{"terms":{"field":"log.level"}}}}`))
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "the volume needs a KQL, Lucene or filter-only query body") {
			t.Fatalf("%s: error = %v", key, err)
		}
	}
	if f.count(searchPath) != before {
		t.Fatal("a refused query reached the search endpoint")
	}
}

func TestLogVolumeRefusesPartialAnswersAndBadSteps(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		step  time.Duration
		want  error
	}{
		{"timed out", `{"timed_out":true,"_shards":{"failed":0},"aggregations":{"volume":{"buckets":[]}}}`, volumeStep, datasource.ErrTimeout},
		{"failed shard", `{"timed_out":false,"_shards":{"failed":1},"aggregations":{"volume":{"buckets":[]}}}`, volumeStep, datasource.ErrUnavailable},
		{"sub-second step", volumeReply, 500 * time.Millisecond, datasource.ErrInvalidQuery},
		{"fractional second step", volumeReply, 1500 * time.Millisecond, datasource.ErrInvalidQuery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ds := searchServer(t, tc.reply)
			req := volumeRequest("")
			req.Step = tc.step
			if _, err := ds.LogVolume(context.Background(), req); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

const volumeReply = `{"timed_out":false,"_shards":{"failed":0},"aggregations":{"volume":{"buckets":[]}}}`

func TestLogVolumeAnswersAnEmptyHistogramWhenNoIndexMatches(t *testing.T) {
	_, ds := searchServer(t, `{"timed_out":false,"_shards":{"failed":0},"hits":{"total":{"value":0,"relation":"eq"},"hits":[]}}`)
	hist, err := ds.LogVolume(context.Background(), volumeRequest(""))
	if err != nil {
		t.Fatal(err)
	}
	if got := hist.Volume(); got.Total != 0 || len(got.Buckets) != 3 {
		t.Fatalf("volume = %+v", got)
	}
}

func TestLogVolumeMapsUpstreamQueryErrors(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusBadRequest, esError("parsing_exception", "bad"))
	_, err := serve(t, f).LogVolume(context.Background(), volumeRequest(""))
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("error = %v", err)
	}
}
