package clickhouse

import (
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type tokenKind int

const (
	tokWord tokenKind = iota
	tokQuoted
	tokString
	tokNumber
	tokPunct
)

type token struct {
	kind  tokenKind
	text  string
	start int
}

var forbiddenClauses = map[string]bool{"SETTINGS": true, "FORMAT": true, "INTO": true, "OUTFILE": true}

var forbiddenFunctions = map[string]bool{
	"url": true, "urlcluster": true, "file": true, "filecluster": true,
	"remote": true, "remotesecure": true, "cluster": true, "clusterallreplicas": true,
	"s3": true, "s3cluster": true, "gcs": true, "azureblobstorage": true, "azureblobstoragecluster": true,
	"hdfs": true, "hdfscluster": true, "mysql": true, "postgresql": true, "mongodb": true,
	"redis": true, "sqlite": true, "jdbc": true, "odbc": true, "executable": true, "input": true,
	"iceberg": true, "iceberglocal": true, "icebergs3": true, "icebergazure": true, "iceberghdfs": true,
	"deltalake": true, "hudi": true,
}

func guard(query string) (string, error) {
	tokens, err := lex(query)
	if err != nil {
		return "", err
	}
	if len(tokens) == 0 {
		return "", forbidden("empty query")
	}
	if !startsStatement(tokens[0]) {
		return "", forbidden("only a single SELECT statement is allowed")
	}
	end, err := statementEnd(query, tokens)
	if err != nil {
		return "", err
	}
	if err := checkTokens(tokens); err != nil {
		return "", err
	}
	return strings.TrimSpace(query[:end]), nil
}

func startsStatement(t token) bool {
	return t.kind == tokWord && (strings.EqualFold(t.text, "SELECT") || strings.EqualFold(t.text, "WITH"))
}

func statementEnd(query string, tokens []token) (int, error) {
	end := len(query)
	for i, t := range tokens {
		if t.kind != tokPunct || t.text != ";" {
			continue
		}
		if i != len(tokens)-1 {
			return 0, forbidden("multiple statements are not allowed")
		}
		end = t.start
	}
	return end, nil
}

func checkTokens(tokens []token) error {
	for i, t := range tokens {
		if t.kind == tokWord {
			if upper := strings.ToUpper(t.text); forbiddenClauses[upper] {
				return forbidden("%s clauses are not allowed", upper)
			}
		}
		isName := t.kind == tokWord || t.kind == tokQuoted
		if isName && i+1 < len(tokens) && tokens[i+1].kind == tokPunct && tokens[i+1].text == "(" {
			if name := strings.ToLower(identName(t)); forbiddenFunctions[name] {
				return forbidden("function %s() is not allowed", name)
			}
		}
	}
	return nil
}

func forbidden(format string, args ...any) error {
	return datasource.Errorf(datasource.ErrForbiddenQuery, format, args...)
}

func identName(t token) string {
	if t.kind == tokQuoted && len(t.text) >= 2 {
		return t.text[1 : len(t.text)-1]
	}
	return t.text
}

// ClickHouse accepts "#" line comments, $tag$ heredoc strings and backslash escapes in quoted
// identifiers; the lexer must agree with the server on every one of them, or a comment or
// literal could hide a second statement from the guard.
type lexer struct {
	s      string
	pos    int
	tokens []token
}

func lex(s string) ([]token, error) {
	lx := &lexer{s: s}
	for lx.pos < len(s) {
		if err := lx.next(); err != nil {
			return nil, err
		}
	}
	return lx.tokens, nil
}

func (lx *lexer) emit(kind tokenKind, end int) {
	lx.tokens = append(lx.tokens, token{kind: kind, text: lx.s[lx.pos:end], start: lx.pos})
	lx.pos = end
}

func (lx *lexer) next() error {
	s, i := lx.s, lx.pos
	c := s[i]
	switch {
	case isSpace(c):
		lx.pos++
	case c == '#' || strings.HasPrefix(s[i:], "--"):
		lx.pos = lineEnd(s, i)
	case strings.HasPrefix(s[i:], "/*"):
		return lx.blockComment()
	case c == '\'' || c == '"' || c == '`':
		return lx.quoted(c)
	case c == '$':
		return lx.dollar()
	case isWordStart(c):
		lx.emit(tokWord, scanWhile(s, i+1, isWordPart))
	case c >= '0' && c <= '9':
		lx.emit(tokNumber, scanWhile(s, i+1, func(b byte) bool { return isWordPart(b) || b == '.' }))
	default:
		lx.emit(tokPunct, i+1)
	}
	return nil
}

func (lx *lexer) blockComment() error {
	end := strings.Index(lx.s[lx.pos+2:], "*/")
	if end < 0 {
		return errUnterminated()
	}
	if strings.Contains(lx.s[lx.pos+2:lx.pos+2+end], "/*") {
		return forbidden("nested block comments are not allowed")
	}
	lx.pos += end + 4
	return nil
}

func (lx *lexer) quoted(quote byte) error {
	end, ok := quotedEnd(lx.s, lx.pos)
	if !ok {
		return errUnterminated()
	}
	if quote == '\'' {
		lx.emit(tokString, end)
	} else {
		lx.emit(tokQuoted, end)
	}
	return nil
}

func (lx *lexer) dollar() error {
	end, isHeredoc, closed := heredocEnd(lx.s, lx.pos)
	switch {
	case !isHeredoc:
		lx.emit(tokPunct, lx.pos+1)
	case !closed:
		return errUnterminated()
	default:
		lx.emit(tokString, end)
	}
	return nil
}

func errUnterminated() error {
	return datasource.Errorf(datasource.ErrInvalidQuery, "unterminated literal")
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func scanWhile(s string, from int, pred func(byte) bool) int {
	for from < len(s) && pred(s[from]) {
		from++
	}
	return from
}

func lineEnd(s string, from int) int {
	if n := strings.IndexByte(s[from:], '\n'); n >= 0 {
		return from + n + 1
	}
	return len(s)
}

func quotedEnd(s string, start int) (int, bool) {
	quote := s[start]
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case quote:
			return i + 1, true
		}
	}
	return 0, false
}

func heredocEnd(s string, start int) (end int, isHeredoc, closed bool) {
	j := start + 1
	if j < len(s) && isWordStart(s[j]) {
		j = scanWhile(s, j, isWordPart)
	}
	if j >= len(s) || s[j] != '$' {
		return 0, false, false
	}
	tag := s[start : j+1]
	idx := strings.Index(s[j+1:], tag)
	if idx < 0 {
		return 0, true, false
	}
	return j + 1 + idx + len(tag), true, true
}

func isWordStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isWordPart(c byte) bool {
	return isWordStart(c) || (c >= '0' && c <= '9')
}
