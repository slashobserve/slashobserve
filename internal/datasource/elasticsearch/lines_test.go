package elasticsearch_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const (
	ascSort  = `[{"@timestamp":{"order":"asc","numeric_type":"date_nanos","unmapped_type":"date"}},{"_doc":"asc"}]`
	linesAsc = `{"size": 3, "track_total_hits": false, "timeout": "30000ms", "sort": ` + ascSort + `,
		"query": {"bool": {"filter": [` + rangeFilter + `, {"match": {"service.name": "checkout"}}]}}}`
	linesDesc = `{"size": 3, "track_total_hits": false, "timeout": "30000ms", "sort": ` + defaultSort + `,
		"query": {"bool": {"filter": [` + rangeFilter + `, {"match": {"service.name": "checkout"}}]}}}`
)

func linesRequest(direction datasource.LineDirection, limit int, text string) datasource.LinesRequest {
	return datasource.LinesRequest{Query: datasource.Query{Text: text}, Range: queryRange, Direction: direction, Limit: limit}
}

func TestLinesSortsByDirectionAndBoundsTheRange(t *testing.T) {
	for _, tc := range []struct {
		name      string
		direction datasource.LineDirection
		want      string
	}{
		{"forward reads the oldest first", datasource.Forward, linesAsc},
		{"backward reads the newest first", datasource.Backward, linesDesc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := searchServer(t, hitsOf(2))
			if _, err := ds.Lines(context.Background(), linesRequest(tc.direction, 3, "service.name:checkout")); err != nil {
				t.Fatal(err)
			}
			assertJSONEqual(t, f.last(t).body, tc.want)
		})
	}
}

func TestLinesIgnoresTheSortAndSizeOfAQueryBody(t *testing.T) {
	f, ds := searchServer(t, hitsOf(1))
	text := `{"size": 7, "sort": [{"other": "asc"}], "query": {"match_all": {}}}`
	if _, err := ds.Lines(context.Background(), linesRequest(datasource.Backward, 3, text)); err != nil {
		t.Fatal(err)
	}
	body := searchBody(t, f.last(t))
	if body["size"] != 3.0 {
		t.Fatalf("size = %v, want the request limit", body["size"])
	}
	assertJSONEqual(t, mustJSON(t, body["sort"]), defaultSort)
}

func TestLinesCapsTheLimitAtTheLogLineLimit(t *testing.T) {
	f, ds := searchServer(t, hitsOf(1))
	if _, err := ds.Lines(context.Background(), linesRequest(datasource.Backward, 100000, "")); err != nil {
		t.Fatal(err)
	}
	if got := searchBody(t, f.last(t))["size"]; got != 5000.0 {
		t.Fatalf("size = %v, want 5000", got)
	}
}

func TestLinesMarksTruncationAtTheLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hits  int
		limit int
		want  bool
	}{
		{"fewer lines than the limit", 2, 3, false},
		{"as many lines as the limit", 3, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ds := searchServer(t, hitsOf(tc.hits))
			frames, err := ds.Lines(context.Background(), linesRequest(datasource.Forward, tc.limit, ""))
			if err != nil || len(frames) != 1 {
				t.Fatalf("frames = %v, err = %v", frames, err)
			}
			if frames[0].Rows() != tc.hits || frames[0].Meta == nil || frames[0].Meta.Truncated != tc.want {
				t.Fatalf("rows = %d, meta = %+v, want truncated %v", frames[0].Rows(), frames[0].Meta, tc.want)
			}
		})
	}
}

func TestLinesRefusesWhatItCannotReadBeforeSearching(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  datasource.LinesRequest
	}{
		{"no limit", linesRequest(datasource.Forward, 0, "")},
		{"an empty range", datasource.LinesRequest{Range: datasource.TimeRange{From: queryRange.To, To: queryRange.From}, Limit: 5}},
		{"a body with aggregations", linesRequest(datasource.Forward, 5, `{"aggs": {"x": {"terms": {"field": "service.name"}}}}`)},
		{"a query that does not parse", linesRequest(datasource.Forward, 5, "service.name:")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := searchServer(t, hitsOf(1))
			if _, err := ds.Lines(context.Background(), tc.req); !errors.Is(err, datasource.ErrInvalidQuery) {
				t.Fatalf("error = %v, want invalid query", err)
			}
			if f.count(searchPath) != 0 {
				t.Fatal("a refused request reached the search endpoint")
			}
		})
	}
}

func TestLinesOnADateFieldTruncatesTheTimeToMilliseconds(t *testing.T) {
	reply := `{"hits":{"hits":[{"_index":"logs-shop-2026.11","_id":"a","_source":{"event":{"ingested":"2026-11-06T11:59:59.123456789Z"},"message":"m"}}]}}`
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, reply)
	ds := serve(t, f, func(o *elasticsearch.Options) { o.TimeField = "event.ingested" })
	frames, err := ds.Lines(context.Background(), linesRequest(datasource.Forward, 5, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 6, 11, 59, 59, 123000000, time.UTC)
	if got := frames[0].Fields[0].Values[0]; got != want {
		t.Fatalf("Time = %v, want %v", got, want)
	}
}

func TestTailLagIsFiveSeconds(t *testing.T) {
	_, ds := searchServer(t, hitsOf(0))
	if got := ds.TailLag(); got != 5*time.Second {
		t.Fatalf("TailLag = %v, want 5s", got)
	}
}
