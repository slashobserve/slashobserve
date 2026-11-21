package auditsource

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func histogramOver(hours int) map[string]any {
	low := from.UnixMilli()
	return map[string]any{"date_histogram": map[string]any{
		"field": TimeField, "fixed_interval": "1m",
		"extended_bounds": map[string]any{"min": low, "max": low + int64(hours)*time.Hour.Milliseconds()},
	}}
}

func queryAggs(t *testing.T, aggs map[string]any) error {
	t.Helper()
	body, err := json.Marshal(map[string]any{"size": 0, "aggs": aggs})
	if err != nil {
		t.Fatal(err)
	}
	_, err = open(t, &fakeStore{}).QueryInstant(t.Context(), datasource.Query{Text: string(body)}, span)
	return err
}

func TestARequestHoldsAtMostTwentyAggregationsNestedIncluded(t *testing.T) {
	nested := func(n int) map[string]any {
		inner := map[string]any{}
		for i := range n - 1 {
			inner[fmt.Sprintf("c%d", i)] = map[string]any{"cardinality": map[string]any{"field": "actor_id"}}
		}
		return map[string]any{"outer": map[string]any{"terms": map[string]any{"field": "action"}, "aggs": inner}}
	}
	if err := queryAggs(t, nested(20)); err != nil {
		t.Fatalf("twenty aggregations: %v", err)
	}
	err := queryAggs(t, nested(21))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "at most 20 aggregations") {
		t.Fatalf("twenty-one aggregations = %v, want an invalid query naming the cap", err)
	}
}

func TestTheFilledBucketsOfARequestAreCappedTogether(t *testing.T) {
	if err := queryAggs(t, map[string]any{"a": histogramOver(100)}); err != nil {
		t.Fatalf("one histogram of 6001 buckets: %v", err)
	}
	err := queryAggs(t, map[string]any{"a": histogramOver(100), "b": histogramOver(100)})
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "more than 11000 buckets in one request") {
		t.Fatalf("two histograms of 6001 buckets = %v, want an invalid query naming the cap", err)
	}
}
