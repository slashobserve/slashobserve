package macros

import (
	"strings"
)

type TokenKind int

const (
	TokenWhitespace TokenKind = iota + 1
	TokenComment
	TokenWord
	TokenQuotedIdent
	TokenString
	TokenHeredoc
	TokenNumber
	TokenPunct
)

type Token struct {
	Kind  TokenKind
	Start int
	End   int
	Name  string

	// NameExact is false when the identifier holds an escape ClickHouse cannot decode (malformed \x or a
	// trailing backslash); consumers must not trust Name then.
	NameExact bool
}

type prevKind int

const (
	prevOther prevKind = iota
	prevDot
	prevClosing
	prevWord
	prevNumber
)

type chScanner struct {
	q    string
	pos  int
	prev prevKind
}

func TokenizeClickHouse(q string) ([]Token, error) {
	s := chScanner{q: q}
	var tokens []Token
	for s.pos < len(q) {
		tok, err := s.next()
		if err != nil {
			return nil, err
		}
		if tok.Kind != TokenWhitespace && tok.Kind != TokenComment {
			s.prev = prevOf(tok, q)
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

func prevOf(tok Token, q string) prevKind {
	switch tok.Kind {
	case TokenWord, TokenQuotedIdent:
		return prevWord
	case TokenNumber:
		return prevNumber
	case TokenPunct:
		switch q[tok.Start:tok.End] {
		case ".":
			return prevDot
		case ")", "]":
			return prevClosing
		}
	}
	return prevOther
}

func (s *chScanner) fail(what string) error {
	return offsetErrorf(s.pos, what+" at offset ", "")
}

func (s *chScanner) tok(kind TokenKind, end int) (Token, error) {
	t := Token{Kind: kind, Start: s.pos, End: end}
	s.pos = end
	return t, nil
}

func (s *chScanner) at(i int) byte {
	if i < len(s.q) {
		return s.q[i]
	}
	return 0
}

func (s *chScanner) next() (Token, error) {
	q, p := s.q, s.pos
	c := q[p]
	switch {
	case isChSpace(c):
		end := p + 1
		for end < len(q) && isChSpace(q[end]) {
			end++
		}
		return s.tok(TokenWhitespace, end)
	case isDigit(c):
		return s.number()
	}
	switch c {
	case '\'':
		return s.quoted(TokenString)
	case '"', '`':
		return s.quoted(TokenQuotedIdent)
	case '(', ')', '[', ']', '{', '}', ',', ';', '+', '*', '%', '?', '^':
		return s.tok(TokenPunct, p+1)
	case '.':
		return s.dot()
	case '-':
		return s.minus()
	case '/':
		return s.slash()
	case '#':
		return s.hash()
	case '=', '!', '~', '<', '>', ':', '|', '\\', '@':
		return s.operator(c)
	case 0xE2:
		return s.e2()
	}
	return s.wordOrOther()
}

func (s *chScanner) hash() (Token, error) {
	if n := s.at(s.pos + 1); n == ' ' || n == '!' {
		return s.tok(TokenComment, lineEnd(s.q, s.pos, false))
	}
	return Token{}, s.fail("unexpected \"#\"")
}

func (s *chScanner) dot() (Token, error) {
	q, p := s.q, s.pos
	nextIsDigit := p+1 < len(q) && isDigit(q[p+1])
	if p > 0 && (!nextIsDigit || s.prev == prevClosing || s.prev == prevWord || s.prev == prevNumber) {
		return s.tok(TokenPunct, p+1)
	}
	end := p + 1
	end = skipDigits(q, end, true, false)
	end = skipExponent(q, end, false)
	return s.tok(TokenNumber, end)
}

func (s *chScanner) minus() (Token, error) {
	p := s.pos
	switch s.at(p + 1) {
	case '>':
		return s.tok(TokenPunct, p+2)
	case '-':
		return s.tok(TokenComment, lineEnd(s.q, p, false))
	}
	return s.tok(TokenPunct, p+1)
}

func (s *chScanner) slash() (Token, error) {
	q, p := s.q, s.pos
	switch s.at(p + 1) {
	case '/':
		return s.tok(TokenComment, lineEnd(q, p, false))
	case '*':
		depth := 1
		for j := p + 2; j+2 <= len(q); {
			switch {
			case q[j] == '/' && q[j+1] == '*':
				depth++
				j += 2
			case q[j] == '*' && q[j+1] == '/':
				depth--
				j += 2
				if depth == 0 {
					return s.tok(TokenComment, j)
				}
			default:
				j++
			}
		}
		return Token{}, s.fail("unterminated block comment")
	}
	return s.tok(TokenPunct, p+1)
}

var multiCharOperators = []string{"<=>", "!~*", "==", "!=", "!~", "~*", "<=", "<>", ">=", "::", "||", "|>", "@@", "\\G"}

func (s *chScanner) operator(c byte) (Token, error) {
	rest := s.q[s.pos:]
	for _, op := range multiCharOperators {
		if strings.HasPrefix(rest, op) {
			return s.tok(TokenPunct, s.pos+len(op))
		}
	}
	switch c {
	case '!':
		return Token{}, s.fail("unexpected \"!\"")
	case '\\':
		return Token{}, s.fail("unexpected \"\\\"")
	}
	return s.tok(TokenPunct, s.pos+1)
}

func (s *chScanner) e2() (Token, error) {
	q, p := s.q, s.pos
	if p+3 <= len(q) && q[p+1] == 0x88 && q[p+2] == 0x92 {
		return s.tok(TokenPunct, p+3)
	}
	if p+5 < len(q) && q[p+1] == 0x80 && (q[p+2] == 0x98 || q[p+2] == 0x9C) {
		kind := TokenString
		if q[p+2] == 0x9C {
			kind = TokenQuotedIdent
		}
		closing := q[p+2] + 1
		for j := p + 3; j+2 < len(q); j++ {
			if q[j] == 0xE2 && q[j+1] == 0x80 && q[j+2] == closing {
				t := Token{Kind: kind, Start: p, End: j + 3}
				if kind == TokenQuotedIdent {
					t.Name, t.NameExact = q[p+3:j], true
				}
				s.pos = j + 3
				return t, nil
			}
		}
		return Token{}, s.fail("unterminated unicode quoted literal")
	}
	return s.wordOrOther()
}

func (s *chScanner) wordOrOther() (Token, error) {
	q, p := s.q, s.pos
	c := q[p]
	if c == '$' {
		if end, ok := heredocEnd(q, p); ok {
			return s.tok(TokenHeredoc, end)
		}
		if p+1 == len(q) || !isChWord(q[p+1]) {
			return s.tok(TokenPunct, p+1)
		}
	}
	if p+2 < len(q) && q[p+1] == '\'' && strings.IndexByte("xXbB", c) >= 0 {
		return s.hexOrBinString()
	}
	if isChWord(c) || c == '$' {
		end := p + 1
		for end < len(q) && (isChWord(q[end]) || q[end] == '$') {
			end++
		}
		return s.tok(TokenWord, end)
	}
	if end := skipUTF8Whitespace(q, p); end > p {
		return s.tok(TokenWhitespace, end)
	}
	return Token{}, s.fail("unexpected character")
}

func heredocEnd(q string, p int) (int, bool) {
	n := strings.IndexByte(q[p+1:], '$')
	if n < 0 {
		return 0, false
	}
	tagEnd := p + 1 + n
	for j := p + 1; j < tagEnd; j++ {
		if !isChWord(q[j]) {
			return 0, false
		}
	}
	tag := q[p : tagEnd+1]
	m := strings.Index(q[tagEnd+1:], tag)
	if m < 0 {
		return 0, false
	}
	return tagEnd + 1 + m + len(tag), true
}

func (s *chScanner) hexOrBinString() (Token, error) {
	q := s.q
	hex := q[s.pos] == 'x' || q[s.pos] == 'X'
	end := s.pos + 2
	for end < len(q) && ((hex && isHexDigit(q[end])) || (!hex && (q[end] == '0' || q[end] == '1'))) {
		end++
	}
	if end >= len(q) || q[end] != '\'' {
		return Token{}, s.fail("unterminated string")
	}
	return s.tok(TokenString, end+1)
}

func (s *chScanner) quoted(kind TokenKind) (Token, error) {
	q, p := s.q, s.pos
	end := scanQuoted(q, p, true)
	if end < 0 {
		return Token{}, s.fail("unterminated quoted literal")
	}
	t := Token{Kind: kind, Start: p, End: end}
	if kind == TokenQuotedIdent {
		t.Name, t.NameExact = unquote(q[p+1:end-1], q[p])
	}
	s.pos = end
	return t, nil
}

func unquote(body string, quote byte) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		switch {
		case body[i] == quote && i+1 < len(body) && body[i+1] == quote:
			b.WriteByte(quote)
			i++
		case body[i] != '\\':
			b.WriteByte(body[i])
		default:
			n, ok := decodeEscape(&b, body[i+1:])
			if !ok {
				return "", false
			}
			i += n
		}
	}
	return b.String(), true
}

func decodeEscape(b *strings.Builder, rest string) (int, bool) {
	switch {
	case rest == "":
		return 0, false
	case rest[0] == 'x':
		if len(rest) < 3 || !isHexDigit(rest[1]) || !isHexDigit(rest[2]) {
			return 0, false
		}
		b.WriteByte(hexValue(rest[1])<<4 | hexValue(rest[2]))
		return 3, true
	case rest[0] == 'N':
		return 1, true
	}
	decoded := unescape(rest[0])
	if !strings.ContainsRune("\\'\"`/=", rune(decoded)) && decoded > 31 {
		b.WriteByte('\\')
	}
	b.WriteByte(decoded)
	return 1, true
}

func hexValue(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

func unescape(c byte) byte {
	switch c {
	case 'a':
		return '\a'
	case 'b':
		return '\b'
	case 'e':
		return 0x1B
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'v':
		return '\v'
	case '0':
		return 0
	}
	return c
}

func (s *chScanner) number() (Token, error) {
	q, start := s.q, s.pos
	p := numberEnd(q, start, s.prev == prevDot)
	if p < len(q) && isChWord(q[p]) {
		for p < len(q) && isChWord(q[p]) {
			p++
		}
		for j := start; j < p; j++ {
			if !isChWord(q[j]) && q[j] != '$' {
				return Token{}, s.fail("wrong number")
			}
		}
		return s.tok(TokenWord, p)
	}
	return s.tok(TokenNumber, p)
}

func numberEnd(q string, p int, afterDot bool) int {
	if afterDot {
		return skipDigits(q, p+1, false, false)
	}
	startOfBlock, hex := false, false
	switch {
	case p+2 < len(q) && q[p] == '0' && (q[p+1] == 'x' || q[p+1] == 'X') && isHexDigit(q[p+2]):
		p += 2
		startOfBlock, hex = true, true
	case p+2 < len(q) && q[p] == '0' && (q[p+1] == 'b' || q[p+1] == 'B') && (q[p+2] == '0' || q[p+2] == '1'):
		p += 2
		startOfBlock = true
	default:
		p++
	}
	p = skipDigits(q, p, startOfBlock, hex)
	if p < len(q) && q[p] == '.' {
		p = skipDigits(q, p+1, true, hex)
	}
	return skipExponent(q, p, hex)
}

func skipDigits(q string, p int, startOfBlock, hex bool) int {
	for p < len(q) {
		isDig := isDigit(q[p])
		if hex {
			isDig = isHexDigit(q[p])
		}
		if !isDig && !isNumberSeparator(q, p, startOfBlock, hex) {
			break
		}
		p++
		startOfBlock = false
	}
	return p
}

func skipExponent(q string, p int, hex bool) int {
	marker := func(c byte) bool { return c == 'e' || c == 'E' }
	if hex {
		marker = func(c byte) bool { return c == 'p' || c == 'P' }
	}
	if p+1 >= len(q) || !marker(q[p]) {
		return p
	}
	p++
	if p+1 < len(q) && (q[p] == '-' || q[p] == '+') {
		p++
	}
	return skipDigits(q, p, true, false)
}

func isNumberSeparator(q string, p int, startOfBlock, hex bool) bool {
	if q[p] != '_' || startOfBlock || p+1 == len(q) {
		return false
	}
	if hex {
		return isHexDigit(q[p+1])
	}
	return isDigit(q[p+1])
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isChWord(c byte) bool {
	return c == '_' || isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isChSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func skipUTF8Whitespace(q string, p int) int {
	for p < len(q) {
		switch n := utf8WhitespaceLen(q, p); {
		case n > 0:
			p += n
		case isChSpace(q[p]):
			p++
		default:
			return p
		}
	}
	return p
}

var utf8Whitespace = func() []string {
	list := []string{"\u0085", "\u00a0", "\u180e", "\u202f", "\u2028", "\u2029", "\u205f", "\u2060", "\u3000", "\ufeff"}
	for r := '\u2000'; r <= '\u200d'; r++ {
		list = append(list, string(r))
	}
	return list
}()

func utf8WhitespaceLen(q string, p int) int {
	for _, w := range utf8Whitespace {
		if strings.HasPrefix(q[p:], w) {
			return len(w)
		}
	}
	return 0
}
