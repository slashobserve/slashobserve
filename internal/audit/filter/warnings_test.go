package filter

import (
	"slices"
	"strings"
	"testing"
)

func TestAMisspelledOperatorBetweenTwoTermsIsWarnedAbout(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"outcome:denied andd origin:mcp", []string{`"andd" is searched as text; did you mean "and"?`}},
		{"outcome:denied nad origin:mcp", []string{`"nad" is searched as text; did you mean "and"?`}},
		{"outcome:denied orr origin:mcp", []string{`"orr" is searched as text; did you mean "or"?`}},
		{"outcome:denied nott origin:mcp", []string{`"nott" is searched as text; did you mean "not"?`}},
		{"outcome:denied ANDD origin:mcp", []string{`"ANDD" is searched as text; did you mean "and"?`}},
		{"kql: outcome:denied andd origin:mcp", []string{`"andd" is searched as text; did you mean "and"?`}},
		{"(outcome:denied andd origin:mcp) or status:403", []string{`"andd" is searched as text; did you mean "and"?`}},
		{"outcome:denied andd origin:mcp annd status:403", []string{`"andd" is searched as text; did you mean "and"?`, `"annd" is searched as text; did you mean "and"?`}},
		{"lucene: outcome:denied andd origin:mcp", []string{`"andd" is searched as text; did you mean "AND"?`}},
		{"lucene: outcome:denied and origin:mcp", []string{`"and" is searched as text; did you mean "AND"?`}},
		{"lucene:a or b", []string{`"or" is searched as text; did you mean "OR"?`}},
		{"lucene:a Or b", []string{`"Or" is searched as text; did you mean "OR"?`}},
		{"lucene:a And b", []string{`"And" is searched as text; did you mean "AND"?`}},
		{"lucene:a not b", []string{`"not" is searched as text; did you mean "NOT"?`}},
		{"outcome:denied adn origin:mcp", []string{`"adn" is searched as text; did you mean "and"?`}},
		{"outcome:denied nto origin:mcp", []string{`"nto" is searched as text; did you mean "not"?`}},
		{"lucene: outcome:denied Orr origin:mcp", []string{`"Orr" is searched as text; did you mean "OR"?`}},
		{"lucene: (outcome:denied nott origin:mcp)", []string{`"nott" is searched as text; did you mean "NOT"?`}},
	} {
		if got := mustParse(t, tc.text).Warnings; !slices.Equal(got, tc.want) {
			t.Errorf("Parse(%q).Warnings = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestWordsThatAreNotMisspelledOperatorsAreNotWarnedAbout(t *testing.T) {
	for _, text := range []string{
		"",
		"outcome:denied and origin:mcp",
		"outcome:denied AND origin:mcp",
		"outcome:denied andd",
		"andd outcome:denied",
		"outcome:denied or andd origin:mcp",
		"outcome:denied andd or origin:mcp",
		`outcome:denied "andd" origin:mcp`,
		"outcome:denied and* origin:mcp",
		"outcome:denied checkout origin:mcp",
		"outcome:denied of origin:mcp",
		"login failed for user",
		"login failed for user end now",
		"outcome:denied nor origin:mcp",
		"outcome:denied add origin:mcp",
		"outcome:denied any origin:mcp",
		"outcome:denied Or origin:mcp",
		"outcome:denied Not origin:mcp",
		"lucene: login failed for user",
		"lucene: outcome:denied nor add any origin:mcp",
		"lucene: a OR b",
		"lucene: a Or* b",
		"actor_name:andd origin:mcp status:403",
		"lucene: outcome:denied AND origin:mcp",
		"lucene: outcome:denied OR andd origin:mcp",
		"lucene: outcome:denied andd AND origin:mcp",
		"lucene: outcome:denied andd",
		`lucene: outcome:denied "andd" origin:mcp`,
		`{"query":{"match_all":{}}}`,
	} {
		if got := mustParse(t, text).Warnings; len(got) != 0 {
			t.Errorf("Parse(%q).Warnings = %q, want none", text, got)
		}
	}
}

func TestAWarnedQueryStillSearchesTheWordAsText(t *testing.T) {
	_, args := whereAndArgs(t, "outcome:denied andd origin:mcp")
	found := false
	for _, arg := range args {
		if s, ok := arg.(string); ok && strings.Contains(s, "andd") {
			found = true
		}
	}
	if !found {
		t.Fatalf("args %v do not search for andd", args)
	}
}
