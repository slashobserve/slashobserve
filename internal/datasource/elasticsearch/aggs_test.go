package elasticsearch_test

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

//go:embed testdata/aggs_*.json
var aggsData embed.FS

type aggsCase struct {
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
	Want     json.RawMessage `json:"want"`
}

func loadAggsCase(t *testing.T, name string) aggsCase {
	t.Helper()
	raw, err := aggsData.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var c aggsCase
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return c
}

func foldWith(t *testing.T, c aggsCase, limits datasource.Limits) ([]datasource.Frame, error) {
	t.Helper()
	reply := `{"timed_out":false,"hits":{"total":{"value":6,"relation":"eq"},"hits":[]},"aggregations":` + string(c.Response) + `}`
	_, ds := searchServer(t, reply)
	q := datasource.Query{Text: string(c.Request), Limits: limits}
	return ds.QueryRange(context.Background(), q, queryRange, time.Minute)
}

func render(frames []datasource.Frame) any {
	if len(frames) == 1 && !frames[0].IsSeries() {
		f := frames[0]
		columns := make([]string, len(f.Fields))
		rows := make([][]any, f.Rows())
		for i, field := range f.Fields {
			columns[i] = field.Name
			for j, v := range field.Values {
				rows[j] = append(rows[j], v)
			}
		}
		return map[string]any{"table": map[string]any{"columns": columns, "rows": rows}}
	}
	series := make([]map[string]any, len(frames))
	for i, f := range frames {
		points := make([][]any, f.Rows())
		for j := range points {
			points[j] = []any{f.Fields[0].Values[j].(interface{ UnixMilli() int64 }).UnixMilli(), f.Fields[1].Values[j]}
		}
		labels := f.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		series[i] = map[string]any{"name": f.Name, "labels": labels, "points": points}
	}
	return map[string]any{"series": series}
}

func TestFoldAggregations(t *testing.T) {
	for _, name := range []string{
		"aggs_histogram_terms_avg",
		"aggs_terms_histogram_count",
		"aggs_terms_terms_table",
		"aggs_terms_metrics_table",
		"aggs_histogram_percentiles",
		"aggs_histogram_filters",
	} {
		t.Run(name, func(t *testing.T) {
			c := loadAggsCase(t, name)
			frames, err := foldWith(t, c, datasource.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(render(frames))
			if err != nil {
				t.Fatal(err)
			}
			assertJSONEqual(t, string(got), string(c.Want))
		})
	}
}

func TestFoldAggregationsRefusesWhatItCannotFold(t *testing.T) {
	hist := `"date_histogram": {"field": "@timestamp", "fixed_interval": "1m"}`
	for _, tc := range []struct{ name, request, message string }{
		{"top_hits", `{"aggs": {"t": {` + hist + `, "aggs": {"hits": {"top_hits": {"size": 1}}}}}}`, `unsupported aggregation "hits" (type top_hits)`},
		{"composite", `{"aggs": {"c": {"composite": {"sources": []}}}}`, `unsupported aggregation "c" (type composite)`},
		{"two buckets on one level", `{"aggs": {"a": {"terms": {"field": "x"}}, "b": {"terms": {"field": "y"}}}}`, "one bucket aggregation per level"},
		{"a time bucket inside a time bucket", `{"aggs": {"t": {` + hist + `, "aggs": {"u": {` + hist + `}}}}}`, "a time bucket inside a time bucket"},
		{"not an object", `{"aggs": {"t": 3}}`, `unsupported aggregation "t"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := aggsCase{Request: json.RawMessage(tc.request), Response: json.RawMessage(`{}`)}
			_, err := foldWith(t, c, datasource.Limits{})
			if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error = %v, want ErrInvalidQuery containing %q", err, tc.message)
			}
		})
	}
}

func TestFoldAggregationsEnforcesTheLimits(t *testing.T) {
	series := loadAggsCase(t, "aggs_histogram_terms_avg")
	if _, err := foldWith(t, series, datasource.Limits{MaxSeries: 1}); !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("max series: %v", err)
	}
	if _, err := foldWith(t, series, datasource.Limits{MaxPoints: 2}); !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("max points: %v", err)
	}
	table := loadAggsCase(t, "aggs_terms_terms_table")
	frames, err := foldWith(t, table, datasource.Limits{MaxRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	if frames[0].Rows() != 2 || frames[0].Meta == nil || !frames[0].Meta.Truncated {
		t.Fatalf("rows = %d, meta = %+v, want 2 rows and Truncated", frames[0].Rows(), frames[0].Meta)
	}
	if !reflect.DeepEqual(frames[0].Fields[2].Values, []any{2.0, 1.0}) {
		t.Fatalf("counts = %v", frames[0].Fields[2].Values)
	}
}

func TestFoldAggregationsMarksPartialResults(t *testing.T) {
	c := loadAggsCase(t, "aggs_terms_terms_table")
	reply := `{"timed_out":true,"hits":{"total":{"value":6,"relation":"eq"},"hits":[]},"aggregations":` + string(c.Response) + `}`
	_, ds := searchServer(t, reply)
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: string(c.Request)}, queryRange)
	if err != nil || frames[0].Meta == nil || !frames[0].Meta.Partial {
		t.Fatalf("frames = %+v, err = %v", frames, err)
	}
}

func TestFoldAggregationsWithOnlyMetricsIsOneRow(t *testing.T) {
	c := aggsCase{
		Request:  json.RawMessage(`{"aggs": {"avg_ms": {"avg": {"field": "d"}}, "uniq": {"cardinality": {"field": "u"}}}}`),
		Response: json.RawMessage(`{"avg_ms": {"value": 3.5}, "uniq": {"value": 4}}`),
	}
	frames, err := foldWith(t, c, datasource.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(render(frames))
	assertJSONEqual(t, string(got), `{"table": {"columns": ["count", "avg_ms", "uniq"], "rows": [[6, 3.5, 4]]}}`)
}
