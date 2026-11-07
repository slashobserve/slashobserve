package clickhouse

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

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

func TestPageRefusesAQueryWithoutTimestamp(t *testing.T) {
	conn := &pageConn{fakeConn: &fakeConn{columns: stringColumns("Body"), rows: [][]any{{"line"}}}}
	_, _, err := pageDatasource(conn).QueryPage(context.Background(), pageRequest("SELECT Body FROM otel_logs", 1, 100))
	if !errors.Is(err, datasource.ErrInvalidQuery) || err.Error() != "invalid query: the query must select Timestamp" {
		t.Fatalf("err = %v", err)
	}
	if len(conn.statements) != 1 {
		t.Fatalf("statements = %q, want only the probe", conn.statements)
	}
}
