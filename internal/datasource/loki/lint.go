package loki

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const variableLintPrefix = "could not lint around a variable: "

var (
	pipelineStages = []string{"json", "logfmt", "regexp", "pattern", "unpack", "line_format", "label_format", "drop", "keep", "decolorize", "unwrap"}
	comparisons    = []string{"==", "!=", "=~", "!~", ">=", "<=", "=", ">", "<"}
	closers        = map[byte]byte{'}': '{', ')': '(', ']': '['}
)

type problem struct {
	message    string
	start, end int
}

func (d *Datasource) Lint(_ context.Context, query string) ([]datasource.Diagnostic, error) {
	diags := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.LogQL, query)
	if err != nil {
		return append(diags, datasource.Diagnostic{Severity: datasource.SeverityError, Message: messageOf(err)}), nil
	}
	p, found := checkLogQL(exp.Text)
	if !found {
		return diags, nil
	}
	start, end, inVariable := exp.Locate(clamp(p.start, len(exp.Text)), clamp(p.end, len(exp.Text)))
	diag := datasource.Diagnostic{
		Severity: datasource.SeverityError,
		Message:  p.message,
		Start:    clamp(start, len(query)),
		End:      max(clamp(start, len(query)), clamp(end, len(query))),
	}
	if inVariable {
		diag.Severity = datasource.SeverityWarning
		diag.Message = variableLintPrefix + diag.Message
	}
	return append(diags, diag), nil
}

func checkLogQL(text string) (problem, bool) {
	var open []int
	selector := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch c {
		case '"', '`', '#':
			end, p, bad := skipLiteral(text, i)
			if bad {
				return p, true
			}
			i = end - 1
		case '{', '(', '[':
			selector = selector || c == '{'
			open = append(open, i)
		case '}', ')', ']':
			if len(open) == 0 || text[open[len(open)-1]] != closers[c] {
				return problem{"unexpected " + strconv.Quote(string(c)), i, i + 1}, true
			}
			open = open[:len(open)-1]
		case '|':
			if p, bad := checkStage(text, i); bad {
				return p, true
			}
		}
	}
	return checkEnd(text, open, selector)
}

func skipLiteral(text string, i int) (int, problem, bool) {
	if text[i] == '#' {
		if n := strings.IndexByte(text[i:], '\n'); n >= 0 {
			return i + n, problem{}, false
		}
		return len(text), problem{}, false
	}
	end := stringEnd(text, i)
	if end < 0 {
		return 0, problem{"unterminated string", i, len(text)}, true
	}
	return end, problem{}, false
}

func checkEnd(text string, open []int, selector bool) (problem, bool) {
	if len(open) > 0 {
		at := open[len(open)-1]
		return problem{"unclosed " + strconv.Quote(string(text[at])), at, at + 1}, true
	}
	if !selector {
		return problem{`a LogQL query needs a stream selector such as {service_name="checkout"}`, 0, len(text)}, true
	}
	return problem{}, false
}

func checkStage(text string, bar int) (problem, bool) {
	next := bar + 1
	if next < len(text) && strings.IndexByte("=~>", text[next]) >= 0 {
		return problem{}, false
	}
	i := skipSpace(text, next)
	if i < len(text) && text[i] == '(' {
		return problem{}, false
	}
	start := i
	for i < len(text) && isWordByte(text[i], i == start) {
		i++
	}
	name := text[start:i]
	switch {
	case name == "":
		return problem{"expected a parser, formatter or label filter after \"|\"", bar, bar + 1}, true
	case slices.Contains(pipelineStages, name):
		return problem{}, false
	}
	rest := text[skipSpace(text, i):]
	if slices.ContainsFunc(comparisons, func(op string) bool { return strings.HasPrefix(rest, op) }) {
		return problem{}, false
	}
	return problem{"unknown pipeline stage " + strconv.Quote(name), start, i}, true
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

func skipSpace(text string, i int) int {
	for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
		i++
	}
	return i
}

func isWordByte(c byte, first bool) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || (!first && '0' <= c && c <= '9')
}

func clamp(v, upper int) int {
	return max(0, min(v, upper))
}

func messageOf(err error) string {
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return ue.Message
	}
	return err.Error()
}
