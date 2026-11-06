package cli

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const traceSearchLimit = 100

var durationPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$`)

var nanosecondsPer = map[string]float64{
	"ns": 1,
	"us": 1e3,
	"µs": 1e3,
	"ms": 1e6,
	"s":  1e9,
	"m":  60e9,
	"h":  3600e9,
}

var traceSearchHeaders = []string{"STARTED", "SERVICE", "SPAN", "DURATION", "SPANS", "ERRORS"}

type traceFilters struct {
	Service string `json:"service"`
	Span    string `json:"span"`
	Min     string `json:"min"`
	Max     string `json:"max"`
	Errors  bool   `json:"errors"`
}

func parseDuration(text string) (int64, bool) {
	match := durationPattern.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return int64(n*nanosecondsPer[match[2]] + 0.5), true
}

func traceQLQuoted(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func sqlQuoted(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

func tempoTraceQL(f traceFilters) string {
	var conditions []string
	if f.Service != "" {
		conditions = append(conditions, "resource.service.name = "+traceQLQuoted(f.Service))
	}
	if f.Span != "" {
		conditions = append(conditions, "name = "+traceQLQuoted(f.Span))
	}
	if f.Min != "" {
		conditions = append(conditions, "duration >= "+strings.Replace(f.Min, "µs", "us", 1))
	}
	if f.Max != "" {
		conditions = append(conditions, "duration <= "+strings.Replace(f.Max, "µs", "us", 1))
	}
	if f.Errors {
		conditions = append(conditions, "status = error")
	}
	return "{" + strings.Join(conditions, " && ") + "}"
}

func clickhouseTraceSearch(f traceFilters) string {
	var conditions strings.Builder
	if f.Service != "" {
		conditions.WriteString(" AND otel_traces.ServiceName = " + sqlQuoted(f.Service))
	}
	if f.Span != "" {
		conditions.WriteString(" AND otel_traces.SpanName = " + sqlQuoted(f.Span))
	}
	if ns, ok := parseDuration(f.Min); ok {
		conditions.WriteString(" AND otel_traces.Duration >= " + strconv.FormatInt(ns, 10))
	}
	if ns, ok := parseDuration(f.Max); ok {
		conditions.WriteString(" AND otel_traces.Duration <= " + strconv.FormatInt(ns, 10))
	}
	if f.Errors {
		conditions.WriteString(" AND lower(StatusCode) IN ('error', 'status_code_error')")
	}
	return "SELECT TraceId, min(Timestamp) AS StartTime, argMax(otel_traces.ServiceName, otel_traces.Duration) AS ServiceName, argMax(otel_traces.SpanName, otel_traces.Duration) AS SpanName, max(otel_traces.Duration) AS Duration, count() AS Spans, countIf(lower(StatusCode) IN ('error', 'status_code_error')) AS Errors\n" +
		"FROM otel_traces\n" +
		"WHERE $__timeFilter(Timestamp)" + conditions.String() + "\n" +
		"GROUP BY TraceId\n" +
		"ORDER BY StartTime DESC\n" +
		"LIMIT " + strconv.Itoa(traceSearchLimit)
}

func traceSearchRows(frames []frame) [][]string {
	var rows [][]string
	for _, f := range frames {
		started, service, span := fieldIndex(f, "StartTime"), fieldIndex(f, "ServiceName"), fieldIndex(f, "SpanName")
		duration, spans, errs := fieldIndex(f, "Duration"), fieldIndex(f, "Spans"), fieldIndex(f, "Errors")
		for row := range f.rows() {
			rows = append(rows, []string{
				startedText(f, started, row),
				text(f.stringAt(service, row)),
				text(f.stringAt(span, row)),
				durationText(f, duration, row),
				text(f.stringAt(spans, row)),
				text(f.stringAt(errs, row)),
			})
		}
	}
	return rows
}

func startedText(f frame, field, row int) string {
	if ms, ok := f.numberAt(field, row); ok {
		return time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339)
	}
	return text(f.stringAt(field, row))
}

func durationText(f frame, field, row int) string {
	if len(f.raw(field, row)) == 0 {
		return ""
	}
	return formatSpanDuration(time.Duration(nanosAt(f, field, row)))
}
