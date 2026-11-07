package clickhouse

import (
	"fmt"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func logRows(n int) *resultSet {
	rs := &resultSet{columns: []column{{name: "Timestamp", typ: datasource.FieldTime}, {name: "Body", typ: datasource.FieldString}}}
	start := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for i := range n {
		rs.rows = append(rs.rows, []any{start.Add(time.Duration(i) * time.Second), "line"})
	}
	return rs
}

func TestRawFramesMarkTruncationAtTheRowLimit(t *testing.T) {
	cases := []struct {
		name      string
		rows      int
		truncated bool
	}{
		{"exactly the row limit", 3, true},
		{"fewer rows than the limit", 2, false},
		{"no rows", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frames, err := rawFrames(logRows(tc.rows), datasource.Limits{MaxRows: 3, MaxPoints: 100})
			if err != nil {
				t.Fatal(err)
			}
			if len(frames) != 1 || frames[0].Rows() != tc.rows {
				t.Fatalf("frames = %+v, want one frame of %d rows", frames, tc.rows)
			}
			if got := frames[0].Meta != nil && frames[0].Meta.Truncated; got != tc.truncated {
				t.Fatalf("truncated = %v, want %v (meta %+v)", got, tc.truncated, frames[0].Meta)
			}
		})
	}
}

func TestSeriesFramesMarkTruncationOnTheFirstFrameOnly(t *testing.T) {
	rs := &resultSet{columns: []column{{name: "Time", typ: datasource.FieldTime}, {name: "service", typ: datasource.FieldString}, {name: "count", typ: datasource.FieldNumber}}}
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	rs.rows = [][]any{{at, "a", int64(1)}, {at, "b", int64(2)}}
	frames := markTruncated(rs.seriesFrames(), rs, datasource.Limits{MaxRows: 2})
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if frames[0].Meta == nil || !frames[0].Meta.Truncated || frames[1].Meta != nil {
		t.Fatalf("metas = %+v, %+v: want truncated on the first frame only", frames[0].Meta, frames[1].Meta)
	}
}

func TestTableFrameAddsNanosecondTimesForADateTime64_9Timestamp(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 0, 0, 123456789, time.UTC)
	cases := []struct {
		name   string
		column column
		want   []string
	}{
		{"DateTime64(9)", column{name: "Timestamp", typ: datasource.FieldTime, dbType: "DateTime64(9)"}, []string{"time:Timestamp", "string:Body", "string:TimeNs"}},
		{"Nullable DateTime64(9)", column{name: "Timestamp", typ: datasource.FieldTime, dbType: "Nullable(DateTime64(9))"}, []string{"time:Timestamp", "string:Body", "string:TimeNs"}},
		{"DateTime64(3)", column{name: "Timestamp", typ: datasource.FieldTime, dbType: "DateTime64(3)"}, []string{"time:Timestamp", "string:Body"}},
		{"DateTime", column{name: "Timestamp", typ: datasource.FieldTime, dbType: "DateTime"}, []string{"time:Timestamp", "string:Body"}},
		{"another column name", column{name: "At", typ: datasource.FieldTime, dbType: "DateTime64(9)"}, []string{"time:At", "string:Body"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := &resultSet{columns: []column{tc.column, {name: "Body", typ: datasource.FieldString}}, rows: [][]any{{at, "line"}, {nil, "gap"}}}
			f := rs.tableFrame()
			var names []string
			for _, field := range f.Fields {
				names = append(names, string(field.Type)+":"+field.Name)
			}
			if fmt.Sprint(names) != fmt.Sprint(tc.want) {
				t.Fatalf("fields = %v, want %v", names, tc.want)
			}
			if len(tc.want) != 3 {
				return
			}
			if got := f.Fields[2].Values; len(got) != 2 || got[0] != "1793966400123456789" || got[1] != nil {
				t.Fatalf("TimeNs = %v", got)
			}
			if ns, ok := datasource.TimeNsOf(f, 0); !ok || ns != 1793966400123456789 {
				t.Fatalf("TimeNsOf = %d, %v", ns, ok)
			}
			if _, ok := datasource.TimeNsOf(f, 1); ok {
				t.Fatal("TimeNsOf found a value in a null row")
			}
		})
	}
}
