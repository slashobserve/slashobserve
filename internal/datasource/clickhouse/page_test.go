package clickhouse

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type pageConn struct {
	*fakeConn
	total uint64
}

func (c *pageConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	if !strings.HasPrefix(query, "SELECT count() FROM (") {
		return c.fakeConn.Query(ctx, query, args...)
	}
	c.mu.Lock()
	c.statements = append(c.statements, query)
	c.mu.Unlock()
	return &fakeRows{columns: []fakeColumn{{"count()", "UInt64"}}, rows: [][]any{{c.total}}}, nil
}

func pageDatasource(conn *pageConn) *Datasource {
	d := &Datasource{conn: conn, limits: datasource.DefaultLimits, database: "otel"}
	d.readOnlyUntil.Store(math.MaxInt64)
	return d
}

func pageRequest(text string, page, size int) datasource.PageRequest {
	return datasource.PageRequest{Query: datasource.Query{Text: text}, Range: datasource.TimeRange{From: linesStart, To: linesEnd}, Page: page, PageSize: size}
}

func TestPageQueryAddsOffsetAndOrder(t *testing.T) {
	inner := "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE " +
		"(Timestamp >= fromUnixTimestamp64Milli(toInt64(1793962800123), 'UTC') AND Timestamp <= fromUnixTimestamp64Milli(toInt64(1793966400001), 'UTC')) "
	conn := &pageConn{fakeConn: &fakeConn{columns: logColumns, rows: logLines(100)}, total: 350}
	frames, info, err := pageDatasource(conn).QueryPage(context.Background(), pageRequest(webLogsTemplate, 3, 100))
	if err != nil {
		t.Fatal(err)
	}
	if len(conn.statements) != 3 {
		t.Fatalf("statements = %q, want the Timestamp probe, the page query and the count", conn.statements)
	}
	filter := "WHERE `Timestamp` >= {from:DateTime64(9, 'UTC')} AND `Timestamp` < {to:DateTime64(9, 'UTC')}\n"
	if want := "SELECT * FROM (\n" + inner + "\n)\n" + filter + "ORDER BY `Timestamp` DESC\nLIMIT {limit:UInt64}\nOFFSET {offset:UInt64}"; conn.statements[1] != want {
		t.Fatalf("page query =\n%s\nwant\n%s", conn.statements[1], want)
	}
	if want := "SELECT count() FROM (\n" + inner + "\n)\n" + filter; conn.statements[2] != want {
		t.Fatalf("count query =\n%s\nwant\n%s", conn.statements[2], want)
	}
	if len(frames) != 1 || frames[0].Rows() != 100 || frames[0].Meta != nil {
		t.Fatalf("frames = %+v, want one frame of 100 rows with no truncation meta", frames)
	}
	if info.Plain {
		t.Fatalf("a result with Timestamp is not plain: %+v", info)
	}
	want := datasource.PageInfo{Page: 3, PageSize: 100, Total: 350, TotalRelation: datasource.TotalExact, HasNext: true, Next: &datasource.PageCursor{Page: 4}}
	if !reflect.DeepEqual(info, want) {
		t.Fatalf("info = %+v, want %+v", info, want)
	}
}

func TestPageParametersCarryTheLimitAndOffset(t *testing.T) {
	got := linesParameters(datasource.TimeRange{From: linesStart, To: linesEnd}, 100, 200)
	want := ch.Parameters{"from": "2026-11-06 11:00:00.123456789", "to": "2026-11-06 12:00:00.000000005", "limit": "100", "offset": "200"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parameters = %v, want %v", got, want)
	}
}

func TestCountQueryPassesTheGuard(t *testing.T) {
	inner, err := volumeInner("SELECT Timestamp, Body FROM otel_logs WHERE ServiceName = 'checkout' ORDER BY Timestamp DESC LIMIT 1000")
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"count": countQuery(inner), "page": linesQuery(inner, datasource.Backward)} {
		if _, err := guard(text); err != nil {
			t.Fatalf("%s query refused by the guard: %v", name, err)
		}
	}
}

func TestPageInfoFromCounts(t *testing.T) {
	cases := []struct {
		name    string
		offset  int
		rows    int
		total   int
		hasNext bool
	}{
		{"rows remain past the page", 200, 100, 350, true},
		{"the last page", 200, 50, 250, false},
		{"an empty result", 0, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := pageInfo(tc.offset/100+1, 100, tc.offset, tc.rows, tc.total)
			if info.HasNext != tc.hasNext || info.Total != int64(tc.total) || info.TotalRelation != datasource.TotalExact {
				t.Fatalf("info = %+v, want has_next %v and an exact total of %d", info, tc.hasNext, tc.total)
			}
			if (info.Next != nil) != tc.hasNext {
				t.Fatalf("next = %+v, want one only with has_next", info.Next)
			}
		})
	}
}

