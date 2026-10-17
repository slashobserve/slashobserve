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
	if slices.ContainsFunc(v.Values, hasControlByte) {
		return "", refErrorf(ref, "variable %s has a value with a control character", refLabel(ref))
	}
	format := ref.Format
	if format == "" {
		format = defaultFormat(d, c)
	}
	regexMatcher := d == PromQL && c.kind == ctxString && c.matcher == matcherRegex
	text, err := applyFormat(ref, format, v, regexMatcher)
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

func applyFormat(ref spec.Reference, format string, v datasource.Variable, regexMatcher bool) (string, error) {
	values := v.Values
	if format == "regex" && v.All {
		return ".*", nil
	}
	if format == "raw" {
		return applyRaw(ref, v, regexMatcher)
	}
	if len(values) == 0 {
		return "", noValuesError(ref, v)
	}
	if regexMatcher && format != "regex" {
		if slices.Contains([]string{"singlequote", "doublequote", "sqlstring"}, format) {
			return "", refErrorf(ref, "format %q is not allowed in a regex matcher for variable %s", format, refLabel(ref))
		}
		values = quoteMetaEach(values)
	}
	switch format {
	case "csv":
		return strings.Join(values, ","), nil
	case "pipe":
		return strings.Join(values, "|"), nil
	case "regex":
		quoted := quoteMetaEach(values)
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

func noValuesError(ref spec.Reference, v datasource.Variable) error {
	if v.All {
		return refErrorf(ref, "%s selects All but its value list is unavailable (empty, too many or too large values, or a value that cannot be substituted); use the regex format or narrow the variable query", refLabel(ref))
	}
	return refErrorf(ref, "%s has no values", refLabel(ref))
}

func applyRaw(ref spec.Reference, v datasource.Variable, regexMatcher bool) (string, error) {
	if v.All {
		return "", refErrorf(ref, "%s selects All; use a multi-value format", refLabel(ref))
	}
	if len(v.Values) != 1 {
		return "", refErrorf(ref, "%s has %d values; use a multi-value format", refLabel(ref), len(v.Values))
	}
	if regexMatcher && regexp.QuoteMeta(v.Values[0]) != v.Values[0] {
		return "", refErrorf(ref, "format \"raw\" is not allowed in a regex matcher for a value with regex metacharacters in %s", refLabel(ref))
	}
	return v.Values[0], nil
}

func quoteMetaEach(values []string) []string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = regexp.QuoteMeta(value)
	}
	return quoted
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
	case "doublequote":
		if d == ClickHouse {
			return "", refErrorf(ref, "format \"doublequote\" is not allowed in code for variable %s", refLabel(ref))
		}
		return text, nil
	case "singlequote", "sqlstring":
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
