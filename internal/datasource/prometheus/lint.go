package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
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

func (d *Datasource) Lint(ctx context.Context, query string) ([]datasource.Diagnostic, error) {
	diags := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.PromQL, query)
	if err != nil {
		return append(diags, datasource.Diagnostic{Severity: datasource.SeverityError, Message: messageOf(err)}), nil
	}
	expr, err := promParser.ParseExpr(exp.Text)
	if err != nil {
		return append(diags, parseDiagnostics(err, exp, len(query))...), nil
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
		spanStart, spanEnd := int(pos.Start), int(pos.End)
		if vs.Name != "" {
			spanEnd = spanStart + len(vs.Name)
		} else if nameStart, nameEnd, ok := braceNameSpan(exp.Text, spanStart); ok {
			spanStart, spanEnd = nameStart, nameEnd
		}
		start, end, inVariable := exp.Locate(clampOffset(spanStart, len(exp.Text)), clampOffset(spanEnd, len(exp.Text)))
		if inVariable || slices.ContainsFunc(refs, func(r metricRef) bool { return r.name == name }) {
			return nil
		}
		refs = append(refs, metricRef{name: name, start: clampOffset(start, len(query)), end: clampOffset(end, len(query))})
		return nil
	})
	if len(refs) > maxLintMetrics {
		refs = refs[:maxLintMetrics]
	}
	missing, err := d.existenceDiagnostics(ctx, refs)
	if err != nil {
		return nil, err
	}
	return append(diags, missing...), nil
}

func braceNameSpan(text string, open int) (int, int, bool) {
	if open < 0 || open >= len(text) || text[open] != '{' {
		return 0, 0, false
	}
	i := open + 1
	for i < len(text) {
		i = skipSpace(text, i)
		if i >= len(text) || text[i] == '}' {
			return 0, 0, false
		}
		if end := quotedEnd(text, i); end > 0 {
			if next := skipSpace(text, end); next < len(text) && (text[next] == ',' || text[next] == '}') {
				return i, end, true
			}
		}
		if start, end := nameMatcherSpan(text, i); end > 0 {
			return start, end, true
		}
		i = nextMatcher(text, i)
	}
	return 0, 0, false
}

func nameMatcherSpan(text string, i int) (int, int) {
	labelEnd := i + len(metricNameLabel)
	if quoted := quotedEnd(text, i); quoted > 0 {
		if text[i+1:quoted-1] != metricNameLabel {
			return 0, 0
		}
		labelEnd = quoted
	} else if !strings.HasPrefix(text[i:], metricNameLabel) {
		return 0, 0
	}
	j := skipSpace(text, labelEnd)
	if j >= len(text) || text[j] != '=' || strings.HasPrefix(text[j:], "=~") {
		return 0, 0
	}
	start := skipSpace(text, j+1)
	return start, quotedEnd(text, start)
}

func nextMatcher(text string, i int) int {
	for i < len(text) {
		switch c := text[i]; c {
		case ',':
			return i + 1
		case '}':
			return len(text)
		case '"', '\'', '`':
			end := quotedEnd(text, i)
			if end <= 0 {
				return len(text)
			}
			i = end
		default:
			i++
		}
	}
	return i
}

func skipSpace(text string, i int) int {
	for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
		i++
	}
	return i
}

func quotedEnd(text string, i int) int {
	if i >= len(text) {
		return 0
	}
	quote := text[i]
	if quote != '"' && quote != '\'' && quote != '`' {
		return 0
	}
	for j := i + 1; j < len(text); j++ {
		switch {
		case text[j] == '\\' && quote != '`':
			j++
		case text[j] == quote:
			return j + 1
		}
	}
	return 0
}

func selectorName(vs *parser.VectorSelector) string {
	if vs.Name != "" {
		return vs.Name
	}
	for _, m := range vs.LabelMatchers {
		if m.Name == metricNameLabel && m.Type == labels.MatchEqual {
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
	} else if !errors.As(err, &errs) || len(errs) == 0 {
		return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: truncateText(err.Error())}}
	}
	e := errs[0]
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
	return []datasource.Diagnostic{diag}
}

func (d *Datasource) existenceDiagnostics(ctx context.Context, refs []metricRef) ([]datasource.Diagnostic, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	ctx, cancel := bound(ctx, lintExistenceLimit)
	defer cancel()
	matchers := make([]string, len(refs))
	for i, ref := range refs {
		matchers[i] = metricMatcher(ref.name)
	}
	now := time.Now()
	names, err := d.getStrings(ctx, "/api/v1/label/__name__/values", url.Values{
		"match[]": matchers,
		"start":   {unixSeconds(now.Add(-lookback))},
		"end":     {unixSeconds(now)},
	})
	if err != nil {
		if datasource.IsTransient(err) {
			return nil, err
		}
		return []datasource.Diagnostic{{
			Severity: datasource.SeverityWarning,
			Message:  "could not check metric existence: " + messageOf(err),
		}}, nil
	}
	var out []datasource.Diagnostic
	for _, ref := range refs {
		if !slices.Contains(names, ref.name) {
			out = append(out, datasource.Diagnostic{
				Severity: datasource.SeverityError,
				Message:  "metric " + strconv.Quote(ref.name) + " has no data in the last hour",
				Start:    ref.start,
				End:      ref.end,
			})
		}
	}
	return out, nil
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
