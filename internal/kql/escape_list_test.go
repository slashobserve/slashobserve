package kql

import (
	"strconv"
	"strings"
	"testing"
)

func TestQuotedStringDecodesTheWhitespaceEscapes(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{`message:"a\nb"`, "a\nb"},
		{`message:"a\tb"`, "a\tb"},
		{`message:"a\rb"`, "a\rb"},
		{`message:"say \"hi\""`, `say "hi"`},
		{`message:"back\\slash"`, `back\slash`},
		{`message:"keep \q"`, "keep q"},
	} {
		node, diags := Parse(tc.text)
		if len(diags) != 0 || node == nil || node.Value != tc.want {
			t.Errorf("%s: node %+v diags %+v, want value %q", tc.text, node, diags, tc.want)
		}
	}
}

func valueList(n int) string {
	values := make([]string, n)
	for i := range values {
		values[i] = `"v` + strconv.Itoa(i) + `"`
	}
	return "service.name:(" + strings.Join(values, " or ") + ")"
}

func TestAValueListIsNotCountedAgainstTheNodeCap(t *testing.T) {
	if _, diags := Parse(valueList(1000)); len(diags) != 0 {
		t.Fatalf("1000 values: %+v", diags)
	}
}

func TestAValueListNextToOtherTermsStillParses(t *testing.T) {
	text := valueList(300) + ` and level:error`
	if _, diags := Parse(text); len(diags) != 0 {
		t.Fatalf("diagnostics: %+v", diags)
	}
}

func TestAValueListPastItsOwnCapIsTooComplex(t *testing.T) {
	_, diags := Parse(valueList(maxListNodes + 1))
	if len(diags) != 1 || diags[0].Message != tooComplex {
		t.Fatalf("diagnostics: %+v", diags)
	}
}

func TestTermsOutsideAValueListAreStillCapped(t *testing.T) {
	wider := strings.TrimSpace(strings.Repeat("a ", 200))
	if _, diags := Parse(wider); len(diags) != 1 {
		t.Fatalf("diagnostics: %+v", diags)
	}
}
