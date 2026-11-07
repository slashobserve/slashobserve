package elasticsearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

var rangeEnd = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

const (
	termsReply = `{"took":1,"timed_out":false,"hits":{"hits":[]},"aggregations":{"values":{"doc_count_error_upper_bound":0,"sum_other_doc_count":0,"buckets":[{"key":"frontend","doc_count":3},{"key":"checkout","doc_count":2},{"key":500,"key_as_string":"500","doc_count":1}]}}}`
	hitsReply  = `{"took":1,"timed_out":false,"hits":{"hits":[
		{"_index":"logs-shop-2026.11","_id":"a1","_source":{"@timestamp":"2026-11-06T11:59:59.123456789Z","message":"payment failed","log":{"level":"error"},"service":{"name":"checkout"},"trace":{"id":"abc"},"span":{"id":"def"},"labels":{"region":"eu"},"http":{"response":{"status_code":500}},"tags":["a","b"]}},
		{"_index":"logs-legacy-2026.11","_id":"b2","_source":{"@timestamp":1793966398000,"message":"legacy line","meta":{"text":"structured"},"log.level":"info","service.name":"legacy"}}
	]}}`
)

func searchBody(t *testing.T, r request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatalf("search body %q: %v", r.body, err)
	}
	return body
}

func termsField(t *testing.T, r request) string {
	t.Helper()
	body := searchBody(t, r)
	terms := body["aggs"].(map[string]any)["values"].(map[string]any)["terms"].(map[string]any)
	return terms["field"].(string)
}

func TestDiscoverLabelsAndColumns(t *testing.T) {
	ds := serve(t, healthyServer(t))
	labels, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabels})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"message", "message.keyword", "log.level", "service.name", "labels", "http.response.status_code", "host.ip", "@timestamp"} {
		if !slices.Contains(labels.Labels, want) {
			t.Errorf("labels %v lack %s", labels.Labels, want)
		}
	}
	for _, unwanted := range []string{"error.stack_trace", "error", "event", "_index"} {
		if slices.Contains(labels.Labels, unwanted) {
			t.Errorf("labels %v include %s", labels.Labels, unwanted)
		}
	}
	if !slices.IsSorted(labels.Labels) {
		t.Errorf("labels are not sorted: %v", labels.Labels)
	}

	searched, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Search: "LEVEL", Limit: 1})
	if err != nil || !slices.Equal(searched.Labels, []string{"log.level"}) {
		t.Fatalf("searched labels = %v, %v", searched, err)
	}

	columns, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Search: "message"})
	if err != nil {
		t.Fatal(err)
	}
	want := []datasource.Column{{Name: "message", Type: "text"}, {Name: "message.keyword", Type: "keyword"}}
	if !slices.Equal(columns.Columns, want) {
		t.Fatalf("columns = %v, want %v", columns.Columns, want)
	}
	all, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverColumns})
	if err != nil || !slices.ContainsFunc(all.Columns, func(c datasource.Column) bool { return c.Name == "event" && c.Type == "object" }) {
		t.Fatalf("columns = %v, %v, want the object field event", all, err)
	}
}

