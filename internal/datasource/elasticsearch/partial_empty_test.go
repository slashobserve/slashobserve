package elasticsearch_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestAFailedShardOnAnEmptyAggregationStillSaysItIsPartial(t *testing.T) {
	reply := `{"timed_out":false,"_shards":{"failed":2},"hits":{"hits":[]},"aggregations":{"count":{"buckets":[]}}}`
	_, ds := searchServer(t, reply)

	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: "service.name:checkout"}, queryRange, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if !datasource.MergeMeta(frames).Partial {
		t.Fatalf("frames = %+v", frames)
	}
}

func TestAnEmptyAggregationWithHealthyShardsIsNotPartial(t *testing.T) {
	reply := strings.Replace(`{"timed_out":false,"_shards":{"failed":2},"hits":{"hits":[]},"aggregations":{"count":{"buckets":[]}}}`, `"failed":2`, `"failed":0`, 1)
	_, ds := searchServer(t, reply)

	frames, err := ds.QueryRange(context.Background(), datasource.Query{Text: "service.name:checkout"}, queryRange, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if datasource.MergeMeta(frames).Partial {
		t.Fatalf("frames = %+v", frames)
	}
}

func TestACountOverTimeCarriesTheDashboardTimeZone(t *testing.T) {
	reply := `{"hits":{"hits":[]},"aggregations":{"count":{"buckets":[]}}}`
	f, ds := searchServer(t, reply)

	if _, err := ds.QueryRange(context.Background(), datasource.Query{Text: "service.name:checkout", TimeZone: "Asia/Kolkata"}, queryRange, time.Hour); err != nil {
		t.Fatal(err)
	}

	histogram := searchBody(t, f.last(t))["aggs"].(map[string]any)["count"].(map[string]any)["date_histogram"].(map[string]any)
	if histogram["time_zone"] != "Asia/Kolkata" {
		t.Fatalf("date_histogram = %v", histogram)
	}
}

func TestACountOverTimeWithoutAZoneSendsNone(t *testing.T) {
	reply := `{"hits":{"hits":[]},"aggregations":{"count":{"buckets":[]}}}`
	f, ds := searchServer(t, reply)

	if _, err := ds.QueryRange(context.Background(), datasource.Query{Text: "service.name:checkout"}, queryRange, time.Hour); err != nil {
		t.Fatal(err)
	}

	histogram := searchBody(t, f.last(t))["aggs"].(map[string]any)["count"].(map[string]any)["date_histogram"].(map[string]any)
	if _, has := histogram["time_zone"]; has {
		t.Fatalf("date_histogram = %v", histogram)
	}
}
