package savedsearches

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/usertext"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxTitleRunes  = 120
	maxQueryRunes  = 8000
	maxColumns     = 20
	maxColumnRunes = 256
	maxFilterRunes = 200
	maxTimeRunes   = 64
	defaultFrom    = "now-1h"
	defaultTo      = "now"
)

var (
	datasourceUID   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)
	durationPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$`)

	logsDatasources   = []spec.DatasourceType{spec.Loki, spec.ClickHouse, spec.Elasticsearch, spec.OpenSearch}
	tracesDatasources = []spec.DatasourceType{spec.Tempo, spec.Jaeger, spec.ClickHouse}

	metricsDatasources = []spec.DatasourceType{spec.Prometheus}
)

func invalid(format string, args ...any) *InvalidInputError {
	return &InvalidInputError{Code: CodeInvalid, Message: fmt.Sprintf(format, args...)}
}

func cleanTitle(raw string) (string, error) {
	title, err := usertext.Clean("title", raw, usertext.Line(maxTitleRunes))
	if err != nil {
		return "", &InvalidInputError{Code: CodeInvalid, Message: err.Error(), Cause: err}
	}
	return title, nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

type validated struct {
	CreateInput
	filters Filters
}

func RangeOrDefault(from, to string) (string, string) {
	return cmp.Or(from, defaultFrom), cmp.Or(to, defaultTo)
}

func validate(in CreateInput, now time.Time) (validated, error) {
	out := validated{CreateInput: in}
	if !in.Kind.Valid() {
		return out, invalid("kind must be logs, traces or metrics")
	}
	title, err := cleanTitle(in.Title)
	if err != nil {
		return out, err
	}
	out.Title = title
	if !datasourceUID.MatchString(in.DatasourceUID) {
		return out, invalid("datasource_uid is not a datasource uid")
	}
	if err := checkQuery(in.Kind, in.Query); err != nil {
		return out, err
	}
	if out.filters, err = parseFilters(in.Kind, in.Filters); err != nil {
		return out, err
	}
	out.From, out.To = RangeOrDefault(out.From, out.To)
	for _, t := range []struct {
		name, expr string
		roundUp    bool
	}{{"from", out.From, false}, {"to", out.To, true}} {
		if utf8.RuneCountInString(t.expr) > maxTimeRunes {
			return out, invalid("%s must be at most %d characters", t.name, maxTimeRunes)
		}
		if _, err := spec.ResolveTime(t.expr, now, t.roundUp, spec.TimeOptions{}); err != nil {
			return out, invalid("%s: %v", t.name, err)
		}
	}
	return out, nil
}

func checkQuery(kind Kind, query *string) error {
	if query == nil {
		if kind != KindTraces {
			return invalid("query is required for a %s search", kind)
		}
		return nil
	}
	switch n := utf8.RuneCountInString(*query); {
	case strings.TrimSpace(*query) == "":
		return invalid("query must not be blank")
	case n > maxQueryRunes:
		return invalid("query must be at most %d characters", maxQueryRunes)
	case strings.ContainsRune(*query, 0) || !utf8.ValidString(*query):
		return invalid("query must be valid text without the NUL character")
	}
	return nil
}

func parseFilters(kind Kind, raw []byte) (Filters, error) {
	var filters Filters
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return filters, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&filters); err != nil {
		return Filters{}, invalid("filters must be an object with only the keys of a %s search: %v", kind, err)
	}
	switch kind {
	case KindLogs:
		return filters, checkLogsFilters(filters)
	case KindMetrics:
		return filters, checkMetricsFilters(filters)
	}
	return filters, checkTraceFilters(filters)
}

func checkMetricsFilters(f Filters) error {
	if len(f.Columns) > 0 || f.Service != "" || f.Span != "" || f.Min != "" || f.Max != "" || f.Errors {
		return invalid("a metrics search takes only the filter key mode")
	}
	if f.Mode != "" && f.Mode != "graph" && f.Mode != ModeTable {
		return invalid("mode must be graph or table")
	}
	return nil
}

func checkLogsFilters(f Filters) error {
	if f.Service != "" || f.Span != "" || f.Min != "" || f.Max != "" || f.Errors || f.Mode != "" {
		return invalid("a logs search takes only the filter key columns")
	}
	if len(f.Columns) > maxColumns {
		return invalid("columns takes at most %d names", maxColumns)
	}
	for _, name := range f.Columns {
		if n := utf8.RuneCountInString(name); n < 1 || n > maxColumnRunes || strings.Contains(name, ",") || hasControl(name) || !utf8.ValidString(name) {
			return invalid("a column name is 1 to %d characters without a comma or control characters", maxColumnRunes)
		}
	}
	return nil
}

func checkTraceFilters(f Filters) error {
	if len(f.Columns) > 0 || f.Mode != "" {
		return invalid("a traces search takes only the filter keys service, span, min, max and errors")
	}
	for _, v := range []struct{ name, value string }{{"service", f.Service}, {"span", f.Span}} {
		if utf8.RuneCountInString(v.value) > maxFilterRunes || hasControl(v.value) {
			return invalid("%s must be at most %d characters without control characters", v.name, maxFilterRunes)
		}
	}
	for _, v := range []struct{ name, value string }{{"min", f.Min}, {"max", f.Max}} {
		if v.value != "" && !durationPattern.MatchString(v.value) {
			return invalid("%s must be a duration such as 250ms or 2s", v.name)
		}
	}
	if f.Min != "" && f.Max != "" && durationNanos(f.Min) > durationNanos(f.Max) {
		return invalid("min must not be greater than max")
	}
	return nil
}

var nanosecondsPer = map[string]float64{"ns": 1, "us": 1e3, "µs": 1e3, "ms": 1e6, "s": 1e9, "m": 60e9, "h": 3600e9}

func durationNanos(text string) float64 {
	match := durationPattern.FindStringSubmatch(text)
	n, _ := strconv.ParseFloat(match[1], 64)
	return n * nanosecondsPer[match[2]]
}

func fitDatasource(kind Kind, dsType spec.DatasourceType) error {
	fits := tracesDatasources
	switch kind {
	case KindLogs:
		fits = logsDatasources
	case KindMetrics:
		fits = metricsDatasources
	}
	supported := false
	for _, t := range fits {
		supported = supported || t == dsType
	}
	if !supported {
		return &InvalidInputError{Code: CodeUnsupportedDatasource, Message: fmt.Sprintf("a %s datasource cannot hold a %s search", dsType, kind)}
	}
	return nil
}
