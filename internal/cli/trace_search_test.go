package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func readTraceListSQL(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "datasource", "clickhouse", "testdata", name)) //nolint:gosec // a checked-in fixture
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(data), "\n")
}

func TestClickHouseTraceSearchIsTheSQLTheDatasourceTestsRun(t *testing.T) {
	if got, want := clickhouseTraceSearch(traceFilters{}), readTraceListSQL(t, "trace_list_unfiltered.sql"); got != want {
		t.Errorf("unfiltered:\n%s\nwant\n%s", got, want)
	}
	if got, want := clickhouseTraceSearch(traceFilters{Service: "tl-front", Min: "50ms"}), readTraceListSQL(t, "trace_list_filtered.sql"); got != want {
		t.Errorf("filtered:\n%s\nwant\n%s", got, want)
	}
}

func TestClickHouseTraceSearchConditions(t *testing.T) {
	cases := []struct {
		filters traceFilters
		want    string
	}{
		{traceFilters{Service: "checkout"}, "otel_traces.ServiceName = 'checkout'"},
		{traceFilters{Span: "GET /cart"}, "otel_traces.SpanName = 'GET /cart'"},
		{traceFilters{Min: "250µs"}, "otel_traces.Duration >= 250000"},
		{traceFilters{Max: "1.5s"}, "otel_traces.Duration <= 1500000000"},
		{traceFilters{Min: "2m"}, "otel_traces.Duration >= 120000000000"},
		{traceFilters{Errors: true}, "lower(StatusCode) IN ('error', 'status_code_error')"},
		{traceFilters{Service: `o'brien\x`}, `otel_traces.ServiceName = 'o\'brien\\x'`},
		{traceFilters{Service: "checkout", Span: "pay", Min: "500ms", Max: "2s", Errors: true}, "otel_traces.ServiceName = 'checkout' AND otel_traces.SpanName = 'pay' AND otel_traces.Duration >= 500000000 AND otel_traces.Duration <= 2000000000 AND lower(StatusCode) IN ('error', 'status_code_error')"},
	}
	for _, c := range cases {
		got := clickhouseTraceSearch(c.filters)
		if !strings.Contains(got, "AS Matching") || !strings.Contains(got, "countIf("+c.want+") AS Matching") || !strings.Contains(got, "AND "+c.want+")\nGROUP BY TraceId") {
			t.Errorf("%+v: %q", c.filters, got)
		}
	}
	for _, unusable := range []traceFilters{{}, {Min: "fast"}} {
		if got := clickhouseTraceSearch(unusable); strings.Contains(got, "Matching") || strings.Contains(got, "IN (SELECT") {
			t.Errorf("%+v: %q", unusable, got)
		}
	}
}

func TestParseTraceSort(t *testing.T) {
	cases := map[string]string{"start": "StartTime", "-duration": "-Duration", "spans": "Spans", "-errors": "-Errors", "service": "ServiceName", "-span": "-SpanName"}
	for in, want := range cases {
		sort, err := parseTraceSort(in)
		if err != nil || sort.param() != want {
			t.Errorf("%s: %+v, %v, want %s", in, sort, err, want)
		}
	}
	if sort, err := parseTraceSort(""); sort != nil || err != nil {
		t.Errorf("empty: %+v, %v", sort, err)
	}
	if _, err := parseTraceSort("Duration"); err == nil || !strings.Contains(err.Error(), "--sort takes start") {
		t.Errorf("Duration: %v", err)
	}
}

func TestTraceSearchRowsSortOnTheClient(t *testing.T) {
	frames := []frame{{Fields: []frameField{
		{Name: "StartTime", Type: "time", Values: raws(`1762000000000`, `1762000060000`, `1762000120000`)},
		{Name: "ServiceName", Type: "string", Values: raws(`"b"`, `"c"`, `"a"`)},
		{Name: "SpanName", Type: "string", Values: raws(`"x"`, `"y"`, `"z"`)},
		{Name: "Duration", Type: "number", Values: raws(`2000000`, `9000000`, `500000`)},
		{Name: "Spans", Type: "number", Values: raws(`3`, `10`, `2`)},
	}}}
	order := func(sort string) string {
		parsed, err := parseTraceSort(sort)
		if err != nil {
			t.Fatal(err)
		}
		var services []string
		for _, row := range traceSearchRows(frames, parsed) {
			services = append(services, row[1])
		}
		return strings.Join(services, "")
	}
	for sort, want := range map[string]string{"": "bca", "-duration": "cba", "duration": "abc", "-spans": "cba", "service": "abc", "-start": "acb"} {
		if got := order(sort); got != want {
			t.Errorf("--sort %q: %s, want %s", sort, got, want)
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
	rows := traceSearchRows(frames, nil)
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

func TestJaegerSearchWritesOnlyTheSetFilters(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   traceFilters
		want string
	}{
		{"none", traceFilters{}, ""},
		{"all", traceFilters{Service: "checkout", Span: "POST /pay", Min: "500ms", Max: "2s", Errors: true}, `service=checkout operation="POST /pay" minDuration=500ms maxDuration=2s error=true`},
		{"microseconds", traceFilters{Min: "250µs"}, "minDuration=250us"},
		{"quote", traceFilters{Span: `say "hi"`}, `operation="say \"hi\""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := jaegerSearch(tc.in); got != tc.want {
				t.Fatalf("jaegerSearch = %q, want %q", got, tc.want)
			}
		})
	}
}
