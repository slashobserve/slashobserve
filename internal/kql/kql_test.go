package kql

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestLexGivesEveryTokenItsByteOffsets(t *testing.T) {
	type tok struct {
		kind       tokenKind
		text       string
		start, end int
	}
	for _, tc := range []struct {
		text string
		want []tok
	}{
		{"a:b", []tok{{tokWord, "a", 0, 1}, {tokColon, ":", 1, 2}, {tokWord, "b", 2, 3}, {tokEOF, "", 3, 3}}},
		{`a:"x y"`, []tok{{tokWord, "a", 0, 1}, {tokColon, ":", 1, 2}, {tokQuoted, "x y", 2, 7}, {tokEOF, "", 7, 7}}},
		{"a>=3", []tok{{tokWord, "a", 0, 1}, {tokRangeOp, ">=", 1, 3}, {tokWord, "3", 3, 4}, {tokEOF, "", 4, 4}}},
		{"a < 3", []tok{{tokWord, "a", 0, 1}, {tokRangeOp, "<", 2, 3}, {tokWord, "3", 4, 5}, {tokEOF, "", 5, 5}}},
		{`foo\:bar`, []tok{{tokWord, "foo:bar", 0, 8}, {tokEOF, "", 8, 8}}},
		{"(a or b)", []tok{{tokLParen, "(", 0, 1}, {tokWord, "a", 1, 2}, {tokOr, "or", 3, 5}, {tokWord, "b", 6, 7}, {tokRParen, ")", 7, 8}, {tokEOF, "", 8, 8}}},
		{"NOT a AND b", []tok{{tokNot, "NOT", 0, 3}, {tokWord, "a", 4, 5}, {tokAnd, "AND", 6, 9}, {tokWord, "b", 10, 11}, {tokEOF, "", 11, 11}}},
		{"x:{", []tok{{tokWord, "x", 0, 1}, {tokColon, ":", 1, 2}, {tokLBrace, "{", 2, 3}, {tokEOF, "", 3, 3}}},
		{`"a\"b\\c"`, []tok{{tokQuoted, `a"b\c`, 0, 9}, {tokEOF, "", 9, 9}}},
		{"web-*", []tok{{tokWord, "web-*", 0, 5}, {tokEOF, "", 5, 5}}},
		{"", []tok{{tokEOF, "", 0, 0}}},
	} {
		t.Run(tc.text, func(t *testing.T) {
			toks, diag := lex(tc.text)
			if diag != nil {
				t.Fatalf("lex error: %+v", *diag)
			}
			got := make([]tok, 0, len(toks))
			for _, k := range toks {
				got = append(got, tok{k.kind, k.text, k.start, k.end})
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("tokens\n got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestLexSplitsWildcardWordsIntoSegments(t *testing.T) {
	toks, diag := lex(`a\*b*c?`)
	if diag != nil {
		t.Fatalf("lex error: %+v", *diag)
	}
	want := []segment{{text: "a*b"}, {star: true}, {text: "c?"}}
	if !slices.Equal(toks[0].segments, want) {
		t.Fatalf("segments %+v, want %+v", toks[0].segments, want)
	}
	if toks[0].text != "a*b*c?" {
		t.Fatalf("text %q", toks[0].text)
	}
}

func TestLexReportsAnUnterminatedStringFromItsOpeningQuote(t *testing.T) {
	for _, text := range []string{`a:"x`, `"`, `a:"x\"`} {
		_, diag := lex(text)
		if diag == nil || diag.Message != "unterminated string" || diag.Start != strings.IndexByte(text, '"') || diag.End != len(text) {
			t.Fatalf("%q: %+v", text, diag)
		}
	}
}

type parseCase struct {
	Text  string `json:"text"`
	AST   string `json:"ast"`
	Error string `json:"error"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func TestParseMatchesTheCaseFile(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []parseCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Text, func(t *testing.T) {
			node, diags := Parse(tc.Text)
			if tc.Error != "" {
				want := Diagnostic{Severity: SeverityError, Message: tc.Error, Start: tc.Start, End: tc.End}
				if node != nil || len(diags) != 1 || diags[0] != want {
					t.Fatalf("got node %v diags %+v, want %+v", node != nil, diags, want)
				}
				return
			}
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics %+v", diags)
			}
			if got := render(node); got != tc.AST {
				t.Fatalf("ast\n got %s\nwant %s", got, tc.AST)
			}
		})
	}
}

func TestParseReturnsNoNodeForAnEmptyQuery(t *testing.T) {
	for _, text := range []string{"", "   ", "\n\t"} {
		node, diags := Parse(text)
		if node != nil || diags != nil {
			t.Fatalf("%q: node %v diags %v", text, node, diags)
		}
	}
}

func TestParseRejectsTooDeepAndTooLargeQueries(t *testing.T) {
	deep := strings.Repeat("(", 20) + "a" + strings.Repeat(")", 20)
	if _, diags := Parse(deep); len(diags) != 0 {
		t.Fatalf("20 levels: %+v", diags)
	}
	deeper := strings.Repeat("(", 21) + "a" + strings.Repeat(")", 21)
	want := Diagnostic{Severity: SeverityError, Message: "query too complex", Start: 20, End: 21}
	if _, diags := Parse(deeper); len(diags) != 1 || diags[0] != want {
		t.Fatalf("21 levels: %+v", diags)
	}
	wide := strings.TrimSpace(strings.Repeat("a ", 199))
	if _, diags := Parse(wide); len(diags) != 0 {
		t.Fatalf("199 values plus their and: %+v", diags)
	}
	wider := strings.TrimSpace(strings.Repeat("a ", 200))
	want = Diagnostic{Severity: SeverityError, Message: "query too complex", Start: 0, End: 399}
	if _, diags := Parse(wider); len(diags) != 1 || diags[0] != want {
		t.Fatalf("200 values plus their and: %+v", diags)
	}
}

func render(n *Node) string {
	var b strings.Builder
	b.WriteString("(" + string(n.Kind) + " " + strconv.Itoa(n.Start) + ":" + strconv.Itoa(n.End))
	if n.Field != "" {
		b.WriteString(" " + n.Field)
	}
	if n.Op != "" {
		b.WriteString(" " + n.Op)
	}
	if n.Kind == Match || n.Kind == Free || n.Kind == Range {
		b.WriteString(" " + strconv.Quote(n.Value))
	}
	if n.Wildcard {
		b.WriteString(" wildcard")
	}
	if n.Quoted {
		b.WriteString(" quoted")
	}
	for _, child := range n.Children {
		b.WriteString(" " + render(child))
	}
	b.WriteString(")")
	return b.String()
}
