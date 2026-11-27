package elasticsearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const fakeSpanIndex = "jaeger-span-*"

func serveSpans(t *testing.T, f *fakeServer) *elasticsearch.SpanIndex {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	ds, err := elasticsearch.Open(elasticsearch.Options{Flavor: elasticsearch.OpenSearch, URL: srv.URL, Index: fakeSpanIndex, Spans: elasticsearch.SpansJaeger})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds.(*elasticsearch.SpanIndex)
}

func capturedHits(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/jaeger_spans.json")
	if err != nil {
		t.Fatal(err)
	}
	var docs []json.RawMessage
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatal(err)
	}
	hits := make([]string, len(docs))
	for i, d := range docs {
		hits[i] = `{"_source":` + string(d) + `,"sort":[` + string(rune('1'+i)) + `,"x"]}`
	}
	return `{"hits":{"total":{"value":4},"hits":[` + strings.Join(hits, ",") + `]}}`
}

func TestOpenMakesASpanIndexOnlyWhenTheOptionsSaySo(t *testing.T) {
	base := elasticsearch.Options{Flavor: elasticsearch.OpenSearch, URL: "http://search:9200", Index: "jaeger-span-*"}
	plain, err := elasticsearch.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain.(datasource.TraceSource); ok {
		t.Error("a plain index answers trace reads")
	}
	withSpans := base
	withSpans.Spans = elasticsearch.SpansJaeger
	spans, err := elasticsearch.Open(withSpans)
	if err != nil {
		t.Fatal(err)
	}
	index, ok := spans.(*elasticsearch.SpanIndex)
	if !ok {
		t.Fatalf("spans: jaeger opened a %T", spans)
	}
	if _, field := index.Index(); field != elasticsearch.JaegerTimeField || index.SpanFormat() != "jaeger" || datasource.SignalsOf(index)[0] != datasource.SignalTraces {
		t.Errorf("time field %q, format %q, signals %v", field, index.SpanFormat(), datasource.SignalsOf(index))
	}
	for _, mutate := range []func(*elasticsearch.Options){
		func(o *elasticsearch.Options) { o.Spans = "zipkin" },
		func(o *elasticsearch.Options) { o.TimeField = "@timestamp" },
	} {
		bad := withSpans
		mutate(&bad)
		var config *datasource.ConfigError
		if _, err := elasticsearch.Open(bad); !errors.As(err, &config) {
			t.Errorf("%+v opened: %v", bad, err)
		}
	}
}

func TestSpanTraceAsksForEveryFormJaegerStoresAnIDIn(t *testing.T) {
	f := &fakeServer{routes: map[string]func(request) reply{}}
	f.fixed(http.MethodPost, "/"+fakeSpanIndex+"/_search", http.StatusOK, capturedHits(t))
	ds := serveSpans(t, f)
	frame, err := ds.Trace(context.Background(), "0000000000000000"+"00f7651916cd43dd", datasource.TimeRange{From: time.UnixMilli(0), To: time.UnixMilli(1000)}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Rows() != 4 || frame.Meta.Truncated {
		t.Fatalf("rows = %d, meta %+v", frame.Rows(), frame.Meta)
	}
	var body struct {
		Query struct {
			Bool struct {
				Filter []map[string]map[string]any `json:"filter"`
			} `json:"bool"`
		} `json:"query"`
		TrackTotalHits bool `json:"track_total_hits"`
	}
	if err := json.Unmarshal([]byte(f.last(t).body), &body); err != nil {
		t.Fatal(err)
	}
	ids, _ := json.Marshal(body.Query.Bool.Filter[1]["terms"]["traceID"])
	if string(ids) != `["000000000000000000f7651916cd43dd","f7651916cd43dd","00f7651916cd43dd"]` || !body.TrackTotalHits {
		t.Errorf("ids = %s, body %s", ids, f.last(t).body)
	}
}

func TestSpanIndexRefusesToReadLogLines(t *testing.T) {
	ds := serveSpans(t, &fakeServer{routes: map[string]func(request) reply{}})
	if _, err := ds.TraceLogs(context.Background(), datasource.TraceLogsRequest{TraceID: traceID}); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Errorf("TraceLogs = %v", err)
	}
}
