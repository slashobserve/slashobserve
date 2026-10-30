package clickhouse_test

import (
	"context"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var scratchValues sync.Once

func provisionScratchValues(t *testing.T) {
	t.Helper()
	if testsSkipped {
		t.Skip("integration tests need Docker")
	}
	scratchValues.Do(func() {
		admin, err := rawConn("admin", adminPass)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = admin.Close() }()
		for _, stmt := range []string{
			`CREATE TABLE otel.adhoc_values (Body String) ENGINE = MergeTree ORDER BY Body`,
			`INSERT INTO otel.adhoc_values VALUES ('plain'), ('it''s'), ('a\\b'), ('other')`,
		} {
			if err := admin.Exec(context.Background(), stmt); err != nil {
				t.Fatalf("%v in %q", err, stmt)
			}
		}
	})
}

func filteredCount(t *testing.T, ds datasource.Datasource, text string, filters ...datasource.AdhocFilter) float64 {
	t.Helper()
	q := datasource.Query{Text: text, Filters: filters}
	frames, err := ds.QueryInstant(context.Background(), q, lastHour())
	if err != nil {
		t.Fatalf("QueryInstant(%q) with %+v: %v", text, filters, err)
	}
	if len(frames) != 1 || frames[0].Rows() != 1 {
		t.Fatalf("frames = %+v, want one frame with one row", frames)
	}
	return column(t, frames[0], "n").Values[0].(float64)
}

func TestAdhocFiltersApplyToTheTablesAQueryReads(t *testing.T) {
	provisionScratchValues(t)
	ds := roDS(t)
	cart := datasource.AdhocFilter{Key: "ServiceName", Op: "=", Value: "cart"}
	checkout := datasource.AdhocFilter{Key: "ServiceName", Op: "=", Value: "checkout"}
	notCheckout := datasource.AdhocFilter{Key: "ServiceName", Op: "!=", Value: "checkout"}
	matchChe := datasource.AdhocFilter{Key: "ServiceName", Op: "=~", Value: "che.*"}
	notMatchChe := datasource.AdhocFilter{Key: "ServiceName", Op: "!~", Value: "che.*"}
	tests := []struct {
		name    string
		query   string
		filters []datasource.AdhocFilter
		want    float64
	}{
		{"no filters leaves the query alone", "SELECT count() AS n FROM otel_logs", nil, 7},
		{"a bare table name", "SELECT count() AS n FROM otel_logs", []datasource.AdhocFilter{cart}, 3},
		{"a database qualified table", "SELECT count() AS n FROM otel.otel_logs", []datasource.AdhocFilter{cart}, 3},
		{"a quoted table", "SELECT count() AS n FROM `otel`.`otel_logs`", []datasource.AdhocFilter{cart}, 3},
		{"two filters are ANDed", "SELECT count() AS n FROM otel_logs", []datasource.AdhocFilter{checkout, {Key: "SeverityText", Op: "=", Value: "ERROR"}}, 4},
		{"not equal", "SELECT count() AS n FROM otel_logs", []datasource.AdhocFilter{notCheckout}, 3},
		{"regex match", "SELECT count() AS n FROM otel_logs", []datasource.AdhocFilter{matchChe}, 4},
		{"regex mismatch", "SELECT count() AS n FROM otel_logs", []datasource.AdhocFilter{notMatchChe}, 3},
		{"a subquery", "SELECT count() AS n FROM (SELECT * FROM otel_logs)", []datasource.AdhocFilter{cart}, 3},
		{"a CTE", "WITH l AS (SELECT * FROM otel_logs) SELECT count() AS n FROM l", []datasource.AdhocFilter{cart}, 3},
		{"every table of a join", "SELECT count() AS n FROM otel_logs AS l CROSS JOIN otel_traces AS t", []datasource.AdhocFilter{checkout}, 20},
		{"a filter column only one joined table has", "SELECT count() AS n FROM otel_logs AS l CROSS JOIN otel_traces AS t", []datasource.AdhocFilter{{Key: "SpanName", Op: "=", Value: "GET /cart"}}, 35},
		{"a table without the filter column stays unfiltered", "SELECT count() AS n FROM otel_logs", []datasource.AdhocFilter{{Key: "SpanName", Op: "=", Value: "GET /cart"}}, 7},
		{"a key qualified by its table", "SELECT count() AS n FROM otel_logs AS l CROSS JOIN otel_traces AS t", []datasource.AdhocFilter{{Key: "otel_logs.ServiceName", Op: "=", Value: "cart"}}, 15},
		{"a query that reads no table", "SELECT 1 AS n", []datasource.AdhocFilter{cart}, 1},
		{"a table function", "SELECT count() AS n FROM numbers(5)", []datasource.AdhocFilter{cart}, 5},
		{"a single quote in a value", "SELECT count() AS n FROM adhoc_values", []datasource.AdhocFilter{{Key: "Body", Op: "=", Value: "it's"}}, 1},
		{"a backslash in a value", "SELECT count() AS n FROM adhoc_values", []datasource.AdhocFilter{{Key: "Body", Op: "=", Value: `a\b`}}, 1},
		{"a quote in a value is not an injection", "SELECT count() AS n FROM adhoc_values", []datasource.AdhocFilter{{Key: "Body", Op: "=", Value: "x') OR 1=1 OR ('"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filteredCount(t, ds, tt.query, tt.filters...); got != tt.want {
				t.Fatalf("count = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdhocFiltersApplyToRangeQueries(t *testing.T) {
	ds := roDS(t)
	q := datasource.Query{
		Text:    "SELECT $__timeGroup(TimeUnix) AS t, Attributes['pod'] AS pod, avg(Value) AS v FROM otel_metrics_gauge WHERE $__timeFilter(TimeUnix) GROUP BY t, pod ORDER BY t",
		Filters: []datasource.AdhocFilter{{Key: "ServiceName", Op: "=", Value: "nothing-like-this"}},
	}
	frames, err := ds.QueryRange(context.Background(), q, lastHour(), 10_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if datasource.Points(frames) != 0 {
		t.Fatalf("points = %d, want none", datasource.Points(frames))
	}
}