func TestDiscoverLabelValues(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_search", 200, termsReply)
	ds := serve(t, f)
	r := datasource.TimeRange{From: rangeEnd.Add(-time.Hour), To: rangeEnd}

	values, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "message", Range: r, Limit: 10})
	if err != nil || !slices.Equal(values.Values, []string{"500", "checkout", "frontend"}) {
		t.Fatalf("values = %v, %v", values, err)
	}
	req := f.last(t)
	if termsField(t, req) != "message.keyword" {
		t.Fatalf("terms field = %s, want message.keyword", termsField(t, req))
	}
	body := searchBody(t, req)
	if body["size"] != 0.0 || body["track_total_hits"] != false {
		t.Fatalf("body = %v", body)
	}
	window := body["query"].(map[string]any)["range"].(map[string]any)["@timestamp"].(map[string]any)
	if window["gte"] != float64(r.From.UnixMilli()) || window["lte"] != float64(r.To.UnixMilli()) || window["format"] != "epoch_millis" {
		t.Fatalf("range = %v", window)
	}
	terms := body["aggs"].(map[string]any)["values"].(map[string]any)["terms"].(map[string]any)
	if terms["size"] != 10.0 {
		t.Fatalf("terms size = %v, want the limit", terms["size"])
	}
	if req.params.Get("ignore_unavailable") != "true" || req.params.Get("allow_no_indices") != "true" {
		t.Fatalf("params = %v", req.params)
	}

	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "service.name", Search: "front"}); err != nil {
		t.Fatal(err)
	}
	req = f.last(t)
	if termsField(t, req) != "service.name" {
		t.Fatalf("terms field = %s", termsField(t, req))
	}
	terms = searchBody(t, req)["aggs"].(map[string]any)["values"].(map[string]any)["terms"].(map[string]any)
	if terms["size"] != float64(datasource.MaxDiscoverLimit) {
		t.Fatalf("terms size with a search = %v, want the maximum", terms["size"])
	}
	window = searchBody(t, req)["query"].(map[string]any)["range"].(map[string]any)["@timestamp"].(map[string]any)
	if span := window["lte"].(float64) - window["gte"].(float64); span != float64((24 * time.Hour).Milliseconds()) {
		t.Fatalf("default window = %v ms, want 24h", span)
	}

	for _, tt := range []struct{ label, want string }{
		{"error.stack_trace", "is text and has no keyword subfield"},
		{"event", "is not aggregatable"},
		{"nope", "is not in the mapping"},
	} {
		_, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: tt.label})
		if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("values of %s: error = %v, want %q", tt.label, err, tt.want)
		}
	}
}

func TestDiscoverTables(t *testing.T) {
	ds := serve(t, healthyServer(t))
	tables, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
	if err != nil {
		t.Fatal(err)
	}
	want := []datasource.Table{
		{Name: "logs", Kind: "alias"},
		{Name: "logs-generic-default", Kind: "data_stream"},
		{Name: "logs-legacy-2026.11", Kind: "index"},
		{Name: "logs-shop-2026.11", Kind: "index"},
	}
	if !slices.Equal(tables.Tables, want) {
		t.Fatalf("tables = %v, want %v", tables.Tables, want)
	}
	few, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables, Search: "shop", Limit: 1})
	if err != nil || !slices.Equal(few.Tables, want[3:]) {
		t.Fatalf("searched tables = %v, %v", few, err)
	}
}

func TestDiscoverSampleRows(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_search", 200, hitsReply)
	ds := serve(t, f)
	cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	body := searchBody(t, f.last(t))
	if body["size"] != 20.0 {
		t.Fatalf("size = %v, want the sample cap", body["size"])
	}
	sort := body["sort"].([]any)[0].(map[string]any)["@timestamp"].(map[string]any)
	if sort["order"] != "desc" || sort["unmapped_type"] != "date" {
		t.Fatalf("sort = %v", sort)
	}
	names := make([]string, len(cat.Sample.Fields))
	for i, field := range cat.Sample.Fields {
		names[i] = string(field.Type) + ":" + field.Name
	}
	want := []string{"time:Time", "string:TimeNs", "string:Body", "string:level", "string:ID", "string:Index", "string:service", "string:trace_id", "string:span_id", "string:Fields"}
	if !slices.Equal(names, want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	row := func(i int) map[string]any {
		out := map[string]any{}
		for _, field := range cat.Sample.Fields {
			out[field.Name] = field.Values[i]
		}
		return out
	}
	first := row(0)
	if first["Time"] != time.Date(2026, 11, 6, 11, 59, 59, 123456789, time.UTC) || first["TimeNs"] != "1793966399123456789" {
		t.Fatalf("first time = %v, %v", first["Time"], first["TimeNs"])
	}
	if first["Body"] != "payment failed" || first["level"] != "error" || first["ID"] != "a1" || first["Index"] != "logs-shop-2026.11" ||
		first["service"] != "checkout" || first["trace_id"] != "abc" || first["span_id"] != "def" {
		t.Fatalf("first row = %v", first)
	}
	if first["Fields"] != `{"http.response.status_code":500,"labels.region":"eu","tags":["a","b"]}` {
		t.Fatalf("first fields = %v", first["Fields"])
	}
	second := row(1)
	if second["Time"] != time.UnixMilli(1793966398000).UTC() || second["Body"] != "legacy line" || second["level"] != "info" ||
		second["service"] != "legacy" || second["trace_id"] != nil || second["Fields"] != `{"meta.text":"structured"}` {
		t.Fatalf("second row = %v", second)
	}

	plain := serve(t, f, func(o *elasticsearch.Options) { o.Fields.Service = "tags" })
	cat, err = plain.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows})
	if err != nil || searchBody(t, f.last(t))["size"] != 5.0 {
		t.Fatalf("default sample = %v, %v", cat, err)
	}
	if got := cat.Sample.Fields[6]; got.Name != "service" || got.Values[0] != `["a","b"]` {
		t.Fatalf("array role value = %+v", got)
	}
}

