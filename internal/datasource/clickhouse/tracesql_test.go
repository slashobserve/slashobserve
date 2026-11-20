package clickhouse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var exploreLimits = datasource.Limits{Timeout: 15 * time.Second, MaxRows: 1000}

const traceSQL = "SELECT TraceId, ServiceName, count() AS Spans FROM otel_traces WHERE $__timeFilter(Timestamp) GROUP BY TraceId, ServiceName"

func traceSQLRequest(text string, size int) datasource.PageRequest {
	req := pageRequest(text, 1, size)
	req.Query.Limits = exploreLimits
	return req
}

func traceRows(n int) [][]any {
	rows := make([][]any, n)
	for i := range rows {
		rows[i] = []any{"4bf92f3577b34da6a3ce929d0e0e4736", "checkout"}
	}
	return rows
}

func TestTraceSQLRefusesAnythingButOneReadingSelectBeforeItRuns(t *testing.T) {
	for _, text := range []string{
		"INSERT INTO otel_traces SELECT * FROM otel_traces",
		"ALTER TABLE otel_traces DELETE WHERE 1",
		"OPTIMIZE TABLE otel_traces FINAL",
		"DROP TABLE otel_traces",
		traceSQL + "; DROP TABLE otel_traces",
		traceSQL + " SETTINGS readonly = 0",
		traceSQL + " FORMAT JSONEachRow",
		traceSQL + " INTO OUTFILE '/tmp/traces.tsv'",
		"SELECT * FROM remote('other:9000', otel.otel_traces) WHERE $__timeFilter(Timestamp)",
		"SELECT TraceId FROM otel_traces WHERE $__timeFilter(Timestamp) AND TraceId IN (SELECT c1 FROM url('http://example.com/ids.csv', CSV))",
		"SELECT * FROM file('/etc/passwd', 'LineAsString') WHERE $__timeFilter(Timestamp)",
		"WITH t AS (SELECT * FROM s3('https://bucket/x.parquet')) SELECT * FROM t WHERE $__timeFilter(Timestamp)",
	} {
		conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId", "ServiceName"), rows: traceRows(1)}, total: 1}
		_, _, err := pageDatasource(conn).QueryPage(context.Background(), traceSQLRequest(text, 50))
		if !errors.Is(err, datasource.ErrForbiddenQuery) && !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("%q: err = %v, want a refused query", text, err)
		}
		if sent := conn.sent(); len(sent) != 0 {
			t.Errorf("%q: sent %q, want nothing sent", text, sent)
		}
	}
}

func TestTraceSQLWithoutTheRangeIsRefusedAfterTheColumnProbe(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId", "ServiceName"), rows: traceRows(1)}, total: 1}
	_, _, err := pageDatasource(conn).QueryPage(context.Background(), traceSQLRequest("SELECT TraceId, ServiceName FROM otel_traces -- $__timeFilter(Timestamp)", 50))
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "$__timeFilter") {
		t.Fatalf("err = %v, want the range rule", err)
	}
	if sent := conn.sent(); len(sent) != 1 || !strings.HasSuffix(sent[0], "LIMIT 0") {
		t.Fatalf("sent %q, want only the column probe", sent)
	}
}

func TestTraceSQLStopsAtThePageSize(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId", "ServiceName"), rows: traceRows(51)}, total: 51}
	_, _, err := pageDatasource(conn).QueryPage(context.Background(), traceSQLRequest(traceSQL, 50))
	if !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("err = %v, want the row limit", err)
	}
}

func TestTraceSQLReturnsAPlainPageOfItsColumns(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId", "ServiceName"), rows: traceRows(2)}, total: 2}
	frames, info, err := pageDatasource(conn).QueryPage(context.Background(), traceSQLRequest(traceSQL, 50))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Rows() != 2 || frames[0].Fields[0].Name != "TraceId" || !info.Plain || info.Total != 2 {
		t.Fatalf("frames = %+v, info = %+v", frames, info)
	}
	for _, sql := range conn.sent() {
		if strings.Contains(sql, "$__") {
			t.Fatalf("a macro reached ClickHouse unexpanded: %q", sql)
		}
	}
}

type deadlineConn struct {
	*pageConn
	deadlines []time.Duration
}

func (c *deadlineConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	if deadline, ok := ctx.Deadline(); ok {
		c.deadlines = append(c.deadlines, time.Until(deadline))
	}
	return c.pageConn.Query(ctx, query, args...)
}

func TestTraceSQLRunsWithinTheDraftTimeout(t *testing.T) {
	conn := &deadlineConn{pageConn: &pageConn{fakeConn: &fakeConn{columns: stringColumns("TraceId", "ServiceName"), rows: traceRows(1)}, total: 1}}
	d := pageDatasource(conn.pageConn)
	d.conn = conn
	if _, _, err := d.QueryPage(context.Background(), traceSQLRequest(traceSQL, 50)); err != nil {
		t.Fatal(err)
	}
	if len(conn.deadlines) == 0 {
		t.Fatal("no statement ran under a deadline")
	}
	for _, left := range conn.deadlines {
		if left <= 0 || left > exploreLimits.Timeout {
			t.Fatalf("deadline in %s, want within %s", left, exploreLimits.Timeout)
		}
	}
}
