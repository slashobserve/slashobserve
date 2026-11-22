package cli

import (
	"cmp"
	"regexp"
	"slices"
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

func logfmtValue(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\r\"=\\") {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	return s
}

func jaegerSearch(f traceFilters) string {
	var pairs []string
	add := func(key, value string) {
		if value != "" {
			pairs = append(pairs, key+"="+logfmtValue(value))
		}
	}
	add("service", f.Service)
	add("operation", f.Span)
	add("minDuration", strings.Replace(f.Min, "µs", "us", 1))
	add("maxDuration", strings.Replace(f.Max, "µs", "us", 1))
	if f.Errors {
		add("error", "true")
	}
	return strings.Join(pairs, " ")
}

const clickhouseErrorStatus = "lower(StatusCode) IN ('error', 'status_code_error')"

const clickhouseTraceTime = "otel_traces.Timestamp"

func clickhouseTraceRoot(column string) string {
	root := "otel_traces.ParentSpanId = ''"
	return "if(countIf(" + root + ") > 0, argMinIf(otel_traces." + column + ", " + clickhouseTraceTime + ", " + root + "), argMin(otel_traces." + column + ", " + clickhouseTraceTime + "))"
}

func clickhouseTraceConditions(f traceFilters) []string {
	var conditions []string
	if f.Service != "" {
		conditions = append(conditions, "otel_traces.ServiceName = "+sqlQuoted(f.Service))
	}
	if f.Span != "" {
		conditions = append(conditions, "otel_traces.SpanName = "+sqlQuoted(f.Span))
	}
	if ns, ok := parseDuration(f.Min); ok {
		conditions = append(conditions, "otel_traces.Duration >= "+strconv.FormatInt(ns, 10))
	}
	if ns, ok := parseDuration(f.Max); ok {
		conditions = append(conditions, "otel_traces.Duration <= "+strconv.FormatInt(ns, 10))
	}
	if f.Errors {
		conditions = append(conditions, clickhouseErrorStatus)
	}
	return conditions
}

func clickhouseTraceSearch(f traceFilters) string {
	joined := strings.Join(clickhouseTraceConditions(f), " AND ")
	matching, scope := "", ""
	if joined != "" {
		matching = ", countIf(" + joined + ") AS Matching"
		scope = " AND TraceId IN (SELECT TraceId FROM otel_traces WHERE $__timeFilter(otel_traces.Timestamp) AND " + joined + ")"
	}
	return "SELECT TraceId, min(" + clickhouseTraceTime + ") AS StartTime, min(" + clickhouseTraceTime + ") AS Timestamp, " + clickhouseTraceRoot("ServiceName") + " AS ServiceName, " + clickhouseTraceRoot("SpanName") + " AS SpanName, toUInt64(max(toUnixTimestamp64Nano(" + clickhouseTraceTime + ") + toInt64(otel_traces.Duration)) - min(toUnixTimestamp64Nano(" + clickhouseTraceTime + "))) AS Duration, count() AS Spans, countIf(" + clickhouseErrorStatus + ") AS Errors" + matching + "\n" +
		"FROM otel_traces\n" +
		"WHERE $__timeFilter(otel_traces.Timestamp)" + scope + "\n" +
		"GROUP BY TraceId"
}

type traceSort struct {
	column     string
	descending bool
}

var traceSortColumns = map[string]string{
	"start":    "StartTime",
	"service":  "ServiceName",
	"span":     "SpanName",
	"duration": "Duration",
	"spans":    "Spans",
	"errors":   "Errors",
}

func parseTraceSort(text string) (*traceSort, error) {
	if text == "" {
		return nil, nil
	}
	key, descending := strings.CutPrefix(text, "-")
	column, ok := traceSortColumns[key]
	if !ok {
		return nil, usagef("--sort takes start, service, span, duration, spans or errors, with a minus in front for descending, such as -duration; got %q", text)
	}
	return &traceSort{column: column, descending: descending}, nil
}

func (s traceSort) param() string {
	if s.descending {
		return "-" + s.column
	}
	return s.column
}

type traceRecord struct {
	cells   []string
	numbers map[string]float64
	words   map[string]string
}

func (r traceRecord) compare(other traceRecord, column string) int {
	if column == "ServiceName" || column == "SpanName" {
		return strings.Compare(r.words[column], other.words[column])
	}
	return cmp.Compare(r.numbers[column], other.numbers[column])
}

func traceSearchRecords(frames []frame) []traceRecord {
	var records []traceRecord
	for _, f := range frames {
		started, service, span := fieldIndex(f, "StartTime"), fieldIndex(f, "ServiceName"), fieldIndex(f, "SpanName")
		duration, spans, errs := fieldIndex(f, "Duration"), fieldIndex(f, "Spans"), fieldIndex(f, "Errors")
		for row := range f.rows() {
			startMs, _ := f.numberAt(started, row)
			spanCount, _ := f.numberAt(spans, row)
			errorCount, _ := f.numberAt(errs, row)
			records = append(records, traceRecord{
				cells: []string{
					startedText(f, started, row),
					text(f.stringAt(service, row)),
					text(f.stringAt(span, row)),
					durationText(f, duration, row),
					text(f.stringAt(spans, row)),
					text(f.stringAt(errs, row)),
				},
				numbers: map[string]float64{"StartTime": startMs, "Duration": nanosAt(f, duration, row), "Spans": spanCount, "Errors": errorCount},
				words:   map[string]string{"ServiceName": f.stringAt(service, row), "SpanName": f.stringAt(span, row)},
			})
		}
	}
	return records
}

func traceSearchRows(frames []frame, clientSort *traceSort) [][]string {
	records := traceSearchRecords(frames)
	if clientSort != nil {
		slices.SortStableFunc(records, func(a, b traceRecord) int {
			if clientSort.descending {
				return b.compare(a, clientSort.column)
			}
			return a.compare(b, clientSort.column)
		})
	}
	rows := make([][]string, len(records))
	for i, record := range records {
		rows[i] = record.cells
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