func plainPage(t *testing.T, text string, page int) (*pageConn, error) {
	t.Helper()
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("Body"), rows: [][]any{{"line"}}}, total: 41}
	_, _, err := pageDatasource(conn).QueryPage(context.Background(), pageRequest(text, page, 20))
	return conn, err
}

func TestPageReadsAQueryWithoutTimestampAsAPlainTable(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("Body"), rows: [][]any{{"line"}}}, total: 41}
	frames, info, err := pageDatasource(conn).QueryPage(context.Background(), pageRequest("SELECT Body FROM otel_logs WHERE $__timeFilter(EventTime)", 2, 20))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || len(frames[0].Fields) != 1 || frames[0].Fields[0].Name != "Body" {
		t.Fatalf("frames = %+v", frames)
	}
	if info.Total != 41 || info.Page != 2 || !info.HasNext || !info.Plain {
		t.Fatalf("info = %+v, want a plain page", info)
	}
	if len(conn.statements) != 3 {
		t.Fatalf("statements = %q, want the probe, the page and the count", conn.statements)
	}
	if strings.Contains(conn.statements[1], "`Timestamp`") || strings.Contains(conn.statements[2], "`Timestamp`") {
		t.Fatalf("a plain page filters or orders by Timestamp: %q", conn.statements[1:])
	}
}

func asWrittenPage(t *testing.T, text string, rows [][]any) (*pageConn, []datasource.Frame, datasource.PageInfo) {
	t.Helper()
	columns := []fakeColumn{{"Timestamp", "DateTime64(9)"}, {"SpanName", "String"}, {"Duration", "UInt64"}}
	conn := &pageConn{fakeConn: &fakeConn{columns: columns, rows: rows}, total: uint64(len(rows))}
	req := pageRequest(text, 1, 50)
	req.AsWritten = true
	frames, info, err := pageDatasource(conn).QueryPage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return conn, frames, info
}

func TestPageAsWrittenKeepsTheOrderLimitAndColumnsOfAQueryThatSelectsTimestamp(t *testing.T) {
	text := "SELECT Timestamp, SpanName, Duration FROM otel_traces WHERE $__timeFilter(Timestamp) ORDER BY Duration DESC LIMIT 3"
	rows := [][]any{
		{linesStart, "a", uint64(30)},
		{linesStart.Add(time.Second), "b", uint64(20)},
		{linesStart.Add(2 * time.Second), "c", uint64(10)},
	}
	conn, frames, info := asWrittenPage(t, text, rows)
	for _, sql := range conn.statements[1:] {
		if !strings.Contains(sql, "ORDER BY Duration DESC LIMIT 3") {
			t.Fatalf("statement lost the person's ORDER BY and LIMIT: %q", sql)
		}
		if strings.Contains(sql, "`Timestamp` >=") || strings.Contains(sql, "ORDER BY `Timestamp`") {
			t.Fatalf("statement reorders or refilters by Timestamp: %q", sql)
		}
	}
	var names []string
	for _, f := range frames[0].Fields {
		names = append(names, f.Name)
	}
	if want := []string{"Timestamp", "SpanName", "Duration"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("columns = %v, want %v", names, want)
	}
	if !info.Plain || info.Total != 3 || info.HasNext {
		t.Fatalf("info = %+v, want a plain page of the 3 rows the query asked for", info)
	}
}

func TestPageAsWrittenStillRefusesAQueryThatDoesNotReadTheRange(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: logColumns, rows: logLines(1)}, total: 1}
	req := pageRequest("SELECT Timestamp, Body FROM otel_logs LIMIT 3", 1, 50)
	req.AsWritten = true
	_, _, err := pageDatasource(conn).QueryPage(context.Background(), req)
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "$__timeFilter") {
		t.Fatalf("err = %v, want an invalid query error naming the macro", err)
	}
}

func TestPageWithoutAsWrittenStillAddsTimeNsToATimestampResult(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: logColumns, rows: logLines(2)}, total: 2}
	frames, _, err := pageDatasource(conn).QueryPage(context.Background(), pageRequest(webLogsTemplate, 1, 50))
	if err != nil {
		t.Fatal(err)
	}
	last := frames[0].Fields[len(frames[0].Fields)-1]
	if last.Name != datasource.TimeNsFieldName {
		t.Fatalf("last field = %q, want %s", last.Name, datasource.TimeNsFieldName)
	}
}

