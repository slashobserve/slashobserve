package loki_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	patternsPath = "/loki/api/v1/patterns"
	lokiPatterns = `{"status":"success","data":[{"pattern":"level=error msg=\"payment declined\" order_id=<_> gateway=stripe code=card_declined duration_ms=<_>","level":"error","samples":[[1793992190,60],[1793992200,2]]},{"pattern":"ts=<_> level=info msg=<_> duration_ms=<_> client=<_>","level":"info","samples":[[1793992190,60]]}]}`
)

func patternRequest(query string, step time.Duration) datasource.PatternRequest {
	return datasource.PatternRequest{Query: datasource.Query{Text: query}, Range: linesRange, Step: step}
}

func TestLogPatternsSendsTheSelectorNanosecondBoundsAndStepOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		name string
		step time.Duration
		want string
	}{
		{"a minute", time.Minute, "60s"},
		{"a step below a second is a second", 100 * time.Millisecond, "1s"},
		{"no step is a second", 0, "1s"},
		{"a fraction rounds down", 90 * time.Second, "90s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, fake := serve(t, 0, lokiPatterns)
			if _, err := ds.LogPatterns(context.Background(), patternRequest(` {service_name="checkout"} `, tc.step)); err != nil {
				t.Fatal(err)
			}
			req := fake.last(t)
			if req.path != patternsPath {
				t.Fatalf("path = %q", req.path)
			}
			want := url.Values{
				"query": {`{service_name="checkout"}`},
				"start": {"1793962800123456789"},
				"end":   {"1793966400000000005"},
				"step":  {tc.want},
			}
			if fmt.Sprint(req.params) != fmt.Sprint(want) {
				t.Fatalf("params = %v, want %v", req.params, want)
			}
		})
	}
}

func TestLogPatternsDecodesPatternsAndRewritesTheWildcard(t *testing.T) {
	ds, _ := serve(t, 0, lokiPatterns)
	got, err := ds.LogPatterns(context.Background(), patternRequest(`{service_name="checkout"}`, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d patterns, want 2", len(got))
	}
	first := got[0]
	if first.Pattern != `level=error msg="payment declined" order_id=<*> gateway=stripe code=card_declined duration_ms=<*>` {
		t.Fatalf("pattern = %q", first.Pattern)
	}
	if len(first.Samples) != 2 || first.Samples[0].Count != 60 || first.Samples[1].Count != 2 {
		t.Fatalf("samples = %+v", first.Samples)
	}
	if want := time.Unix(1793992190, 0).UTC(); !first.Samples[0].At.Equal(want) {
		t.Fatalf("sample at %v, want %v", first.Samples[0].At, want)
	}
	if got[1].Pattern != "ts=<*> level=info msg=<*> duration_ms=<*> client=<*>" {
		t.Fatalf("second pattern = %q", got[1].Pattern)
	}
}

func TestLogPatternsAnswersNotFoundWhenLokiCannotServePatterns(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		query  string
	}{
		{"a 404 from a Loki without the pattern ingester", http.StatusNotFound, "404 page not found", `{service_name="checkout"}`},
		{"a success envelope without data", http.StatusOK, `{"status":"success"}`, `{service_name="checkout"}`},
		{"a query with a line filter the API refuses", http.StatusOK, lokiPatterns, `{service_name="checkout"} |= "error"`},
		{"a query with a parser stage", http.StatusOK, lokiPatterns, `{service_name="checkout"} | json`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds, _ := serve(t, tc.status, tc.body)
			_, err := ds.LogPatterns(context.Background(), patternRequest(tc.query, time.Minute))
			if !errors.Is(err, datasource.ErrNotFound) {
				t.Fatalf("err = %v, want not found so the caller computes patterns itself", err)
			}
		})
	}
}

func TestLogPatternsRefusesAMetricQueryAndReportsOtherStatuses(t *testing.T) {
	ds, fake := serve(t, 0, lokiPatterns)
	_, err := ds.LogPatterns(context.Background(), patternRequest(`rate({service_name="checkout"}[1m])`, time.Minute))
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want invalid query", err)
	}
	if want := "invalid query: patterns need a log query that starts with a stream selector"; err.Error() != want {
		t.Fatalf("err = %q, want %q", err, want)
	}
	if len(fake.requests) != 0 {
		t.Fatalf("%d requests reached Loki for a refused query", len(fake.requests))
	}
	ds, _ = serve(t, http.StatusBadRequest, "only label matchers are supported")
	_, err = ds.LogPatterns(context.Background(), patternRequest(`{service_name="checkout"}`, time.Minute))
	if !errors.Is(err, datasource.ErrInvalidQuery) || errors.Is(err, datasource.ErrNotFound) {
		t.Fatalf("err = %v, want the upstream rejection as an invalid query", err)
	}
	ds, _ = serve(t, http.StatusOK, `{"status":"success","data":[{"pattern":1}]}`)
	if _, err = ds.LogPatterns(context.Background(), patternRequest(`{service_name="checkout"}`, time.Minute)); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want unreadable as unavailable", err)
	}
}
