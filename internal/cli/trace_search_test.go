package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

const traceSearchSelect = "SELECT TraceId, min(Timestamp) AS StartTime, argMax(otel_traces.ServiceName, otel_traces.Duration) AS ServiceName, argMax(otel_traces.SpanName, otel_traces.Duration) AS SpanName, max(otel_traces.Duration) AS Duration, count() AS Spans, countIf(lower(StatusCode) IN ('error', 'status_code_error')) AS Errors\nFROM otel_traces\nWHERE $__timeFilter(Timestamp)"

const traceSearchTail = "\nGROUP BY TraceId\nORDER BY StartTime DESC\nLIMIT 100"

func TestTempoTraceQL(t *testing.T) {
	cases := []struct {
		filters traceFilters
		want    string
	}{
		{traceFilters{}, "{}"},
		{traceFilters{Service: "checkout"}, `{resource.service.name = "checkout"}`},
		{traceFilters{Span: "GET /cart"}, `{name = "GET /cart"}`},
		{traceFilters{Min: "250µs"}, "{duration >= 250us}"},
		{traceFilters{Max: "2s"}, "{duration <= 2s}"},
		{traceFilters{Errors: true}, "{status = error}"},
		{traceFilters{Service: `a"b\c`}, `{resource.service.name = "a\"b\\c"}`},
		{traceFilters{Service: "checkout", Span: "pay", Min: "1.5ms", Max: "2s", Errors: true}, `{resource.service.name = "checkout" && name = "pay" && duration >= 1.5ms && duration <= 2s && status = error}`},
	}
	for _, c := range cases {
		if got := tempoTraceQL(c.filters); got != c.want {
			t.Errorf("%+v: %s, want %s", c.filters, got, c.want)
		}
	}
}

func TestClickHouseTraceSearch(t *testing.T) {
	cases := []struct {
		filters traceFilters
		want    string
	}{
		{traceFilters{}, ""},
		{traceFilters{Service: "checkout"}, " AND otel_traces.ServiceName = 'checkout'"},
		{traceFilters{Span: "GET /cart"}, " AND otel_traces.SpanName = 'GET /cart'"},
		{traceFilters{Min: "250µs"}, " AND otel_traces.Duration >= 250000"},
		{traceFilters{Max: "1.5s"}, " AND otel_traces.Duration <= 1500000000"},
		{traceFilters{Min: "2m"}, " AND otel_traces.Duration >= 120000000000"},
		{traceFilters{Errors: true}, " AND lower(StatusCode) IN ('error', 'status_code_error')"},
		{traceFilters{Service: `o'brien\x`}, ` AND otel_traces.ServiceName = 'o\'brien\\x'`},
		{traceFilters{Min: "fast"}, ""},
		{traceFilters{Service: "checkout", Span: "pay", Min: "500ms", Max: "2s", Errors: true}, " AND otel_traces.ServiceName = 'checkout' AND otel_traces.SpanName = 'pay' AND otel_traces.Duration >= 500000000 AND otel_traces.Duration <= 2000000000 AND lower(StatusCode) IN ('error', 'status_code_error')"},
	}
	for _, c := range cases {
		if got := clickhouseTraceSearch(c.filters); got != traceSearchSelect+c.want+traceSearchTail {
			t.Errorf("%+v: %q", c.filters, strings.TrimSuffix(strings.TrimPrefix(got, traceSearchSelect), traceSearchTail))
		}
	}
}

func TestTraceSearchRowsLeaveMissingFieldsBlank(t *testing.T) {
	frames := []frame{{Fields: []frameField{
		{Name: "starttime", Type: "time", Values: raws(`1762000000000`)},
		{Name: "ServiceName", Type: "string", Values: raws(`"checkout"`)},
		{Name: "SpanName", Type: "string", Values: raws(`"POST /checkout"`)},
		{Name: "Duration", Type: "number", Values: raws(`1230000000`)},
	}}}
	rows := traceSearchRows(frames)
	want := []string{"2025-11-01T12:26:40Z", "checkout", "POST /checkout", "1.23s", "", ""}
	if len(rows) != 1 || strings.Join(rows[0], "|") != strings.Join(want, "|") {
		t.Fatalf("rows %q", rows)
	}
}

func raws(values ...string) []json.RawMessage {
	out := make([]json.RawMessage, len(values))
	for i, v := range values {
		out[i] = json.RawMessage(v)
	}
	return out
}
