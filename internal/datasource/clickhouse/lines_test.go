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

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func linesDatasource(conn *fakeConn) *Datasource {
	d := &Datasource{conn: conn, limits: datasource.DefaultLimits, database: "otel"}
	d.readOnlyUntil.Store(math.MaxInt64)
	return d
}

var (
	linesStart = time.Date(2026, 11, 6, 11, 0, 0, 123456789, time.UTC)
	linesEnd   = time.Date(2026, 11, 6, 12, 0, 0, 5, time.UTC)
	logColumns = []fakeColumn{{"Timestamp", "DateTime64(9)"}, {"Body", "String"}}
)

func logLines(n int) [][]any {
	rows := make([][]any, n)
	for i := range rows {
		rows[i] = []any{linesStart.Add(time.Duration(i) * time.Second), "line"}
	}
	return rows
}

func linesRequest(text string, direction datasource.LineDirection, limit int) datasource.LinesRequest {
	return datasource.LinesRequest{Query: datasource.Query{Text: text}, Range: datasource.TimeRange{From: linesStart, To: linesEnd}, Direction: direction, Limit: limit}
}

func TestLinesWrapsTheQueryWithExclusiveNanosecondBoundsInEachDirection(t *testing.T) {
	inner := "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes FROM otel_logs WHERE " +
		"(Timestamp >= fromUnixTimestamp64Milli(toInt64(1793962800123), 'UTC') AND Timestamp <= fromUnixTimestamp64Milli(toInt64(1793966400001), 'UTC')) "
	for direction, order := range map[datasource.LineDirection]string{datasource.Forward: "ASC", datasource.Backward: "DESC"} {
		t.Run(string(direction), func(t *testing.T) {
			conn := &fakeConn{columns: logColumns, rows: logLines(2)}
			frames, err := linesDatasource(conn).Lines(context.Background(), linesRequest(webLogsTemplate, direction, 50))
			if err != nil {
				t.Fatal(err)
			}
			if len(conn.statements) != 2 {
				t.Fatalf("statements = %q, want the Timestamp probe then the lines query", conn.statements)
			}
			if want := "SELECT * FROM (\n" + inner + "\n) LIMIT 0"; conn.statements[0] != want {
				t.Fatalf("probe =\n%s\nwant\n%s", conn.statements[0], want)
			}
			want := "SELECT * FROM (\n" + inner + "\n)\nWHERE `Timestamp` >= {from:DateTime64(9, 'UTC')} AND `Timestamp` < {to:DateTime64(9, 'UTC')}\nORDER BY `Timestamp` " + order + "\nLIMIT {limit:UInt64}"
			if conn.statements[1] != want {
				t.Fatalf("query =\n%s\nwant\n%s", conn.statements[1], want)
			}
			if len(frames) != 1 || frames[0].Rows() != 2 || frames[0].Meta != nil {
				t.Fatalf("frames = %+v, want one frame of 2 rows below the limit", frames)
			}
			if ns, ok := datasource.TimeNsOf(frames[0], 1); !ok || ns != linesStart.Add(time.Second).UnixNano() {
				t.Fatalf("TimeNs of row 1 = %d, %v", ns, ok)
			}
		})
	}
}

func TestLinesParametersCarryTheExactBoundsAndLimit(t *testing.T) {
	got := linesParameters(datasource.TimeRange{From: linesStart, To: linesEnd}, 7)
	want := ch.Parameters{"from": "2026-11-06 11:00:00.123456789", "to": "2026-11-06 12:00:00.000000005", "limit": "7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parameters = %v, want %v", got, want)
	}
}

func TestWholeMillisecondsWidensTheMacroRangeAroundTheBounds(t *testing.T) {
	cases := []struct {
		name     string
		from, to time.Time
		wantFrom time.Time
		wantTo   time.Time
	}{
		{"sub-millisecond bounds", linesStart, linesEnd, time.Date(2026, 11, 6, 11, 0, 0, 123000000, time.UTC), time.Date(2026, 11, 6, 12, 0, 0, 1000000, time.UTC)},
		{"whole milliseconds stay", time.Date(2026, 11, 6, 11, 0, 0, 5000000, time.UTC), time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), time.Date(2026, 11, 6, 11, 0, 0, 5000000, time.UTC), time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wholeMilliseconds(datasource.TimeRange{From: tc.from, To: tc.to})
			if !got.From.Equal(tc.wantFrom) || !got.To.Equal(tc.wantTo) {
				t.Fatalf("range = %v..%v, want %v..%v", got.From, got.To, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

func TestLinesMarksTruncationAtTheLimit(t *testing.T) {
	cases := []struct {
		name      string
		limit     int
		rows      int
		truncated bool
	}{
		{"rows equal the limit", 3, 3, true},
		{"fewer rows", 3, 2, false},
		{"limit below one is one", 0, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn := &fakeConn{columns: logColumns, rows: logLines(tc.rows)}
			frames, err := linesDatasource(conn).Lines(context.Background(), linesRequest(webLogsTemplate, datasource.Backward, tc.limit))
			if err != nil {
				t.Fatal(err)
			}
			if got := frames[0].Meta != nil && frames[0].Meta.Truncated; got != tc.truncated {
				t.Fatalf("truncated = %v, want %v", got, tc.truncated)
			}
		})
	}
}

func TestLinesRefusesAQueryWithoutTimestamp(t *testing.T) {
	conn := &fakeConn{columns: stringColumns("Body"), rows: [][]any{{"line"}}}
	_, err := linesDatasource(conn).Lines(context.Background(), linesRequest("SELECT Body FROM otel_logs", datasource.Forward, 10))
	if !errors.Is(err, datasource.ErrInvalidQuery) || err.Error() != "invalid query: the query must select Timestamp" {
		t.Fatalf("err = %v", err)
	}
	if len(conn.statements) != 1 || !strings.HasSuffix(conn.statements[0], "LIMIT 0") {
		t.Fatalf("statements = %q, want only the probe", conn.statements)
	}
}

func TestLinesRefusesSetOperatorsAndABadRange(t *testing.T) {
	conn := &fakeConn{columns: logColumns}
	d := linesDatasource(conn)
	_, err := d.Lines(context.Background(), linesRequest("SELECT * FROM a UNION ALL SELECT * FROM b", datasource.Forward, 10))
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want invalid query for a set operator", err)
	}
	req := linesRequest(webLogsTemplate, datasource.Forward, 10)
	req.Range = datasource.TimeRange{From: linesEnd, To: linesStart}
	if _, err := d.Lines(context.Background(), req); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want invalid query for from after to", err)
	}
	if len(conn.statements) != 0 {
		t.Fatalf("statements = %q, want none for refused calls", conn.statements)
	}
}
