package kql_test

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/kql"
)

func TestAppendAddsAFilterClauseAndWrapsATopLevelOr(t *testing.T) {
	for _, tc := range []struct {
		name   string
		text   string
		field  string
		value  string
		negate bool
		want   string
	}{
		{"empty text", "", "a", "x", false, `a:"x"`},
		{"empty text negated", "", "a", "x", true, `not a:"x"`},
		{"single clause", "a:1", "c", "d", false, `a:1 and c:"d"`},
		{"top level and", "a:1 and b:2", "c", "d", false, `a:1 and b:2 and c:"d"`},
		{"top level or is wrapped", "a:1 or b:2", "c", "d", true, `(a:1 or b:2) and not c:"d"`},
		{"grouped or is not wrapped again", "(a:1 or b:2)", "c", "d", false, `(a:1 or b:2) and c:"d"`},
		{"top level not", "not a:1", "c", "d", false, `not a:1 and c:"d"`},
		{"value group", "a:(1 or 2)", "c", "d", false, `a:(1 or 2) and c:"d"`},
		{"surrounding whitespace is trimmed", "  a:1  ", "c", "d", false, `a:1 and c:"d"`},
		{"value with quotes and backslashes", "", "c", `x"y\z`, false, `c:"x\"y\\z"`},
		{"unparseable text is wrapped", "a:", "c", "d", false, `(a:) and c:"d"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := kql.Append(tc.text, tc.field, tc.value, tc.negate); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestEscapeQuotesAValueAndEscapesOnlyQuoteAndBackslash(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"plain", `"plain"`},
		{"", `""`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{`a\"b`, `"a\\\"b"`},
		{"with space and *", `"with space and *"`},
	} {
		if got := kql.Escape(tc.value); got != tc.want {
			t.Fatalf("%q: got %s, want %s", tc.value, got, tc.want)
		}
	}
}

func TestAppendedClauseParsesBackToTheSameValue(t *testing.T) {
	text := kql.Append("", "message", `he said "a\b"`, false)
	node, diags := kql.Parse(text)
	if len(diags) != 0 || node.Kind != kql.Match || node.Field != "message" || node.Value != `he said "a\b"` || !node.Quoted {
		t.Fatalf("%s parsed to %+v with %+v", text, node, diags)
	}
}
