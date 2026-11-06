package clickhouse

import (
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
