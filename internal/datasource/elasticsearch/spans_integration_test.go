package elasticsearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
)

const (
	spanIndex        = "jaeger-main-jaeger-span-test"
	spanIndexPattern = "jaeger-main-jaeger-span-*"
	capturedTrace    = "0af7651916cd43dd8448eb211c80319c"
	legacyTrace      = "1d2c3b4a5968f7e6"
	wideTrace        = "b16b16b16b16b16b16b16b16b16b1600"
	checkoutTraces   = 40
	wideSpans        = 6000
	pagedSpans       = 5500
)

func checkoutTraceID(i int) string { return fmt.Sprintf("%032x", 0xc0de0000+i) }

func checkoutRootMillis(i int) int64 { return int64(i+1) * 10 }

// seedSpans indexes span documents in the shape Jaeger 2.21 writes, with the index mapping it creates: the four captured from a Jaeger writing to OpenSearch, a trace in the tags-as-fields form under a 64-bit id, a checkout load and one trace wider than a search page.
func seedSpans(ctx context.Context, s server) error {
	mapping, err := os.ReadFile("testdata/jaeger_span_mapping.json")
	if err != nil {
		return err
	}
	if err := send(ctx, http.MethodPut, s.url+"/"+spanIndex, "application/json", `{"mappings":`+string(mapping)+`}`); err != nil {
		return fmt.Errorf("create %s: %w", spanIndex, err)
	}
	docs, err := spanDocuments()
	if err != nil {
		return err
	}
	var bulk strings.Builder
	for _, doc := range docs {
		encoded, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		fmt.Fprintf(&bulk, `{"index":{"_index":%q}}`+"\n%s\n", spanIndex, encoded)
	}
	return send(ctx, http.MethodPost, s.url+"/_bulk?refresh=true", "application/x-ndjson", bulk.String())
}

func spanDocuments() ([]map[string]any, error) {
	raw, err := os.ReadFile("testdata/jaeger_spans.json")
	if err != nil {
		return nil, err
	}
	var captured []map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&captured); err != nil {
		return nil, err
	}
	base := indexedAt.UnixMicro()
	first, _ := captured[0]["startTime"].(json.Number).Int64()
	for _, doc := range captured {
		start, _ := doc["startTime"].(json.Number).Int64()
		at := base + start - first
		doc["startTime"], doc["startTimeMillis"] = at, at/1000
		if logs, ok := doc["logs"].([]any); ok {
			for _, l := range logs {
				entry := l.(map[string]any)
				ts, _ := entry["timestamp"].(json.Number).Int64()
				entry["timestamp"] = base + ts - first
			}
		}
	}
	return slices.Concat(captured, legacyDocuments(base+5_000_000), checkoutDocuments(base+10_000_000), wideDocuments(base+60_000_000)), nil
}

func span(trace, id, parent, service, operation string, start, durationMicros int64) map[string]any {
	doc := map[string]any{
		"traceID": trace, "spanID": id, "operationName": operation,
		"startTime": start, "startTimeMillis": start / 1000, "duration": durationMicros,
		"tags": []any{}, "process": map[string]any{"serviceName": service, "tags": []any{}},
	}
	if parent != "" {
		doc["parentSpanID"] = parent
		doc["references"] = []any{map[string]any{"refType": "CHILD_OF", "traceID": trace, "spanID": parent}}
	}
	return doc
}

func legacyDocuments(start int64) []map[string]any {
	root := span(legacyTrace, "aa01", "", "legacy", "GET /old", start, 80_000)
	root["tag"] = map[string]any{"span@kind": "server", "http@status_code": "503", "otel@status_code": "ERROR", "otel@status_description": "backend gone"}
	root["process"] = map[string]any{"serviceName": "legacy", "tag": map[string]any{"host@name": "old-1"}}
	child := span(legacyTrace, "aa02", "", "legacy", "SELECT", start+1000, 70_000)
	child["references"] = []any{map[string]any{"refType": "CHILD_OF", "traceID": legacyTrace, "spanID": "aa01"}}
	child["tag"] = map[string]any{"span@kind": "client"}
	return []map[string]any{root, child}
}

