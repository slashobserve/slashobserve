package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	maxLintMetrics     = 20
	lintExistenceLimit = 5 * time.Second
	lookback           = time.Hour
	variableLintPrefix = "could not lint around a variable: "
)

type metricRef struct {
	name       string
	start, end int
}

func (d *Datasource) Lint(ctx context.Context, query string) []datasource.Diagnostic {
	diags := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.PromQL, query)
	if err != nil {
		return append(diags, datasource.Diagnostic{Severity: datasource.SeverityError, Message: messageOf(err)})
	}
	expr, err := promParser.ParseExpr(exp.Text)
	if err != nil {
		return append(diags, parseDiagnostics(err, exp, len(query))...)
	}
	var refs []metricRef
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		vs, ok := node.(*parser.VectorSelector)
		if !ok {
			return nil
		}
		name := selectorName(vs)
		if name == "" {
			return nil
		}
		pos := vs.PositionRange()
		start, end, inVariable := exp.Locate(clampOffset(int(pos.Start), len(exp.Text)), clampOffset(int(pos.End), len(exp.Text)))
		if inVariable || slices.ContainsFunc(refs, func(r metricRef) bool { return r.name == name }) {
			return nil
		}
		refs = append(refs, metricRef{name: name, start: clampOffset(start, len(query)), end: clampOffset(end, len(query))})
		return nil
	})
	if len(refs) > maxLintMetrics {
		refs = refs[:maxLintMetrics]
	}
	return append(diags, d.existenceDiagnostics(ctx, refs)...)
}

func selectorName(vs *parser.VectorSelector) string {
	if vs.Name != "" {
		return vs.Name
	}
	for _, m := range vs.LabelMatchers {
		if m.Name == labels.MetricName && m.Type == labels.MatchEqual {
			return m.Value
		}
	}
	return ""
}

func clampOffset(v, upper int) int {
	return max(0, min(v, upper))
}

func parseDiagnostics(err error, exp macros.Expansion, queryLen int) []datasource.Diagnostic {
	var errs parser.ParseErrors
	if single := (*parser.ParseErr)(nil); errors.As(err, &single) {
		errs = parser.ParseErrors{*single}
	} else if !errors.As(err, &errs) {
		return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: truncateText(err.Error())}}
	}
	out := make([]datasource.Diagnostic, 0, len(errs))
	for _, e := range errs {
		start := clampOffset(int(e.PositionRange.Start), len(exp.Text))
		end := clampOffset(int(e.PositionRange.End), len(exp.Text))
		origStart, origEnd, inVariable := exp.Locate(start, end)
		origStart = clampOffset(origStart, queryLen)
		origEnd = clampOffset(origEnd, queryLen)
		diag := datasource.Diagnostic{
			Severity: datasource.SeverityError,
			Message:  truncateText(e.Err.Error()),
			Start:    origStart,
			End:      max(origStart, origEnd),
		}
		if inVariable {
			diag.Severity = datasource.SeverityWarning
			diag.Message = variableLintPrefix + diag.Message
		}
		out = append(out, diag)
	}
	return out
}

func (d *Datasource) existenceDiagnostics(ctx context.Context, refs []metricRef) []datasource.Diagnostic {
	if len(refs) == 0 {
		return nil
	}
	ctx, cancel := bound(ctx, lintExistenceLimit)
	defer cancel()
	var out []datasource.Diagnostic
	for _, ref := range refs {
		now := time.Now()
		names, err := d.getStrings(ctx, "/api/v1/label/__name__/values", url.Values{
			"match[]": {metricMatcher(ref.name)},
			"start":   {unixSeconds(now.Add(-lookback))},
			"end":     {unixSeconds(now)},
		})
		if err != nil {
			return append(out, datasource.Diagnostic{
				Severity: datasource.SeverityWarning,
				Message:  "could not check metric existence: " + messageOf(err),
			})
		}
		if !slices.Contains(names, ref.name) {
			out = append(out, datasource.Diagnostic{
				Severity: datasource.SeverityError,
				Message:  "metric " + strconv.Quote(ref.name) + " has no data in the last hour",
				Start:    ref.start,
				End:      ref.end,
			})
		}
	}
	return out
}

func metricMatcher(name string) string {
	return "{__name__=" + strconv.Quote(name) + "}"
}

func messageOf(err error) string {
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return ue.Message
	}
	return err.Error()
}

func (d *Datasource) getStrings(ctx context.Context, path string, form url.Values) ([]string, error) {
	raw, err := d.do(ctx, http.MethodGet, path, form)
	if err != nil {
		return nil, err
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "upstream returned an unreadable result")
	}
	return values, nil
}
