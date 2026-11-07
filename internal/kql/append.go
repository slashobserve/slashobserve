package kql

import "strings"

var quoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

func Append(text, field, value string, negate bool) string {
	clause := field + ":" + Escape(value)
	if negate {
		clause = "not " + clause
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return clause
	}
	root, diags := Parse(text)
	if len(diags) > 0 || root.Kind == Or {
		return "(" + text + ") and " + clause
	}
	return text + " and " + clause
}

func Escape(value string) string {
	return `"` + quoteEscaper.Replace(value) + `"`
}