func checkoutDocuments(start int64) []map[string]any {
	var docs []map[string]any
	for i := range checkoutTraces {
		trace, at := checkoutTraceID(i), start+int64(i)*1_000_000
		root := span(trace, fmt.Sprintf("%016x", 0x100+i), "", "checkout", "POST /checkout", at, checkoutRootMillis(i)*1000)
		root["tag"] = map[string]any{"span@kind": "server"}
		root["tags"] = []any{map[string]any{"key": "http.route", "type": "string", "value": "/checkout"}}
		child := span(trace, fmt.Sprintf("%016x", 0x200+i), fmt.Sprintf("%016x", 0x100+i), "payment", "charge", at+1000, checkoutRootMillis(i)*500)
		tag := map[string]any{"span@kind": "client"}
		if i%4 == 0 {
			tag["error"] = true
		}
		child["tag"] = tag
		docs = append(docs, root, child)
	}
	return docs
}

func wideDocuments(start int64) []map[string]any {
	docs := []map[string]any{span(wideTrace, "0000000000000c00", "", "batch", "run", start, 100_000)}
	for i := 1; i < wideSpans; i++ {
		docs = append(docs, span(wideTrace, fmt.Sprintf("%016x", 0xc00+i), "0000000000000c00", "batch", "step", start+int64(i)*10, 10))
	}
	return docs
}

func spanDS(t *testing.T, flavor elasticsearch.Flavor) *elasticsearch.SpanIndex {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	s, ok := servers[flavor]
	if !ok {
		t.Fatalf("no %s server", flavor)
	}
	ds, err := elasticsearch.Open(elasticsearch.Options{Flavor: flavor, URL: s.url, Index: spanIndexPattern, Spans: elasticsearch.SpansJaeger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds.(*elasticsearch.SpanIndex)
}

func spanRows(f datasource.Frame) []map[string]any {
	rows := make([]map[string]any, f.Rows())
	for i := range rows {
		rows[i] = map[string]any{}
		for _, field := range f.Fields {
			rows[i][field.Name] = field.Values[i]
		}
	}
	return rows
}

func rowOf(t *testing.T, rows []map[string]any, spanID string) map[string]any {
	t.Helper()
	for _, r := range rows {
		if r["SpanId"] == spanID {
			return r
		}
	}
	t.Fatalf("no span %s in %v", spanID, rows)
	return nil
}

func TestIntegrationSpanTraceReadsWhatJaegerWrote(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		f, err := spanDS(t, flavor).Trace(context.Background(), capturedTrace, aroundIndexing(), datasource.TraceSpanBudget)
		if err != nil {
			t.Fatal(err)
		}
		rows := spanRows(f)
		if len(rows) != 4 || f.Meta.Truncated {
			t.Fatalf("rows = %d, meta %+v", len(rows), f.Meta)
		}
		root := rowOf(t, rows, "b7ad6b7169203331")
		if root["ParentSpanId"] != "" || root["ServiceName"] != "frontend" || root["SpanName"] != "GET /api/cart" || root["StatusCode"] != "Error" || root["StatusMessage"] != "upstream failed" || root["SpanKind"] != "Server" {
			t.Errorf("root = %v", root)
		}
		if root["Duration"] != int64(250*time.Millisecond) || !strings.Contains(root["SpanAttributes"].(string), `"http.response.status_code":500`) || strings.Contains(root["SpanAttributes"].(string), `"error"`) {
			t.Errorf("root duration and attributes = %v %v", root["Duration"], root["SpanAttributes"])
		}
		if res := root["ResourceAttributes"].(string); !strings.Contains(res, `"service.name":"frontend"`) || !strings.Contains(res, `"host.name":"fe-1"`) {
			t.Errorf("root resource = %s", res)
		}
		client := rowOf(t, rows, "c7ad6b7169203332")
		if client["ParentSpanId"] != "b7ad6b7169203331" || client["SpanKind"] != "Client" || !strings.Contains(client["SpanLinks"].(string), "1af7651916cd43dd8448eb211c80319c") {
			t.Errorf("client = %v", client)
		}
		server := rowOf(t, rows, "e7ad6b7169203333")
		if server["ServiceName"] != "cart" || server["StatusMessage"] != "redis timeout" || !strings.Contains(server["SpanEvents"].(string), `"name":"exception"`) || !strings.Contains(server["SpanAttributes"].(string), `"app.retries":3`) {
			t.Errorf("cart server = %v", server)
		}
		if call := rowOf(t, rows, "f7ad6b7169203334"); call["StatusCode"] != "Ok" || call["ParentSpanId"] != "e7ad6b7169203333" {
			t.Errorf("HGET = %v", call)
		}
	})
}

