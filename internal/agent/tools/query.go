package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	lintTimeout        = 5 * time.Second
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

const (
	propPageSize = `"page_size":{"type":"integer","minimum":50,"maximum":500,"description":"Rows per page, 50 to 500; turns paging on. Prefer it to from and to windows when you need to walk results."}`
	propPage     = `"page":{"type":"integer","minimum":1,"description":"Page to read, from 1; on Loki and Tempo pass cursor instead of a page above 1."}`
	propCursor   = `"cursor":{"type":"string","maxLength":2048,"description":"The next value of an earlier page of the same query."}`

	schemaQueryInstant = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,` + propVariables + `,` + propDashTitle + `,` + propPageSize + `,` + propPage + `,` + propCursor + `},"required":["datasource","query"],"additionalProperties":false}`
)

var queryFormats = []string{"timeseries", "table", "heatmap", "logs"}

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

func init() { registerEntries((*Registry).queryEntries) }

func (r *Registry) queryEntries() []entry {
	return []entry{
		{Tool{Name: nameLintQuery, Description: descLintQuery, InputSchema: json.RawMessage(schemaLintQuery), ReadOnly: true, MinRole: auth.RoleViewer}, r.lintQuery},
		{Tool{Name: nameQueryRange, Description: descQueryRange, InputSchema: json.RawMessage(schemaQueryRange), ReadOnly: true, MinRole: auth.RoleViewer}, r.queryRange},
		{Tool{Name: nameQueryInstant, Description: descQueryInstant, InputSchema: json.RawMessage(schemaQueryInstant), ReadOnly: true, MinRole: auth.RoleViewer}, r.queryInstant},
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
	diagnostics, res, ok, err := r.lint(ctx, inst, in.Query)
	if err != nil || !ok {
		return res, err
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

func (r *Registry) lint(ctx context.Context, inst datasource.Instance, query string) ([]datasource.Diagnostic, Result, bool, error) {
	lintCtx, cancel := context.WithTimeout(ctx, lintTimeout)
	defer cancel()
	diagnostics, err := inst.Datasource.Lint(lintCtx, query)
	if err != nil {
		if err := ctx.Err(); err != nil {
			return nil, Result{}, false, err
		}
		res, err := datasourceFailure("Could not lint a query on "+inst.UID, err)
		return nil, res, false, err
	}
	if lintCtx.Err() != nil && len(diagnostics) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, Result{}, false, err
		}
		return nil, toolError(string(queryproxy.CodeQueryTimeout), "the lint check timed out"), false, nil
	}
	return diagnostics, Result{}, true, nil
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

	PageSize *int   `json:"page_size"`
	Page     *int   `json:"page"`
	Cursor   string `json:"cursor"`
}

type rangeInput struct {
	instantInput
	MaxDataPoints *int   `json:"max_data_points"`
	Format        string `json:"format"`
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
	if in.Format != "" && !slices.Contains(queryFormats, in.Format) {
		return invalid("format must be one of %s", strings.Join(queryFormats, ", ")), nil
	}
	return r.runAdhoc(ctx, spec.Timeseries, in.instantInput, in.Format, points)
}

func (r *Registry) queryInstant(ctx context.Context, input json.RawMessage) (Result, error) {
	var in instantInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.PageSize != nil {
		return r.runPage(ctx, in)
	}
	if in.Page != nil || in.Cursor != "" {
		return invalid("%s", queryproxy.PageNeedsSize), nil
	}
	return r.runAdhoc(ctx, spec.Table, in, "", 0)
}

func (r *Registry) runPage(ctx context.Context, in instantInput) (Result, error) {
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return res, nil
	}
	size, res, ok := boundedInt(in.PageSize, 0, datasource.MinPageSize, datasource.MaxPageSize, "page_size")
	if !ok {
		return res, nil
	}
	page, res, ok := boundedInt(in.Page, 1, 1, math.MaxInt32, "page")
	if !ok {
		return res, nil
	}
	if res, ok := checkLength("cursor", in.Cursor, datasource.MaxCursorBytes); !ok {
		return res, nil
	}
	rng, res, ok := resolveRange(in.From, in.To, time.Now())
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	out, err := r.deps.Runner.RunPage(ctx, inst, queryproxy.PageParams{
		UID:      inst.UID,
		Query:    datasource.Query{Text: in.Query, Limits: queryproxy.DraftLimits, User: queryproxy.UserFor(principal)},
		Range:    rng,
		Page:     page,
		PageSize: size,
		Cursor:   in.Cursor,
	})
	if err != nil {
		return datasourceFailure("Ran a query on "+inst.UID, err)
	}
	summary := summarize(queryproxy.Result{
		Queries: []queryproxy.QueryResult{{Ref: "A", Kind: queryproxy.KindInstant, Frames: out.Frames}},
		Kind:    queryproxy.KindInstant,
		Range:   out.Range,
	})
	summary.Page = pageOutOf(out)
	return success("Ran a query on "+inst.UID+": "+summary.describe(), summary), nil
}

func pageOutOf(out queryproxy.PageResult) *pageOut {
	p := &pageOut{Page: out.Info.Page, PageSize: out.Info.PageSize, TotalRelation: string(out.Info.TotalRelation), HasNext: out.Info.HasNext, Next: out.Next}
	if out.Info.TotalRelation != datasource.TotalUnknown {
		total := out.Info.Total
		p.Total = &total
	}
	return p
}

func (r *Registry) runAdhoc(ctx context.Context, panelType spec.PanelType, in instantInput, format string, dataPoints int) (Result, error) {
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
	dash, selections, res, ok := adhocDashboard(inst, panelType, in, format)
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	out, err := r.deps.Runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:     dash,
		PanelID:       adhocPanelID,
		Range:         rng,
		Selections:    selections,
		MaxDataPoints: dataPoints,
		Limits:        queryproxy.DraftLimits,
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
	start, err := spec.ResolveTime(from, now, false, spec.TimeOptions{})
	if err != nil {
		return datasource.TimeRange{}, invalid("from: %s", err), false
	}
	end, err := spec.ResolveTime(to, now, true, spec.TimeOptions{})
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

func adhocDashboard(inst datasource.Instance, panelType spec.PanelType, in instantInput, format string) (*spec.Dashboard, map[string]queryproxy.Selection, Result, bool) {
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
		variables = append(variables, spec.Variable{Name: name, Type: spec.CustomVariable, Options: spec.PlainOptions(values...), Multi: true})
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
			Queries:    []spec.Query{{Query: in.Query, Format: format}},
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

type pageOut struct {
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
	Total         *int64 `json:"total,omitempty"`
	TotalRelation string `json:"total_relation"`
	HasNext       bool   `json:"has_next"`
	Next          string `json:"next,omitempty"`
}

func (p pageOut) describe() string {
	out := " (page " + strconv.Itoa(p.Page)
	switch {
	case p.Total == nil:
		out += ", total unknown"
	case p.TotalRelation == string(datasource.TotalAtLeast):
		out += " of at least " + withCommas(*p.Total) + " rows"
	default:
		out += " of " + withCommas(*p.Total) + " rows"
	}
	if p.HasNext {
		out += "; " + queryproxy.PageWalkSentence
	}
	return out + ")"
}

func withCommas(n int64) string {
	digits := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

type querySummary struct {
	Kind      queryproxy.Kind `json:"kind"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	StepMS    int64           `json:"step_ms,omitempty"`
	Empty     bool            `json:"empty"`
	Frames    int             `json:"frames"`
	Truncated bool            `json:"truncated"`
	MoreRows  bool            `json:"more_rows,omitempty"`
	Partial   bool            `json:"partial,omitempty"`
	Series    []seriesOut     `json:"series,omitempty"`
	Table     *tableOut       `json:"table,omitempty"`
	Page      *pageOut        `json:"page,omitempty"`
}

func (s querySummary) describe() string {
	return s.describeData() + s.describeLimits()
}

func (s querySummary) describeData() string {
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

func (s querySummary) describeLimits() string {
	var out string
	if s.MoreRows {
		out += " (more rows exist; narrow from and to)"
	}
	if s.Partial {
		out += " (partial: the datasource did not search every block)"
	}
	if s.Page != nil {
		out += s.Page.describe()
	}
	return out
}

func summarize(res queryproxy.Result) querySummary {
	frames := res.Frames()
	meta := datasource.MergeMeta(frames)
	s := querySummary{
		Kind:     res.Kind,
		From:     res.Range.From.UTC().Format(time.RFC3339),
		To:       res.Range.To.UTC().Format(time.RFC3339),
		StepMS:   res.Step.Milliseconds(),
		Empty:    framesEmpty(frames),
		Frames:   len(frames),
		MoreRows: meta.Truncated,
		Partial:  meta.Partial,
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
	return f.IsSeries()
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
