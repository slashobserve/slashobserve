package loki_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
)

const (
	traceID = "0af7651916cd43dd8448eb211c80319c"
	spanID  = "b7ad6b7169203331"
)

var (
	traceWindow = datasource.TimeRange{From: rangeEnd.Add(-time.Minute), To: rangeEnd}
	traceLine   = success("streams", `[{"stream":{"service_name":"checkout"},"values":[["1793966400000000003","charge failed trace_id=`+traceID+`"]]}]`)
	noLines     = success("streams", "[]")
)

type queryLoki struct {
	mu      sync.Mutex
	labels  string
	answers map[string]string
	queries []string
}

func (f *queryLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case labelsPath:
		_, _ = w.Write([]byte(f.labels))
	case rangePath:
		q := r.URL.Query()
		f.queries = append(f.queries, q.Get("query"))
		if q.Get("direction") != "backward" || q.Get("limit") != "50" || q.Get("start") == "" || q.Get("end") == "" {
			http.Error(w, "not a bounded backward line query", http.StatusBadRequest)
			return
		}
		body, ok := f.answers[q.Get("query")]
		if !ok {
			body = noLines
		}
		_, _ = w.Write([]byte(body))
	default:
		http.NotFound(w, r)
	}
}

func (f *queryLoki) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queries...)
}

func serveTraceLogs(t *testing.T, labels []string, answers map[string]string) (*loki.Datasource, *queryLoki) {
	t.Helper()
	fake := &queryLoki{labels: labelList(labels...), answers: answers}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ds, err := loki.New(loki.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds, fake
}

func traceRequest(service, span string) datasource.TraceLogsRequest {
	return datasource.TraceLogsRequest{TraceID: traceID, SpanID: span, Service: service, Range: traceWindow, Limit: 50}
}

func TestTraceLogsBuildTheSelectorAndFallBackToALineFilterOnlyWhenEmpty(t *testing.T) {
	tests := []struct {
		name      string
		labels    []string
		req       datasource.TraceLogsRequest
		answered  string
		wantSent  []string
		wantLines int
	}{
		{
			name:      "service narrows the selector and the metadata filter answers",
			labels:    []string{"app", "service_name", "level"},
			req:       traceRequest("checkout", ""),
			answered:  `{service_name="checkout"} | trace_id="` + traceID + `"`,
			wantSent:  []string{`{service_name="checkout"} | trace_id="` + traceID + `"`},
			wantLines: 1,
		},
		{
			name:     "the first service label in preference order wins",
			labels:   []string{"job", "app"},
			req:      traceRequest("checkout", ""),
			wantSent: []string{`{app="checkout"} | trace_id="` + traceID + `"`, `{app="checkout"} |= "` + traceID + `"`},
		},
		{
			name:      "no service uses the sample label over every stream",
			labels:    []string{"level", "job", "pod"},
			req:       traceRequest("", ""),
			answered:  `{job=~".+"} | trace_id="` + traceID + `"`,
			wantSent:  []string{`{job=~".+"} | trace_id="` + traceID + `"`},
			wantLines: 1,
		},
		{
			name:      "a span id narrows both queries",
			labels:    []string{"service_name"},
			req:       traceRequest("checkout", spanID),
			answered:  `{service_name="checkout"} |= "` + traceID + `" |= "` + spanID + `"`,
			wantSent:  []string{`{service_name="checkout"} | trace_id="` + traceID + `" | span_id="` + spanID + `"`, `{service_name="checkout"} |= "` + traceID + `" |= "` + spanID + `"`},
			wantLines: 1,
		},
		{
			name:     "a service with quotes is escaped",
			labels:   []string{"service_name"},
			req:      traceRequest(`a "quoted" \ name`, ""),
			wantSent: []string{`{service_name="a \"quoted\" \\ name"} | trace_id="` + traceID + `"`, `{service_name="a \"quoted\" \\ name"} |= "` + traceID + `"`},
		},
		{
			name:     "a service that no service label carries falls back to every stream",
			labels:   []string{"level", "pod"},
			req:      traceRequest("checkout", ""),
			wantSent: []string{`{level=~".+"} | trace_id="` + traceID + `"`, `{level=~".+"} |= "` + traceID + `"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answers := map[string]string{}
			if tt.answered != "" {
				answers[tt.answered] = traceLine
			}
			ds, fake := serveTraceLogs(t, tt.labels, answers)
			frames, err := ds.TraceLogs(context.Background(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			lines := 0
			for _, f := range frames {
				lines += f.Rows()
			}
			if lines != tt.wantLines {
				t.Fatalf("%d lines, want %d", lines, tt.wantLines)
			}
			if got := fake.sent(); len(got) != len(tt.wantSent) || !equalStrings(got, tt.wantSent) {
				t.Fatalf("queries = %q, want %q", got, tt.wantSent)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTraceLogsAnswerNoLinesWhenLokiHasNoStreamLabels(t *testing.T) {
	ds, fake := serveTraceLogs(t, nil, nil)
	frames, err := ds.TraceLogs(context.Background(), traceRequest("", ""))
	if err != nil || len(frames) != 0 {
		t.Fatalf("frames = %v, err = %v, want none", frames, err)
	}
	if sent := fake.sent(); len(sent) != 0 {
		t.Fatalf("queries ran without a stream label: %q", sent)
	}
}

func TestTraceLogsRejectBadIdsBeforeCallingLoki(t *testing.T) {
	ds, fake := serveTraceLogs(t, []string{"service_name"}, nil)
	tests := []struct {
		name string
		req  datasource.TraceLogsRequest
		want string
	}{
		{"short trace id", datasource.TraceLogsRequest{TraceID: "abc", Range: traceWindow, Limit: 50}, "trace_id must be 16 or 32 hex characters"},
		{"non-hex trace id", datasource.TraceLogsRequest{TraceID: "zz" + traceID[2:], Range: traceWindow, Limit: 50}, "trace_id must be 16 or 32 hex characters"},
		{"short span id", datasource.TraceLogsRequest{TraceID: traceID, SpanID: "b7ad", Range: traceWindow, Limit: 50}, "span_id must be 16 hex characters"},
		{"no limit", datasource.TraceLogsRequest{TraceID: traceID, Range: traceWindow}, "limit must be at least 1"},
		{"no range", datasource.TraceLogsRequest{TraceID: traceID, Limit: 50}, "time range requires from and to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ds.TraceLogs(context.Background(), tt.req)
			if !errors.Is(err, datasource.ErrInvalidQuery) || err.Error() != "invalid query: "+tt.want {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
	if sent := fake.sent(); len(sent) != 0 {
		t.Fatalf("queries ran for rejected requests: %q", sent)
	}
}
