package clickhouse

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

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
	for _, bad := range []string{"", "span.a'b", "Duration); DROP", "span.", "event.name", "SpanAttributes['x']"} {
		if got, ok := GroupColumn(bad); ok {
			t.Errorf("GroupColumn(%q) = %q, want a refusal", bad, got)
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
