package tempo

import (
	"context"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/internal/datasource/otlphttp"
)

const variableLintPrefix = "could not lint around a variable: "

var closers = map[byte]byte{'}': '{', ')': '(', ']': '['}

type problem struct {
	message    string
	start, end int
}

func (d *Datasource) Lint(_ context.Context, query string) ([]datasource.Diagnostic, error) {
	diags := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.TraceQL, query)
	if err != nil {
		return append(diags, datasource.Diagnostic{Severity: datasource.SeverityError, Message: datasource.MessageOf(err)}), nil
	}
	p, found := checkTraceQL(exp.Text)
	if !found {
		return diags, nil
	}
	start, end, inVariable := exp.Locate(datasource.Clamp(p.start, len(exp.Text)), datasource.Clamp(p.end, len(exp.Text)))
	diag := datasource.Diagnostic{
		Severity: datasource.SeverityError,
		Message:  p.message,
		Start:    datasource.Clamp(start, len(query)),
		End:      max(datasource.Clamp(start, len(query)), datasource.Clamp(end, len(query))),
	}
	if inVariable {
		diag.Severity = datasource.SeverityWarning
		diag.Message = variableLintPrefix + diag.Message
	}
	return append(diags, diag), nil
}

func checkTraceQL(text string) (problem, bool) {
	if _, ok := otlphttp.TraceIDOf(text); ok {
		return problem{}, false
	}
	var open []int
	spanset := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch c {
		case '"', '`':
			end := stringEnd(text, i)
			if end < 0 {
				return problem{"unterminated string", i, len(text)}, true
			}
			i = end - 1
		case '{', '(', '[':
			spanset = spanset || c == '{'
			open = append(open, i)
		case '}', ')', ']':
			if len(open) == 0 || text[open[len(open)-1]] != closers[c] {
				return problem{"unexpected " + strconv.Quote(string(c)), i, i + 1}, true
			}
			open = open[:len(open)-1]
		}
	}
	if len(open) > 0 {
		at := open[len(open)-1]
		return problem{"unclosed " + strconv.Quote(string(text[at])), at, at + 1}, true
	}
	if !spanset {
		return problem{emptyQuery, 0, len(text)}, true
	}
	return problem{}, false
}

func stringEnd(text string, open int) int {
	quote := text[open]
	for j := open + 1; j < len(text); j++ {
		switch {
		case text[j] == '\\' && quote != '`':
			j++
		case text[j] == quote:
			return j + 1
		}
	}
	return -1
}