func TestDiscoverEmptyKindsAndUnknownKind(t *testing.T) {
	ds := serve(t, healthyServer(t))
	for _, kind := range []datasource.DiscoverKind{datasource.DiscoverMetrics, datasource.DiscoverInventory} {
		cat, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: kind, Scope: "x"})
		if err != nil || len(cat.Metrics) != 0 || len(cat.Families) != 0 {
			t.Fatalf("%s = %+v, %v", kind, cat, err)
		}
	}
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: "streams"}); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("unknown kind error = %v", err)
	}
}

func TestVariableValues(t *testing.T) {
	f := healthyServer(t)
	f.fixed(http.MethodPost, "/logs-*/_search", 200, termsReply)
	ds := serve(t, f)
	r := datasource.TimeRange{From: rangeEnd.Add(-time.Hour), To: rangeEnd}

	values, err := ds.VariableValues(context.Background(), datasource.Query{Text: "label_values(service.name)"}, r)
	if err != nil || !slices.Equal(values, []string{"500", "checkout", "frontend"}) {
		t.Fatalf("values = %v, %v", values, err)
	}
	req := f.last(t)
	terms := searchBody(t, req)["aggs"].(map[string]any)["values"].(map[string]any)["terms"].(map[string]any)
	if terms["field"] != "service.name" || terms["size"] != 10000.0 {
		t.Fatalf("terms = %v", terms)
	}
	window := searchBody(t, req)["query"].(map[string]any)["range"].(map[string]any)["@timestamp"].(map[string]any)
	if window["gte"] != float64(r.From.UnixMilli()) || window["lte"] != float64(r.To.UnixMilli()) {
		t.Fatalf("range = %v", window)
	}

	expanded, err := ds.VariableValues(context.Background(), datasource.Query{Text: "label_values(${f:csv})", Variables: []datasource.Variable{{Name: "f", Values: []string{"message"}}}}, r)
	if err != nil || len(expanded) != 3 || termsField(t, f.last(t)) != "message.keyword" {
		t.Fatalf("expanded = %v, %v, field %s", expanded, err, termsField(t, f.last(t)))
	}

	for _, text := range []string{"service.name", "label_values()", "label_values(a b)", "values(x)"} {
		if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: text}, r); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("%q: error = %v, want ErrInvalidQuery", text, err)
		}
	}
	if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: "label_values(error.stack_trace)"}, r); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("text field error = %v", err)
	}
	if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: "label_values(service.name)"}, datasource.TimeRange{}); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("empty range error = %v", err)
	}

	f.fixed(http.MethodPost, "/logs-*/_search", 200, strings.Replace(termsReply, `"sum_other_doc_count":0`, `"sum_other_doc_count":7`, 1))
	if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: "label_values(service.name)"}, r); !errors.Is(err, datasource.ErrLimitExceeded) {
		t.Fatalf("overflow error = %v, want ErrLimitExceeded", err)
	}

	f.fixed(http.MethodPost, "/logs-*/_search", 400, esError("query_shard_exception", "failed to create query"))
	if _, err := ds.VariableValues(context.Background(), datasource.Query{Text: "label_values(service.name)"}, r); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("upstream rejection = %v", err)
	}
}
