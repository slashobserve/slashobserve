package kql

import (
	"strings"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokWord
	tokQuoted
	tokLParen
	tokRParen
	tokColon
	tokRangeOp
	tokAnd
	tokOr
	tokNot
	tokLBrace
)

var punctuation = map[byte]tokenKind{'(': tokLParen, ')': tokRParen, ':': tokColon, '{': tokLBrace}

var keywords = map[string]tokenKind{"and": tokAnd, "or": tokOr, "not": tokNot}

type segment struct {
	text string
	star bool
}

type token struct {
	kind       tokenKind
	text       string
	segments   []segment
	start, end int
}

type lexer struct {
	text string
	pos  int
}

func lex(text string) ([]token, *Diagnostic) {
	l := &lexer{text: text}
	var toks []token
	for {
		tok, diag := l.next()
		if diag != nil {
			return nil, diag
		}
		toks = append(toks, tok)
		if tok.kind == tokEOF {
			return toks, nil
		}
	}
}

func (l *lexer) next() (token, *Diagnostic) {
	for l.pos < len(l.text) && isSpace(l.text[l.pos]) {
		l.pos++
	}
	start := l.pos
	if l.pos >= len(l.text) {
		return token{kind: tokEOF, start: start, end: start}, nil
	}
	c := l.text[l.pos]
	if kind, ok := punctuation[c]; ok {
		l.pos++
		return token{kind: kind, text: string(c), start: start, end: l.pos}, nil
	}
	switch c {
	case '<', '>':
		l.pos++
		if l.pos < len(l.text) && l.text[l.pos] == '=' {
			l.pos++
		}
		return token{kind: tokRangeOp, text: l.text[start:l.pos], start: start, end: l.pos}, nil
	case '"':
		return l.quoted()
	}
	return l.word()
}

func (l *lexer) quoted() (token, *Diagnostic) {
	start := l.pos
	var b strings.Builder
	for i := start + 1; i < len(l.text); i++ {
		switch l.text[i] {
		case '\\':
			if i+1 < len(l.text) {
				i++
				b.WriteByte(l.text[i])
			}
		case '"':
			l.pos = i + 1
			return token{kind: tokQuoted, text: b.String(), start: start, end: l.pos}, nil
		default:
			b.WriteByte(l.text[i])
		}
	}
	return token{}, errorAt("unterminated string", start, len(l.text))
}

func (l *lexer) word() (token, *Diagnostic) {
	start := l.pos
	var segs []segment
	var lit strings.Builder
	escaped := false
	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, segment{text: lit.String()})
			lit.Reset()
		}
	}
	for l.pos < len(l.text) && isWordByte(l.text[l.pos]) {
		c := l.text[l.pos]
		switch {
		case c == '\\' && l.pos+1 < len(l.text):
			next := l.text[l.pos+1]
			if isSpace(next) {
				return token{}, errorAt("quote a value that contains spaces", l.pos, l.pos+2)
			}
			lit.WriteByte(next)
			escaped = true
			l.pos += 2
		case c == '*':
			flush()
			segs = append(segs, segment{star: true})
			l.pos++
		default:
			lit.WriteByte(c)
			l.pos++
		}
	}
	flush()
	text := joinSegments(segs)
	tok := token{kind: tokWord, text: text, segments: segs, start: start, end: l.pos}
	if kind, ok := keywords[strings.ToLower(text)]; ok && !escaped && len(segs) == 1 {
		tok.kind = kind
	}
	return tok, nil
}

func joinSegments(segs []segment) string {
	var b strings.Builder
	for _, s := range segs {
		if s.star {
			b.WriteByte('*')
		} else {
			b.WriteString(s.text)
		}
	}
	return b.String()
}

func hasStar(segs []segment) bool {
	for _, s := range segs {
		if s.star {
			return true
		}
	}
	return false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isWordByte(c byte) bool {
	return !isSpace(c) && strings.IndexByte(`():<>"{`, c) < 0
}

func errorAt(message string, start, end int) *Diagnostic {
	return &Diagnostic{Severity: SeverityError, Message: message, Start: start, End: end}
}
