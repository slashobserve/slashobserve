package clickhouse

import (
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

func TestListQueryBindsEveryFilterValue(t *testing.T) {
	text, params := ListQuery(datasource.TraceListRequest{
		Filter: datasource.TraceFilter{Service: "pay'ment", Route: "GET /checkout", Operation: "Charge"},
		Errors: true, MinDuration: 2 * time.Millisecond, Limit: 7,
		Range: datasource.TimeRange{From: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 6, 11, 0, 0, 0, time.UTC)},
	})
	for _, want := range []string{
		"HAVING countIf(lower(StatusCode) IN ('error','status_code_error')) > 0 AND countIf(ServiceName = {service:String}) > 0 AND countIf(SpanName = {operation:String}) > 0 AND Route = {route:String} AND TraceDuration >= {min:UInt64}",
		"ORDER BY Start DESC, TraceId LIMIT {n:UInt64}",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("query lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "pay'ment") || params["service"] != "pay'ment" || params["min"] != "2000000" || params["n"] != "7" || params["from"] != "2026-11-06 10:00:00.000000000" {
		t.Errorf("params = %v", params)
	}
	if text, _ := ListQuery(datasource.TraceListRequest{}); strings.Contains(text, "HAVING") {
		t.Errorf("an empty filter has a HAVING clause:\n%s", text)
	}
}

func TestGroupColumnMapsAttributesToColumns(t *testing.T) {
	for attribute, want := range map[string]string{
		"resource.service.name":  "ServiceName",
		"ServiceName":            "ServiceName",
		"name":                   "SpanName",
		"status":                 "StatusCode",
		"kind":                   "SpanKind",
		"statusMessage":          "StatusMessage",
		"span.http.route":        "SpanAttributes['http.route']",
		".http.route":            "SpanAttributes['http.route']",
		"resource.k8s.pod.name":  "ResourceAttributes['k8s.pod.name']",
		"span.app.product_id-v2": "SpanAttributes['app.product_id-v2']",
	} {
		if got, ok := GroupColumn(attribute); !ok || got != want {
			t.Errorf("GroupColumn(%q) = %q, %v; want %q", attribute, got, ok, want)
		}
	}
	for _, bad := range []string{"", "span.a'b", `span.a\'b`, `resource.a\b`, "Duration); DROP", "span.", "event.name", "SpanAttributes['x']"} {
		if got, ok := GroupColumn(bad); ok {
			t.Errorf("GroupColumn(%q) = %q, want a refusal", bad, got)
		}
	}
}

func TestGroupColumnEscapesTheKeyItQuotes(t *testing.T) {
	for key, want := range map[string]string{
		"http.route": `SpanAttributes['http.route']`,
		`a'b\c`:      `SpanAttributes['a\'b\\c']`,
	} {
		if got := attributeColumn("span", key); got != want {
			t.Errorf("attributeColumn(%q) = %s, want %s", key, got, want)
		}
	}
}

func TestDurationQuantilesQueryText(t *testing.T) {
	if got, want := DurationQuantilesQuery(checkoutFilter, ""), "SELECT count() AS Spans, countIf(lower(StatusCode) IN ('error','status_code_error')) AS Errors, quantilesExact(0.5,0.95,0.99)(Duration) AS Q\n"+
		"FROM otel_traces WHERE $__timeFilter(Timestamp) AND ("+checkoutFilter+")"; got != want {
		t.Errorf("window =\n%s\nwant\n%s", got, want)
	}
	if got, want := DurationQuantilesQuery("", "ServiceName"), "SELECT ServiceName AS Value, count() AS Spans, countIf(lower(StatusCode) IN ('error','status_code_error')) AS Errors, quantilesExact(0.5,0.95,0.99)(Duration) AS Q\n"+
		"FROM otel_traces WHERE $__timeFilter(Timestamp)\nGROUP BY Value HAVING Value != '' ORDER BY Spans DESC, Value LIMIT 21"; got != want {
		t.Errorf("grouped =\n%s\nwant\n%s", got, want)
	}
	for _, q := range []string{DurationQuantilesQuery(checkoutFilter, ""), DurationQuantilesQuery("", "SpanAttributes['http.route']")} {
		exp, err := macros.ExpandForLint(macros.ClickHouse, q)
		if err != nil {
			t.Fatalf("expand %q: %v", q, err)
		}
		if _, err := guardWith(exp.Text, loadedTableFunctions()); err != nil {
			t.Errorf("guard refused %q: %v", exp.Text, err)
		}
	}
}
