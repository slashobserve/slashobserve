package macros_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

var tokenKindNames = map[macros.TokenKind]string{
	macros.TokenWhitespace:  "ws",
	macros.TokenComment:     "comment",
	macros.TokenWord:        "word",
	macros.TokenQuotedIdent: "ident",
	macros.TokenString:      "string",
	macros.TokenHeredoc:     "heredoc",
	macros.TokenNumber:      "number",
	macros.TokenPunct:       "punct",
}

func describeTokens(t *testing.T, q string) []string {
	t.Helper()
	tokens, err := macros.TokenizeClickHouse(q)
	if err != nil {
		t.Fatalf("TokenizeClickHouse(%q): %v", q, err)
	}
	var out []string
	pos := 0
	for _, tok := range tokens {
		if tok.Start != pos || tok.End <= tok.Start {
			t.Fatalf("TokenizeClickHouse(%q): token %+v does not continue at %d", q, tok, pos)
		}
		pos = tok.End
		s := tokenKindNames[tok.Kind] + ":" + q[tok.Start:tok.End]
		if tok.Kind == macros.TokenQuotedIdent {
			s += "=" + tok.Name
		}
		out = append(out, s)
	}
	if pos != len(q) {
		t.Fatalf("TokenizeClickHouse(%q): tokens end at %d, want %d", q, pos, len(q))
	}
	return out
}

func TestTokenizeClickHouse(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{"SELECT 1", []string{"word:SELECT", "ws: ", "number:1"}},
		{"a\u00a0b\u3000c", []string{"word:a", "ws:\u00a0", "word:b", "ws:\u3000", "word:c"}},
		{"a\u2028b", []string{"word:a", "ws:\u2028", "word:b"}},
		{"a -- x\nb", []string{"word:a", "ws: ", "comment:-- x", "ws:\n", "word:b"}},
		{"a // x\r\nb", []string{"word:a", "ws: ", "comment:// x\r", "ws:\n", "word:b"}},
		{"a # x\nb", []string{"word:a", "ws: ", "comment:# x", "ws:\n", "word:b"}},
		{"#!x\nb", []string{"comment:#!x", "ws:\n", "word:b"}},
		{"a /* x /* y */ z */ b", []string{"word:a", "ws: ", "comment:/* x /* y */ z */", "ws: ", "word:b"}},
		{"a-b", []string{"word:a", "punct:-", "word:b"}},
		{"a->b", []string{"word:a", "punct:->", "word:b"}},
		{"a$b$ c", []string{"word:a$b$", "ws: ", "word:c"}},
		{"$a-b$", []string{"word:$a", "punct:-", "word:b$"}},
		{"$ x", []string{"punct:$", "ws: ", "word:x"}},
		{"$", []string{"punct:$"}},
		{"$1", []string{"word:$1"}},
		{"${v}", []string{"punct:$", "punct:{", "word:v", "punct:}"}},
		{"$$ it's $$", []string{"heredoc:$$ it's $$"}},
		{"$tag$ ' $tag$ x", []string{"heredoc:$tag$ ' $tag$", "ws: ", "word:x"}},
		{"$$ abc", []string{"punct:$", "punct:$", "ws: ", "word:abc"}},
		{"$tag$ abc", []string{"word:$tag$", "ws: ", "word:abc"}},
		{"abc$$ x $$", []string{"word:abc$$", "ws: ", "word:x", "ws: ", "punct:$", "punct:$"}},
		{"1.5e-3 0x1F 0b101 1_000 (.5)", []string{"number:1.5e-3", "ws: ", "number:0x1F", "ws: ", "number:0b101", "ws: ", "number:1_000", "ws: ", "punct:(", "number:.5", "punct:)"}},
		{"t.1.1", []string{"word:t", "punct:.", "number:1", "punct:.", "number:1"}},
		{"1abc", []string{"word:1abc"}},
		{"'it''s' 'a\\'b'", []string{"string:'it''s'", "ws: ", "string:'a\\'b'"}},
		{"x'0F' b'101' X'ab'", []string{"string:x'0F'", "ws: ", "string:b'101'", "ws: ", "string:X'ab'"}},
		{"\u2018it's\u2019", []string{"string:\u2018it's\u2019"}},
		{"`a``b` \"c\\\"d\" \u201ce'f\u201d", []string{"ident:`a``b`=a`b", "ws: ", "ident:\"c\\\"d\"=c\"d", "ws: ", "ident:\u201ce'f\u201d=e'f"}},
		{"a<=>b!~*c||d::e@@f|>g", []string{"word:a", "punct:<=>", "word:b", "punct:!~*", "word:c", "punct:||", "word:d", "punct:::", "word:e", "punct:@@", "word:f", "punct:|>", "word:g"}},
		{"\\G", []string{"punct:\\G"}},
		{"a\u2212b", []string{"word:a", "punct:\u2212", "word:b"}},
	}
	for _, tc := range cases {
		got := describeTokens(t, tc.query)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("TokenizeClickHouse(%q)\n got: %q\nwant: %q", tc.query, got, tc.want)
		}
	}
}

