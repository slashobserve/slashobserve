package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"math"
	"regexp"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	lintTimeout        = 5 * time.Second
	queryGrace         = 5 * time.Second
	maxQueryBytes      = 64 << 10
	maxQueryDays       = 31
	maxVariables       = 20
	defaultFrom        = "now-1h"
	defaultTo          = "now"
	defaultDataPoints  = 200
	minDataPoints      = 10
	maxDataPoints      = 1000
	maxTimeExprBytes   = 64
	maxSeriesOut       = 20
	maxTableColumns    = 30
	maxTableRows       = 10
	maxSummaryCell     = 200
	seriesBudget       = 7 << 10
	tableColumnsBudget = 3 << 10
	tableRowsBudget    = 4 << 10
	adhocPanelID       = "adhoc"
	defaultDashTitle   = "Untitled dashboard"
	maxDashTitleChars  = 200
)

var (
	toolLimits = datasource.Limits{Timeout: 15 * time.Second, MaxSeries: 200, MaxPoints: 100_000, MaxRows: 1000}

	variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

func (r *Registry) queryEntries() []entry {
	return []entry{
		{Tool{Name: nameLintQuery, Description: descLintQuery, InputSchema: json.RawMessage(schemaLintQuery), ReadOnly: true}, r.lintQuery},
		{Tool{Name: nameQueryRange, Description: descQueryRange, InputSchema: json.RawMessage(schemaQueryRange), ReadOnly: true}, r.queryRange},
		{Tool{Name: nameQueryInstant, Description: descQueryInstant, InputSchema: json.RawMessage(schemaQueryInstant), ReadOnly: true}, r.queryInstant},
	}
}

type lintOut struct {
	OK          bool                    `json:"ok"`
	Diagnostics []datasource.Diagnostic `json:"diagnostics"`
	Truncated   bool                    `json:"truncated,omitempty"`
}

func (r *Registry) lintQuery(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource string `json:"datasource"`
		Query      string `json:"query"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return res, nil
	}
	lintCtx, cancel := context.WithTimeout(ctx, lintTimeout)
	defer cancel()
	diagnostics, err := inst.Datasource.Lint(lintCtx, in.Query)
	if err != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return datasourceFailure("Could not lint a query on "+inst.UID, err)
	}
	if lintCtx.Err() != nil && len(diagnostics) == 0 {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return toolError(string(queryproxy.CodeQueryTimeout), "the lint check timed out"), nil
	}
	errs, warnings := 0, 0
	for _, d := range diagnostics {
		if d.Severity == datasource.SeverityError {
			errs++
		} else {
			warnings++
		}
	}
	items, truncated := fit(append([]datasource.Diagnostic{}, diagnostics...), listBudget)
	step := "Checked a query on " + inst.UID + ": " + plural(errs, "error", "errors")
	if warnings > 0 {
		step += ", " + plural(warnings, "warning", "warnings")
	}
	return success(step, lintOut{OK: errs == 0, Diagnostics: items, Truncated: truncated}), nil
}

func checkQueryText(text string) (Result, bool) {
	switch {
	case text == "":
		return invalid("query is required"), false
	case len(text) > maxQueryBytes:
		return invalid("query must be at most %d bytes", maxQueryBytes), false
	}
	return Result{}, true
}

type instantInput struct {
	Datasource string              `json:"datasource"`
	Query      string              `json:"query"`
	From       string              `json:"from"`
	To         string              `json:"to"`
	Variables  map[string][]string `json:"variables"`

	DashboardTitle string `json:"dashboard_title"`
}

type rangeInput struct {
	instantInput
	MaxDataPoints *int `json:"max_data_points"`
}

func (r *Registry) queryRange(ctx context.Context, input json.RawMessage) (Result, error) {
	var in rangeInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	points, res, ok := boundedInt(in.MaxDataPoints, defaultDataPoints, minDataPoints, maxDataPoints, "max_data_points")
	if !ok {
		return res, nil
	}
	return r.runAdhoc(ctx, spec.Timeseries, in.instantInput, points)
}

func (r *Registry) queryInstant(ctx context.Context, input json.RawMessage) (Result, error) {
	var in instantInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	return r.runAdhoc(ctx, spec.Table, in, 0)
}

func (r *Registry) runAdhoc(ctx context.Context, panelType spec.PanelType, in instantInput, dataPoints int) (Result, error) {
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return res, nil
	}
	rng, res, ok := resolveRange(in.From, in.To, time.Now())
	if !ok {
		return res, nil
	}
	dash, selections, res, ok := adhocDashboard(inst, panelType, in)
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, toolLimits.Timeout+queryGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	out, err := r.deps.Runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:     dash,
		PanelID:       adhocPanelID,
		Range:         rng,
		Selections:    selections,
		MaxDataPoints: dataPoints,
		Limits:        toolLimits,
		User:          queryproxy.UserFor(principal),
	})
	if err != nil {
		return datasourceFailure("Ran a query on "+inst.UID, err)
	}
	summary := summarize(out)
	return success("Ran a query on "+inst.UID+": "+summary.describe(), summary), nil
}

func resolveRange(from, to string, now time.Time) (datasource.TimeRange, Result, bool) {
	if from == "" {
		from = defaultFrom
	}
	if to == "" {
		to = defaultTo
	}
	for field, value := range map[string]string{"from": from, "to": to} {
		if len(value) > maxTimeExprBytes {
			return datasource.TimeRange{}, invalid("%s must be at most %d bytes", field, maxTimeExprBytes), false
		}
	}
	start, err := spec.ResolveTime(from, now, false)
	if err != nil {
		return datasource.TimeRange{}, invalid("from: %s", err), false
	}
	end, err := spec.ResolveTime(to, now, true)
	if err != nil {
		return datasource.TimeRange{}, invalid("to: %s", err), false
	}
	switch {
	case !start.Before(end):
		return datasource.TimeRange{}, invalid("from must be before to"), false
	case end.Sub(start) > maxQueryDays*24*time.Hour:
		return datasource.TimeRange{}, invalid("the time range must be at most %d days", maxQueryDays), false
	}
	return datasource.TimeRange{From: start, To: end}, Result{}, true
}

func adhocDashboard(inst datasource.Instance, panelType spec.PanelType, in instantInput) (*spec.Dashboard, map[string]queryproxy.Selection, Result, bool) {
	if len(in.Variables) > maxVariables {
		return nil, nil, invalid("at most %d variables are allowed", maxVariables), false
	}
	if utf8.RuneCountInString(in.DashboardTitle) > maxDashTitleChars {
		return nil, nil, invalid("dashboard_title must be at most %d characters", maxDashTitleChars), false
	}
	names := slices.Sorted(maps.Keys(in.Variables))
	variables := make([]spec.Variable, 0, len(names))
	selections := make(map[string]queryproxy.Selection, len(names))
	for _, name := range names {
		if !variableName.MatchString(name) {
			return nil, nil, invalid("variable names must match %s", variableName), false
		}
		values := in.Variables[name]
		variables = append(variables, spec.Variable{Name: name, Type: spec.CustomVariable, Options: slices.Clone(values), Multi: true})
		selections[name] = queryproxy.Selection{Values: slices.Clone(values)}
	}
	dash := &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       cmp.Or(in.DashboardTitle, defaultDashTitle),
		Time:        spec.TimeRange{From: "now-1h", To: "now"},
		Variables:   variables,
		Panels: []spec.Panel{{
			ID:         adhocPanelID,
			Type:       panelType,
			Title:      adhocPanelID,
			Datasource: inst.UID,
			Queries:    []spec.Query{{Query: in.Query}},
		}},
	}
	return dash, selections, Result{}, true
}

type seriesOut struct {
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels,omitempty"`
	Points  int               `json:"points"`
	NonNull int               `json:"non_null"`
	Min     *float64          `json:"min,omitempty"`
	Max     *float64          `json:"max,omitempty"`
	Last    *float64          `json:"last,omitempty"`
}

type columnOut struct {
	Name string               `json:"name"`
	Type datasource.FieldType `json:"type"`
}

type tableOut struct {
	Columns   []columnOut `json:"columns"`
	Rows      [][]any     `json:"rows"`
	TotalRows int         `json:"total_rows"`
}

type querySummary struct {
	Kind      queryproxy.Kind `json:"kind"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	StepMS    int64           `json:"step_ms,omitempty"`
	Empty     bool            `json:"empty"`
	Frames    int             `json:"frames"`
	Truncated bool            `json:"truncated"`
	Series    []seriesOut     `json:"series,omitempty"`
	Table     *tableOut       `json:"table,omitempty"`
}

func (s querySummary) describe() string {
	switch {
	case s.Empty:
		return "no data"
	case len(s.Series) > 0 && s.Table != nil:
		return plural(len(s.Series), "series", "series") + ", " + plural(s.Table.TotalRows, "row", "rows")
	case len(s.Series) > 0:
		return plural(len(s.Series), "series", "series")
	case s.Table != nil:
		return plural(s.Table.TotalRows, "row", "rows")
	}
	return "no data"
}

func summarize(res queryproxy.Result) querySummary {
	frames := res.Frames()
	s := querySummary{
		Kind:   res.Kind,
		From:   res.Range.From.UTC().Format(time.RFC3339),
		To:     res.Range.To.UTC().Format(time.RFC3339),
		StepMS: res.Step.Milliseconds(),
		Empty:  framesEmpty(frames),
		Frames: len(frames),
	}
	if s.Empty {
		return s
	}
	var series []seriesOut
	for _, f := range frames {
		if isSeriesFrame(f) {
			series = append(series, frameSeries(f)...)
			continue
		}
		if s.Table == nil {
			s.Table = frameTable(f, &s.Truncated)
		}
	}
	if len(series) > maxSeriesOut {
		series, s.Truncated = series[:maxSeriesOut], true
	}
	var cut bool
	if s.Series, cut = fit(series, seriesBudget); cut {
		s.Truncated = true
	}
	return s
}

func framesEmpty(frames []datasource.Frame) bool {
	for _, f := range frames {
		for _, field := range f.Fields {
			if field.Type == datasource.FieldTime {
				continue
			}
			if slices.ContainsFunc(field.Values, func(v any) bool { return v != nil }) {
				return false
			}
		}
	}
	return true
}

func isSeriesFrame(f datasource.Frame) bool {
	if len(f.Fields) < 2 || f.Fields[0].Type != datasource.FieldTime {
		return false
	}
	return !slices.ContainsFunc(f.Fields[1:], func(field datasource.Field) bool { return field.Type != datasource.FieldNumber })
}

func frameSeries(f datasource.Frame) []seriesOut {
	var out []seriesOut
	for _, field := range f.Fields[1:] {
		if field.Type != datasource.FieldNumber {
			continue
		}
		out = append(out, fieldSeries(f.Labels, field))
	}
	return out
}

func fieldSeries(labels map[string]string, field datasource.Field) seriesOut {
	s := seriesOut{Name: clip(field.Name, maxStepNameBytes), Labels: clippedLabels(labels), Points: len(field.Values)}
	for _, v := range field.Values {
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			continue
		}
		s.NonNull++
		s.Last = &n
		if s.Min == nil || n < *s.Min {
			s.Min = &n
		}
		if s.Max == nil || n > *s.Max {
			s.Max = &n
		}
	}
	return s
}

func clippedLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[clip(k, maxStepNameBytes)] = clip(v, maxSummaryCell)
	}
	return out
}

func frameTable(f datasource.Frame, truncated *bool) *tableOut {
	fields := f.Fields
	if len(fields) > maxTableColumns {
		fields, *truncated = fields[:maxTableColumns], true
	}
	columns := make([]columnOut, len(fields))
	for i, field := range fields {
		columns[i] = columnOut{Name: clip(field.Name, maxStepNameBytes), Type: field.Type}
	}
	var cut bool
	if columns, cut = fit(columns, tableColumnsBudget); cut {
		*truncated = true
		fields = fields[:len(columns)]
	}
	total := f.Rows()
	rows := make([][]any, 0, min(total, maxTableRows))
	for i := range min(total, maxTableRows) {
		row := make([]any, len(fields))
		for j, field := range fields {
			if i < len(field.Values) {
				row[j] = cellValue(field.Values[i], maxSummaryCell)
			}
		}
		rows = append(rows, row)
	}
	if total > maxTableRows {
		*truncated = true
	}
	if rows, cut = fit(rows, tableRowsBudget); cut {
		*truncated = true
	}
	return &tableOut{Columns: columns, Rows: rows, TotalRows: total}
}