func TestPageRefusesAPlainQueryThatDoesNotReadTheRange(t *testing.T) {
	for _, text := range []string{
		"SELECT ServiceName, count() AS n FROM otel_logs GROUP BY ServiceName",
		"SELECT Body FROM otel_logs",
		"SELECT Body FROM otel_logs WHERE EventTime > now() - INTERVAL 1 DAY",
		"SELECT ServiceName, count() AS n FROM otel_traces GROUP BY ServiceName -- $__timeFilter(Timestamp)",
		"SELECT ServiceName, count() AS n FROM otel_traces /* WHERE $__timeFilter(Timestamp) */ GROUP BY ServiceName",
		"SELECT SpanName FROM otel_traces # $__fromTime\n",
	} {
		conn, err := plainPage(t, text, 1)
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "$__timeFilter") {
			t.Fatalf("%q: err = %v, want an invalid query error naming the macro", text, err)
		}
		if len(conn.statements) != 1 {
			t.Fatalf("%q: statements = %q, want only the column probe", text, conn.statements)
		}
	}
}

func TestPageKeepsTheOrderAndLimitOfAPlainQuery(t *testing.T) {
	text := "SELECT ServiceName, count() AS n FROM otel_logs WHERE $__timeFilter(EventTime) GROUP BY ServiceName ORDER BY n DESC LIMIT 5;"
	conn, err := plainPage(t, text, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range conn.statements[1:] {
		if !strings.Contains(sql, "ORDER BY n DESC LIMIT 5") || strings.Contains(sql, ";") {
			t.Fatalf("statement lost the person's ORDER BY and LIMIT: %q", sql)
		}
	}
}

func TestPageOrdersAPlainQueryWithoutOrderByAcrossPages(t *testing.T) {
	conn, err := plainPage(t, "SELECT ServiceName, count() AS n FROM otel_logs WHERE $__timeFilter(EventTime) GROUP BY ServiceName", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conn.statements[1], "ORDER BY ALL") {
		t.Fatalf("page query = %q, want a total order so pages do not overlap", conn.statements[1])
	}
}

func TestPageQueryOrdersByTheSortColumnThenNewest(t *testing.T) {
	inner := "SELECT 1"
	got := pageQuery(inner, &datasource.PageSort{Column: "Duration", Descending: true})
	want := "SELECT * FROM (\nSELECT 1\n)\n" + rangeFilter() + "ORDER BY `Duration` DESC, `Timestamp` DESC\nLIMIT {limit:UInt64}\nOFFSET {offset:UInt64}"
	if got != want {
		t.Fatalf("query =\n%s\nwant\n%s", got, want)
	}
	if got := pageQuery(inner, &datasource.PageSort{Column: "Spans"}); !strings.Contains(got, "ORDER BY `Spans` ASC, `Timestamp` DESC") {
		t.Fatalf("query = %s, want an ascending Spans order", got)
	}
	if pageQuery(inner, nil) != linesQuery(inner, datasource.Backward) {
		t.Fatal("no sort must keep the newest first order")
	}
}

func TestPageSortQueryPassesTheGuard(t *testing.T) {
	text := pageQuery("SELECT TraceId, min(Timestamp) AS Timestamp, max(Duration) AS Duration FROM otel_traces GROUP BY TraceId", &datasource.PageSort{Column: "Duration", Descending: true})
	if _, err := guard(text); err != nil {
		t.Fatalf("refused by the guard: %v", err)
	}
}

func TestPageSortNeedsAColumnOfTheResult(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: logColumns, rows: logLines(5)}, total: 5}
	req := pageRequest(webLogsTemplate, 1, 100)
	req.Sort = &datasource.PageSort{Column: "Duration"}
	_, _, err := pageDatasource(conn).QueryPage(context.Background(), req)
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), "sort column Duration is not in the result; the columns are Timestamp, Body") {
		t.Fatalf("err = %v", err)
	}
}

func TestPageSortRunsTheOrderedQueryOnAKnownColumn(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: logColumns, rows: logLines(5)}, total: 5}
	req := pageRequest(webLogsTemplate, 1, 100)
	req.Sort = &datasource.PageSort{Column: "Body", Descending: true}
	if _, _, err := pageDatasource(conn).QueryPage(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(conn.statements) != 4 || !strings.Contains(conn.statements[2], "ORDER BY `Body` DESC, `Timestamp` DESC") {
		t.Fatalf("statements = %q, want the probes, the ordered page query and the count", conn.statements)
	}
}

func TestPageSortsAPlainQueryByTheNamedColumn(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("ServiceName", "n"), rows: [][]any{{"a", "1"}}}, total: 3}
	req := pageRequest("SELECT ServiceName, count() AS n FROM otel_logs WHERE $__timeFilter(EventTime) GROUP BY ServiceName", 1, 20)
	req.Sort = &datasource.PageSort{Column: "n", Descending: true}
	if _, _, err := pageDatasource(conn).QueryPage(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if page := conn.statements[len(conn.statements)-2]; !strings.Contains(page, "ORDER BY `n` DESC\nLIMIT") {
		t.Fatalf("page query = %q, want the sort column to order it", page)
	}
}
