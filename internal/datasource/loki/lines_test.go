package loki_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

var (
	linesFrom  = time.Date(2026, 11, 6, 11, 0, 0, 123456789, time.UTC)
	linesTo    = time.Date(2026, 11, 6, 12, 0, 0, 5, time.UTC)
	linesRange = datasource.TimeRange{From: linesFrom, To: linesTo}
	forward    = success("streams", `[
		{"stream":{"service_name":"checkout"},"values":[["1793966400000000001","first"],["1793966400000000002","second"],["1793966400000000003","third"]]}
	]`)
)

func TestLinesSendsTheDirectionAndNanosecondBoundsOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		name      string
		direction datasource.LineDirection
		limit     int
		want      url.Values
	}{
		{"forward", datasource.Forward, 10, url.Values{"direction": {"forward"}, "limit": {"10"}}},
		{"backward", datasource.Backward, 10, url.Values{"direction": {"backward"}, "limit": {"10"}}},
		{"no direction is backward", "", 10, url.Values{"direction": {"backward"}, "limit": {"10"}}},
		{"limit below one is one", datasource.Forward, 0, url.Values{"direction": {"forward"}, "limit": {"1"}}},
		{"limit above the line cap is the cap", datasource.Forward, 10000, url.Values{"direction": {"forward"}, "limit": {"5000"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, 0, forward)
			if _, err := ds.Lines(context.Background(), datasource.LinesRequest{Query: datasource.Query{Text: `{service_name="checkout"}`}, Range: linesRange, Direction: tc.direction, Limit: tc.limit}); err != nil {
				t.Fatal(err)
			}
			req := fake.last(t)
			if req.path != "/loki/api/v1/query_range" {
				t.Fatalf("path = %q", req.path)
			}
			want := url.Values{
				"query": {`{service_name="checkout"}`},
				"start": {"2026-11-06T11:00:00.123456789Z"},
				"end":   {"2026-11-06T12:00:00.000000005Z"},
			}
			for k, v := range tc.want {
				want[k] = v
			}
			if fmt.Sprint(req.params) != fmt.Sprint(want) {
				t.Fatalf("params = %v, want %v", req.params, want)
			}
		})
	}
}

func TestLinesKeepsLokisOrderAndCarriesNanosecondTimes(t *testing.T) {
	ds, _ := serve(t, 0, forward)
	frames, err := ds.Lines(context.Background(), datasource.LinesRequest{Query: datasource.Query{Text: `{service_name="checkout"}`}, Range: linesRange, Direction: datasource.Forward, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	if got := fieldValues(t, frames[0], "Body"); !slices.Equal(got, []any{"first", "second", "third"}) {
		t.Fatalf("bodies = %v, want oldest first", got)
	}
	if got := fieldValues(t, frames[0], "TimeNs"); !slices.Equal(got, []any{"1793966400000000001", "1793966400000000002", "1793966400000000003"}) {
		t.Fatalf("TimeNs = %v", got)
	}
	if frames[0].Meta != nil {
		t.Fatalf("meta = %+v below the limit", frames[0].Meta)
	}
}

func TestLinesMarksTruncationAtTheLimit(t *testing.T) {
	ds, _ := serve(t, 0, forward)
	frames, err := ds.Lines(context.Background(), datasource.LinesRequest{Query: datasource.Query{Text: `{service_name="checkout"}`}, Range: linesRange, Direction: datasource.Forward, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Meta == nil || !frames[0].Meta.Truncated {
		t.Fatalf("frames = %+v, want the first frame truncated at 3 lines", frames)
	}
}

func TestLinesRefusesAMetricQueryAndABadRange(t *testing.T) {
	ds, fake := serve(t, 0, forward)
	_, err := ds.Lines(context.Background(), datasource.LinesRequest{Query: datasource.Query{Text: `rate({a="b"}[1m])`}, Range: linesRange, Direction: datasource.Forward, Limit: 10})
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want invalid query", err)
	}
	if want := `invalid query: lines need a log query that starts with a stream selector, such as {service_name="checkout"}`; err.Error() != want {
		t.Fatalf("err = %q, want %q", err, want)
	}
	_, err = ds.Lines(context.Background(), datasource.LinesRequest{Query: datasource.Query{Text: `{a="b"}`}, Range: datasource.TimeRange{From: linesTo, To: linesFrom}, Direction: datasource.Forward, Limit: 10})
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want invalid query for from after to", err)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("%d requests reached Loki for refused calls", len(fake.requests))
	}
}
