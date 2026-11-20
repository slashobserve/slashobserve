package clickhouse

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestTheRangeRefusalNamesTheQueriedTablesTimeColumn(t *testing.T) {
	for text, want := range map[string]string{
		"SELECT TraceId FROM otel_traces LIMIT 3":                      "add $__timeFilter(Timestamp) to the WHERE clause so the query reads only the selected time range",
		"SELECT Body FROM otel.otel_logs":                              "add $__timeFilter(Timestamp) to the WHERE clause so the query reads only the selected time range",
		"SELECT MetricName, Value FROM otel_metrics_gauge":             "add $__timeFilter(TimeUnix) to the WHERE clause so the query reads only the selected time range",
		"SELECT name FROM system.tables":                               "add $__timeFilter(column) to the WHERE clause, naming the table's time column, so the query reads only the selected time range",
		"SELECT 1":                                                     "add $__timeFilter(column) to the WHERE clause, naming the table's time column, so the query reads only the selected time range",
		"SELECT t.TraceId FROM otel_traces AS t JOIN otel_logs l ON 1": "add $__timeFilter(Timestamp) to the WHERE clause so the query reads only the selected time range",
	} {
		err := requireRange(text, false)
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("%q: err = %v, want an invalid query", text, err)
		}
		var ue *datasource.UpstreamError
		if !errors.As(err, &ue) || ue.Message != want {
			t.Errorf("%q: message = %q, want %q", text, err, want)
		}
	}
}
