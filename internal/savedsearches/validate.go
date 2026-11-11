package savedsearches

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	tracesDatasources = []spec.DatasourceType{spec.Tempo, spec.ClickHouse}
)

func invalid(format string, args ...any) *InvalidInputError {
	return &InvalidInputError{Code: CodeInvalid, Message: fmt.Sprintf(format, args...)}
}

func cleanTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	switch {
	case title == "":
		return "", invalid("title is required")
	case utf8.RuneCountInString(title) > maxTitleRunes:
		return "", invalid("title must be at most %d characters", maxTitleRunes)
	case hasControl(title):
		return "", invalid("title must not contain control characters")
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

func validate(in CreateInput, now time.Time) (validated, error) {
	out := validated{CreateInput: in}
	if in.Kind != KindLogs && in.Kind != KindTraces {
		return out, invalid("kind must be logs or traces")
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
	if out.From == "" {
		out.From = defaultFrom
	}
	if out.To == "" {
		out.To = defaultTo
	}
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
		if kind == KindLogs {
			return invalid("query is required for a logs search")
		}
		return nil
	}
	switch n := utf8.RuneCountInString(*query); {
	case strings.TrimSpace(*query) == "":
		return invalid("query must not be blank")
	case n > maxQueryRunes:
		return invalid("query must be at most %d characters", maxQueryRunes)
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
	if kind == KindLogs {
		return filters, checkLogsFilters(filters)
	}
	return filters, checkTraceFilters(filters)
}

func checkLogsFilters(f Filters) error {
	if f.Service != "" || f.Span != "" || f.Min != "" || f.Max != "" || f.Errors {
		return invalid("a logs search takes only the filter key columns")
	}
	if len(f.Columns) > maxColumns {
		return invalid("columns takes at most %d names", maxColumns)
	}
	for _, name := range f.Columns {
		if n := utf8.RuneCountInString(name); n < 1 || n > maxColumnRunes || strings.Contains(name, ",") {
			return invalid("a column name is 1 to %d characters without a comma", maxColumnRunes)
		}
	}
	return nil
}

func checkTraceFilters(f Filters) error {
	if len(f.Columns) > 0 {
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
	return nil
}

func fitDatasource(kind Kind, dsType spec.DatasourceType, query *string) error {
	fits := tracesDatasources
	if kind == KindLogs {
		fits = logsDatasources
	}
	supported := false
	for _, t := range fits {
		supported = supported || t == dsType
	}
	if !supported {
		return &InvalidInputError{Code: CodeUnsupportedDatasource, Message: fmt.Sprintf("a %s datasource cannot hold a %s search", dsType, kind)}
	}
	if kind == KindTraces && dsType == spec.ClickHouse && query != nil {
		return invalid("a ClickHouse traces search takes no query")
	}
	return nil
}
