package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	signalLogs   = "logs"
	signalTraces = "traces"

	languageLogQL         = "logql"
	languageClickHouseSQL = "clickhouse_sql"
	languageTraceQL       = "traceql"
	languageFilters       = "filters"

	maxExplanationRunes = 300
	maxFilterRunes      = 200
	sampleWindow        = 15 * time.Minute

	warnNoSampleMatches = "No matches in the last 15 minutes."
)

var durationPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$`)

var nanosecondsPer = map[string]float64{"ns": 1, "us": 1e3, "µs": 1e3, "ms": 1e6, "s": 1e9, "m": 60e9, "h": 3600e9}

type draftRun struct {
	inst   datasource.Instance
	dsType spec.DatasourceType
	signal string
	rng    datasource.TimeRange
	facts  draftFacts
}

type checkFailure struct{ text string }

func (f *checkFailure) Error() string { return f.text }

func brokeRule(rule string) error {
	return &CannotTranslate{Reason: "The model's answer broke a rule: " + rule + "."}
}

func languageFor(dsType spec.DatasourceType, signal string, hasQuery bool) (string, bool) {
	switch {
	case dsType == spec.Loki && signal == signalLogs && hasQuery:
		return languageLogQL, true
	case dsType == spec.ClickHouse && signal == signalLogs && hasQuery:
		return languageClickHouseSQL, true
	case dsType == spec.ClickHouse && signal == signalTraces && !hasQuery:
		return languageFilters, true
	case dsType == spec.Tempo && signal == signalTraces && hasQuery:
		return languageTraceQL, true
	case dsType == spec.Tempo && signal == signalTraces:
		return languageFilters, true
	}
	return "", false
}

func languageRule(dsType spec.DatasourceType, signal string) string {
	switch {
	case dsType == spec.Loki:
		return "answer: a LogQL log query in submit_query's query."
	case dsType == spec.ClickHouse && signal == signalLogs:
		return "answer: a ClickHouse SQL query in submit_query's query."
	case dsType == spec.ClickHouse:
		return "answer: trace_filters in submit_query; this datasource takes no query."
	}
	return "answer: trace_filters in submit_query, or a TraceQL query in query only when the filters cannot express the question."
}

func submitSchema(dsType spec.DatasourceType, signal string) json.RawMessage {
	var props, required string
	switch {
	case dsType == spec.ClickHouse && signal == signalTraces:
		props, required = propTraceFilters, `"trace_filters","explanation"`
	case dsType == spec.Tempo:
		props, required = propDraftedQuery+","+propTraceFilters, `"explanation"`
	default:
		props, required = propDraftedQuery, `"query","explanation"`
	}
	return json.RawMessage(`{"type":"object","properties":{` + props + `,` + propExplanation + `},"required":[` + required + `],"additionalProperties":false}`)
}

const (
	propDraftedQuery = `"query":{"type":"string","minLength":1,"description":"The query, using only names from the context."}`
	propExplanation  = `"explanation":{"type":"string","minLength":1,"maxLength":300,"description":"One sentence saying what the answer matches."}`
	propTraceFilters = `"trace_filters":{"type":"object","additionalProperties":false,"properties":{` +
		`"service":{"type":"string","maxLength":200,"description":"Exact service name from the context."},` +
		`"span":{"type":"string","maxLength":200,"description":"Exact span name."},` +
		`"min":{"type":"string","pattern":"^(\\d+(?:\\.\\d+)?)(ns|us|µs|ms|s|m|h)$","description":"Minimum duration, such as 250ms."},` +
		`"max":{"type":"string","pattern":"^(\\d+(?:\\.\\d+)?)(ns|us|µs|ms|s|m|h)$","description":"Maximum duration, such as 2s."},` +
		`"errors":{"type":"boolean","description":"Only traces with an error span."}},"description":"The trace search form's fields; set only the ones the question needs."}`

	schemaCannotTranslate = `{"type":"object","properties":{"reason":{"type":"string","minLength":1,"maxLength":300,"description":"One sentence saying why the data cannot answer the question."}},"required":["reason"],"additionalProperties":false}`
)

func checkFilters(f TraceFilters) error {
	for field, value := range map[string]string{"service": f.Service, "span": f.Span} {
		switch {
		case utf8.RuneCountInString(value) > maxFilterRunes:
			return fmt.Errorf("trace_filters.%s must be at most %d characters", field, maxFilterRunes)
		case strings.ContainsFunc(value, unicode.IsControl):
			return fmt.Errorf("trace_filters.%s must not contain control characters", field)
		}
	}
	lower, okMin := durationNanos(f.Min)
	upper, okMax := durationNanos(f.Max)
	switch {
	case !okMin:
		return errors.New("trace_filters.min must be a duration such as 250ms")
	case !okMax:
		return errors.New("trace_filters.max must be a duration such as 2s")
	case f.Min != "" && f.Max != "" && lower > upper:
		return errors.New("trace_filters.min must not be greater than max")
	}
	return nil
}

func durationNanos(s string) (float64, bool) {
	if s == "" {
		return 0, true
	}
	m := durationPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	return n * nanosecondsPer[m[2]], err == nil
}

func sampleRange(rng datasource.TimeRange) datasource.TimeRange {
	if from := rng.To.Add(-sampleWindow); from.After(rng.From) {
		return datasource.TimeRange{From: from, To: rng.To}
	}
	return rng
}

func (r *Registry) sample(ctx context.Context, inst datasource.Instance, query string, rng datasource.TimeRange) (empty bool, failure string, err error) {
	dash, selections, _, _ := adhocDashboard(inst, spec.Table, instantInput{Datasource: inst.UID, Query: query}, "")
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	out, err := r.deps.Runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:  dash,
		PanelID:    adhocPanelID,
		Range:      sampleRange(rng),
		Selections: selections,
		Limits:     queryproxy.DraftLimits,
		User:       queryproxy.UserFor(principal),
	})
	if err != nil {
		_, code, message := queryproxy.Classify(err)
		switch code {
		case string(queryproxy.CodeLimitExceeded):
			return false, "", nil
		case string(queryproxy.CodeCanceled), string(queryproxy.CodeInternal):
			return false, "", err
		}
		return false, "the query failed on a sample of the last 15 minutes: " + code + ": " + message, nil
	}
	return summarize(out).Empty, "", nil
}

type submitInput struct {
	Query        string        `json:"query"`
	TraceFilters *TraceFilters `json:"trace_filters"`
	Explanation  string        `json:"explanation"`
}

func (r *Registry) checkSubmit(ctx context.Context, run draftRun, input json.RawMessage) (Draft, error) {
	var in submitInput
	if res, ok := decodeInput(input, &in); !ok {
		return Draft{}, brokeRule(resultMessage(res))
	}
	draft, err := run.shape(in)
	if err != nil {
		return Draft{}, err
	}
	if draft.TraceFilters != nil {
		draft.Warnings = run.serviceWarnings(draft.TraceFilters.Service)
		return draft, nil
	}
	if draft.Warnings, err = r.checkQuery(ctx, run, draft.Query); err != nil {
		return Draft{}, err
	}
	return draft, nil
}

func (run draftRun) shape(in submitInput) (Draft, error) {
	explanation := strings.TrimSpace(in.Explanation)
	query := strings.TrimSpace(in.Query)
	switch n := utf8.RuneCountInString(explanation); {
	case n < 1 || n > maxExplanationRunes:
		return Draft{}, brokeRule(fmt.Sprintf("explanation must be 1 to %d characters", maxExplanationRunes))
	case strings.ContainsFunc(explanation, unicode.IsControl):
		return Draft{}, brokeRule("explanation must be one line of plain text")
	}
	language, ok := languageFor(run.dsType, run.signal, query != "")
	switch {
	case !ok && query != "":
		return Draft{}, brokeRule("this datasource takes trace_filters, not a query")
	case !ok:
		return Draft{}, brokeRule("query is required")
	case language == languageFilters && in.TraceFilters == nil:
		return Draft{}, brokeRule("trace_filters is required")
	case language != languageFilters && in.TraceFilters != nil:
		return Draft{}, brokeRule("send either query or trace_filters, not both")
	}
	draft := Draft{Language: language, Explanation: explanation, Warnings: []string{}}
	if language == languageFilters {
		filters := trimmedFilters(*in.TraceFilters)
		if err := checkFilters(filters); err != nil {
			return Draft{}, brokeRule(err.Error())
		}
		draft.TraceFilters = &filters
		return draft, nil
	}
	if res, ok := checkQueryText(query); !ok {
		return Draft{}, brokeRule(resultMessage(res))
	}
	draft.Query = query
	return draft, nil
}

func trimmedFilters(f TraceFilters) TraceFilters {
	return TraceFilters{Service: strings.TrimSpace(f.Service), Span: strings.TrimSpace(f.Span), Min: strings.TrimSpace(f.Min), Max: strings.TrimSpace(f.Max), Errors: f.Errors}
}

func (run draftRun) serviceWarnings(service string) []string {
	if service == "" || !run.facts.ServicesComplete || slices.Contains(run.facts.Services, service) {
		return []string{}
	}
	return []string{"No service named " + service + " was seen in this datasource; the search may be empty."}
}

func (r *Registry) checkQuery(ctx context.Context, run draftRun, query string) ([]string, error) {
	diagnostics, res, ok, err := r.lint(ctx, run.inst, query)
	if err != nil {
		return nil, err
	}
	if !ok {
		body, _ := res.Content.(errorBody)
		return nil, &DatasourceError{Code: body.Code, Message: body.Message}
	}
	var problems []string
	warnings := []string{}
	for _, d := range diagnostics {
		if d.Severity == datasource.SeverityError {
			problems = append(problems, d.Message)
		} else {
			warnings = append(warnings, d.Message)
		}
	}
	if len(problems) > 0 {
		return nil, &checkFailure{text: "the query fails the lint check: " + strings.Join(problems, "; ")}
	}
	empty, failure, err := r.sample(ctx, run.inst, query, run.rng)
	switch {
	case err != nil:
		return nil, err
	case failure != "":
		return nil, &checkFailure{text: failure}
	case empty:
		warnings = append(warnings, warnNoSampleMatches)
	}
	return append(warnings, draftWarnings(run.dsType, query)...), nil
}

func resultMessage(res Result) string {
	body, _ := res.Content.(errorBody)
	return body.Message
}
