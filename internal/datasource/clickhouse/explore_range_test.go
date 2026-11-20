package clickhouse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var unboundedExploreQueries = []string{
	"SELECT TraceId FROM otel_traces LIMIT 3",
	"SELECT TraceId FROM otel_traces -- WHERE $__timeFilter(Timestamp)\nLIMIT 3",
	"SELECT Timestamp, Body FROM otel_logs LIMIT 3",
	"SELECT ServiceName, count() FROM otel_logs /* $__timeFilter(Timestamp) */ GROUP BY ServiceName",
}

func exploreQuery(text string) datasource.Query {
	return datasource.Query{Text: text, Limits: exploreLimits, Explore: true}
}

func runBoth(d *Datasource, q datasource.Query) map[string]error {
	rng := datasource.TimeRange{From: linesStart, To: linesEnd}
	_, instant := d.QueryInstant(context.Background(), q, rng)
	q.Format = "table"
	_, ranged := d.QueryRange(context.Background(), q, rng, time.Minute)
	return map[string]error{"instant": instant, "range": ranged}
}

func TestExploreQueryWithoutTheRangeIsRefusedBeforeItRuns(t *testing.T) {
	for _, text := range unboundedExploreQueries {
		conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId")}}
		for kind, err := range runBoth(pageDatasource(conn), exploreQuery(text)) {
			if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "$__timeFilter(Timestamp)") {
				t.Errorf("%s %q: err = %v, want the range rule", kind, text, err)
			}
		}
		if sent := conn.sent(); len(sent) != 0 {
			t.Errorf("%q: sent %q, want nothing sent", text, sent)
		}
	}
}

func TestExploreQueryReadingTheRangeRuns(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId")}}
	for kind, err := range runBoth(pageDatasource(conn), exploreQuery("SELECT TraceId FROM otel_traces WHERE $__timeFilter(Timestamp) LIMIT 3")) {
		if err != nil {
			t.Errorf("%s: err = %v", kind, err)
		}
	}
	if sent := conn.sent(); len(sent) != 2 {
		t.Fatalf("sent %q, want both queries", sent)
	}
}

func TestSavedPanelQueryWithoutTheRangeStillRuns(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId")}}
	q := exploreQuery(unboundedExploreQueries[0])
	q.Explore = false
	for kind, err := range runBoth(pageDatasource(conn), q) {
		if err != nil {
			t.Errorf("%s: err = %v", kind, err)
		}
	}
	if sent := conn.sent(); len(sent) != 2 {
		t.Fatalf("sent %q, want both queries", sent)
	}
}
