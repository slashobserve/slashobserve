package elasticsearch_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const (
	traceID = "0af7651916cd43dd8448eb211c80319c"
	spanID  = "b7ad6b7169203331"
)

func traceRequest(span, service string, limit int) datasource.TraceLogsRequest {
	return datasource.TraceLogsRequest{TraceID: traceID, SpanID: span, Service: service, Range: queryRange, Limit: limit}
}

func traceFilter(terms ...string) string {
	return `{"size": 50, "track_total_hits": false, "timeout": "30000ms", "sort": ` + defaultSort + `,
		"query": {"bool": {"filter": [` + rangeFilter + `, ` + strings.Join(terms, ", ") + `]}}}`
}

func TestTraceLogsFiltersOnTheResolvedRoles(t *testing.T) {
	traceTerm := `{"term": {"trace.id": "` + traceID + `"}}`
	spanTerm := `{"term": {"span.id": "` + spanID + `"}}`
	serviceTerm := `{"term": {"service.name": "checkout"}}`
	for _, tc := range []struct {
		name string
		req  datasource.TraceLogsRequest
		want string
	}{
		{"a trace", traceRequest("", "", 50), traceFilter(traceTerm)},
		{"a span of a trace", traceRequest(spanID, "", 50), traceFilter(traceTerm, spanTerm)},
		{"a service of a trace", traceRequest("", "checkout", 50), traceFilter(traceTerm, serviceTerm)},
		{"everything", traceRequest(spanID, "checkout", 50), traceFilter(traceTerm, spanTerm, serviceTerm)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ds := searchServer(t, hitsOf(2))
			frames, err := ds.TraceLogs(context.Background(), tc.req)
			if err != nil || len(frames) != 1 || frames[0].Rows() != 2 {
				t.Fatalf("frames = %v, err = %v", frames, err)
			}
			assertJSONEqual(t, f.last(t).body, tc.want)
		})
	}
}

func TestTraceLogsUsesTheConfiguredRoles(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, searchPath, http.StatusOK, hitsOf(1))
	ds := serve(t, f, func(o *elasticsearch.Options) { o.Fields.TraceID = "event.sequence" })
	if _, err := ds.TraceLogs(context.Background(), traceRequest("", "", 50)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.last(t).body, `{"term":{"event.sequence":"`+traceID+`"}}`) {
		t.Fatalf("body = %s", f.last(t).body)
	}
}

func TestTraceLogsMarksTruncationAtTheLimit(t *testing.T) {
	_, ds := searchServer(t, hitsOf(2))
	frames, err := ds.TraceLogs(context.Background(), traceRequest("", "", 2))
	if err != nil || frames[0].Meta == nil || !frames[0].Meta.Truncated {
		t.Fatalf("frames = %v, err = %v", frames, err)
	}
}

func TestTraceLogsRefusesWithoutTheRoleItNeeds(t *testing.T) {
	noTrace := strings.NewReplacer(`"trace.id"`, `"trace.gone"`, `"trace_id"`, `"trace_gone"`).Replace(fieldCaps(t))
	noSpan := strings.NewReplacer(`"span.id"`, `"span.gone"`).Replace(fieldCaps(t))
	for _, tc := range []struct {
		name string
		caps string
		req  datasource.TraceLogsRequest
		want string
	}{
		{"no trace id field", noTrace, traceRequest("", "", 50), "no trace id field in the mapping; set fields.traceId"},
		{"no span id field", noSpan, traceRequest(spanID, "", 50), "no span id field in the mapping; set fields.spanId"},
		{"an invalid trace id", fieldCaps(t), datasource.TraceLogsRequest{TraceID: "xyz", Range: queryRange, Limit: 5}, "trace_id must be 16 or 32 hex characters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := healthyServer(t)
			f.fixed(http.MethodPost, "/logs-*/_field_caps", http.StatusOK, tc.caps)
			f.fixed(http.MethodPost, searchPath, http.StatusOK, hitsOf(1))
			ds := serve(t, f)
			_, err := ds.TraceLogs(context.Background(), tc.req)
			if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if f.count(searchPath) != 0 {
				t.Fatal("a refused request reached the search endpoint")
			}
		})
	}
}
