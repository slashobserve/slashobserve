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
