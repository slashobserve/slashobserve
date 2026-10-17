package macros

import (
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var (
	promSafeTokens = []*regexp.Regexp{
		regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_:]*$`),
		regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`),
		regexp.MustCompile(`^([0-9]+(ms|s|m|h|d|w|y))+$`),
	}
	clickHouseSafeTokens = []*regexp.Regexp{
		regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`),
		regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`),
	}

	singleQuoteEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	doubleQuoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	sqlStringEscaper   = strings.NewReplacer(`\`, `\\`, `'`, `''`)
)

func expandVariable(d Dialect, query string, ref spec.Reference, c ctx, vars []datasource.Variable) (string, error) {
	if c.kind == ctxQuotedIdent {
		return "", refErrorf(ref, "variable %s is not allowed in a quoted identifier", refLabel(ref))
	}
	if c.kind == ctxString && c.quote != '`' && oddBackslashesBefore(query, ref.Start) {
		return "", refErrorf(ref, "variable %s must not follow an escaping backslash", refLabel(ref))
	}
	if ref.Format != "" && !slices.Contains(spec.VariableFormats(), ref.Format) {
		return "", refErrorf(ref, "unknown format %q for variable %s", ref.Format, refLabel(ref))
	}
	idx := slices.IndexFunc(vars, func(v datasource.Variable) bool { return v.Name == ref.Name })
	if idx < 0 {
		return "", refErrorf(ref, "unknown variable %s", refLabel(ref))
	}
	v := vars[idx]
	if !v.All && len(v.Values) == 0 {
		return "", refErrorf(ref, "no value selected for %s", refLabel(ref))
	}
	for _, value := range v.Values {
		if hasControlByte(value) {
			return "", refErrorf(ref, "variable %s has a value with a control character", refLabel(ref))
		}
	}
	format := ref.Format
	if format == "" {
		format = defaultFormat(d, c)
	}
	text, err := applyFormat(ref, format, v)
	if err != nil {
		return "", err
	}
	return escapeForContext(d, ref, c, format, text, v.Values)
}

func oddBackslashesBefore(query string, offset int) bool {
	n := 0
	for i := offset - 1; i >= 0 && query[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

func hasControlByte(s string) bool {
	for i := range len(s) {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

func defaultFormat(d Dialect, c ctx) string {
	switch d {
	case PromQL:
		if c.kind == ctxString && c.matcher == matcherRegex {
			return "regex"
		}
		return "raw"
	default:
		if c.kind == ctxString {
			return "raw"
		}
		return "sqlstring"
	}
}

func applyFormat(ref spec.Reference, format string, v datasource.Variable) (string, error) {
	values := v.Values
	if format == "regex" && v.All {
		return ".*", nil
	}
	if format == "raw" {
		if v.All || len(values) != 1 {
			return "", refErrorf(ref, "%s has %d values; use a multi-value format", refLabel(ref), len(values))
		}
		return values[0], nil
	}
	if len(values) == 0 {
		return "", refErrorf(ref, "%s has no values", refLabel(ref))
	}
	switch format {
	case "csv":
		return strings.Join(values, ","), nil
	case "pipe":
		return strings.Join(values, "|"), nil
	case "regex":
		quoted := make([]string, len(values))
		for i, value := range values {
			quoted[i] = regexp.QuoteMeta(value)
		}
		if len(quoted) == 1 {
			return quoted[0], nil
		}
		return "(" + strings.Join(quoted, "|") + ")", nil
	case "singlequote":
		return quoteEach(values, '\'', singleQuoteEscaper), nil
	case "doublequote":
		return quoteEach(values, '"', doubleQuoteEscaper), nil
	case "sqlstring":
		return quoteEach(values, '\'', sqlStringEscaper), nil
	}
	return "", refErrorf(ref, "unknown format %q for variable %s", format, refLabel(ref))
}

func quoteEach(values []string, quote byte, escaper *strings.Replacer) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = string(quote) + escaper.Replace(value) + string(quote)
	}
	return strings.Join(quoted, ",")
}

func escapeForContext(d Dialect, ref spec.Reference, c ctx, format, text string, values []string) (string, error) {
	if c.kind == ctxString {
		switch {
		case d == PromQL && c.quote == '`':
			if strings.ContainsRune(text, '`') {
				return "", refErrorf(ref, "variable %s cannot contain a backtick in a raw string", refLabel(ref))
			}
			return text, nil
		case d == PromQL && c.quote == '"':
			return doubleQuoteEscaper.Replace(text), nil
		case d == PromQL:
			return singleQuoteEscaper.Replace(text), nil
		default:
			return singleQuoteEscaper.Replace(text), nil
		}
	}
	switch format {
	case "singlequote", "doublequote", "sqlstring":
		return text, nil
	case "regex":
		return "", refErrorf(ref, "format \"regex\" is not allowed in code for variable %s", refLabel(ref))
	}
	safe := promSafeTokens
	if d == ClickHouse {
		safe = clickHouseSafeTokens
	}
	for _, value := range values {
		if !slices.ContainsFunc(safe, func(re *regexp.Regexp) bool { return re.MatchString(value) }) {
			return "", refErrorf(ref, "a value of %s is not a valid identifier, number or duration for use in code", refLabel(ref))
		}
	}
	return text, nil
}