func TestTokenizeClickHouseFailsClosed(t *testing.T) {
	for _, q := range []string{
		"'abc", "'abc\\'", `"abc`, "`abc", "/* abc", "/* a /* b */", "/*/", "$tag$ ' $other$", "abc$$ x' $$ y", "x'zz'", "x'0F",
		"#x", "#", "!", "a ! b", "\\", "\\x", "caf\u00e9", "\u00e9", "a \u200e b", "\u2018abc", "\u201cabc", "1.5x", "SELECT 1; \x00",
	} {
		_, err := macros.TokenizeClickHouse(q)
		if err == nil {
			t.Errorf("TokenizeClickHouse(%q): want error", q)
		} else if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Errorf("TokenizeClickHouse(%q): error %v is not ErrInvalidQuery", q, err)
		}
	}
}

func TestTokenizeClickHouseQuotedIdentifierNames(t *testing.T) {
	cases := []struct {
		query string
		name  string
		exact bool
	}{
		{"`url`", "url", true},
		{"`\\x75rl`", "url", true},
		{"\"\\x61b\"", "ab", true},
		{"`\\x61b`", "ab", true},
		{"`\\x41\\X`", "A\\X", true},
		{"`a``b`", "a`b", true},
		{"\"a\"\"b\"", "a\"b", true},
		{"`a\\`b`", "a`b", true},
		{"`a\\\\b`", "a\\b", true},
		{"`a\\nb`", "a\nb", true},
		{"`a\\tb\\0c`", "a\tb\x00c", true},
		{"`a\\eb`", "a\x1bb", true},
		{"`a\\%b`", "a\\%b", true},
		{"`a\\db`", "a\\db", true},
		{"`a\\/b`", "a/b", true},
		{"`a\\=b`", "a=b", true},
		{"`a\\Nb`", "ab", true},
		{"`a\\'b`", "a'b", true},
		{"\u201cabc\\x41\u201d", "abc\\x41", true},
		{"`\\x4`", "", false},
		{"`\\xZZ`", "", false},
		{"`\\x4Z`", "", false},
		{"`\\x`", "", false},
	}
	for _, tc := range cases {
		tokens, err := macros.TokenizeClickHouse(tc.query)
		if err != nil {
			t.Fatalf("TokenizeClickHouse(%q): %v", tc.query, err)
		}
		if len(tokens) != 1 || tokens[0].Kind != macros.TokenQuotedIdent {
			t.Fatalf("TokenizeClickHouse(%q) = %+v, want one quoted identifier", tc.query, tokens)
		}
		if tokens[0].NameExact != tc.exact || tokens[0].Name != tc.name {
			t.Errorf("TokenizeClickHouse(%q): Name = %q exact = %v, want %q exact = %v", tc.query, tokens[0].Name, tokens[0].NameExact, tc.name, tc.exact)
		}
	}
}