func TestIntegrationSpanTraceReadsTagsAsFieldsUnderAPaddedID(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		f, err := spanDS(t, flavor).Trace(context.Background(), "0000000000000000"+legacyTrace, aroundIndexing(), datasource.TraceSpanBudget)
		if err != nil {
			t.Fatal(err)
		}
		rows := spanRows(f)
		if len(rows) != 2 {
			t.Fatalf("rows = %v", rows)
		}
		root := rowOf(t, rows, "000000000000aa01")
		if root["TraceId"] != legacyTrace || root["StatusCode"] != "Error" || root["StatusMessage"] != "backend gone" || root["SpanKind"] != "Server" || !strings.Contains(root["SpanAttributes"].(string), `"http.status_code":"503"`) || !strings.Contains(root["ResourceAttributes"].(string), `"host.name":"old-1"`) {
			t.Errorf("root = %v", root)
		}
		if child := rowOf(t, rows, "000000000000aa02"); child["ParentSpanId"] != "000000000000aa01" || child["StatusCode"] != "Unset" {
			t.Errorf("child = %v", child)
		}
	})
}

func TestIntegrationSpanTraceKeepsTheEarliestSpansOfAWideTrace(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := spanDS(t, flavor)
		f, err := ds.Trace(context.Background(), wideTrace, aroundIndexing(), 10)
		if err != nil {
			t.Fatal(err)
		}
		if f.Rows() != 10 || !f.Meta.Truncated || f.Meta.RowsTotal != wideSpans {
			t.Fatalf("rows = %d, meta %+v", f.Rows(), f.Meta)
		}
		if rows := spanRows(f); rows[0]["SpanName"] != "run" || rows[9]["SpanId"] != fmt.Sprintf("%016x", 0xc00+9) {
			t.Errorf("kept = %v ... %v", rows[0], rows[9])
		}
		paged, err := ds.Trace(context.Background(), wideTrace, aroundIndexing(), pagedSpans)
		if err != nil || paged.Rows() != pagedSpans || !paged.Meta.Truncated || paged.Meta.RowsTotal != wideSpans {
			t.Fatalf("paged = %d %+v %v", paged.Rows(), paged.Meta, err)
		}
		if last := spanRows(paged)[pagedSpans-1]; last["SpanId"] != fmt.Sprintf("%016x", 0xc00+pagedSpans-1) {
			t.Errorf("last paged span = %v", last)
		}
		whole, err := ds.Trace(context.Background(), wideTrace, aroundIndexing(), wideSpans)
		if err != nil || whole.Rows() != wideSpans || whole.Meta.Truncated {
			t.Fatalf("whole = %d %+v %v", whole.Rows(), whole.Meta, err)
		}
		_, err = ds.Trace(context.Background(), "ffffffffffffffffffffffffffffffff", aroundIndexing(), 10)
		if !errors.Is(err, datasource.ErrNotFound) {
			t.Fatalf("missing trace = %v", err)
		}
	})
}

func TestIntegrationSpanServicesAndAttributes(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := spanDS(t, flavor)
		ctx := context.Background()
		list, err := ds.Services(ctx, datasource.ServiceRequest{Range: aroundIndexing(), Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"batch", "cart", "checkout", "frontend", "legacy", "payment"}
		if list.Signal != datasource.SignalTraces || list.Field != "process.serviceName" || !slices.Equal(list.Values, want) {
			t.Errorf("services = %+v", list)
		}
		if _, err := ds.Services(ctx, datasource.ServiceRequest{Signal: datasource.SignalLogs, Range: aroundIndexing()}); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("logs signal = %v", err)
		}
		sets, err := ds.TraceAttributes(ctx, datasource.TraceAttributesRequest{IDs: []string{capturedTrace, checkoutTraceID(0), "ffffffffffffffffffffffffffffffff"}, Range: aroundIndexing(), MaxSpans: datasource.CompareSpanBudget})
		if err != nil {
			t.Fatal(err)
		}
		if len(sets) != 2 || sets[0].ID != capturedTrace {
			t.Fatalf("sets = %+v", sets)
		}
		for _, pair := range []datasource.TraceAttribute{{Key: "resource.service.name", Value: "cart"}, {Key: "span.http.route", Value: "/api/cart"}, {Key: "status", Value: "Error"}, {Key: "kind", Value: "Server"}} {
			if !slices.Contains(sets[0].Attributes, pair) {
				t.Errorf("captured trace lacks %+v: %+v", pair, sets[0].Attributes)
			}
		}
	})
}
