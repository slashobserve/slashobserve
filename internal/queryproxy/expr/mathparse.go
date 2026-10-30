package expr

import (
	"fmt"
	"strconv"
	"strings"
)

type node interface{}

type (
	numberNode struct{ value float64 }
	refNode    struct{ name string }
	unaryNode  struct {
		op string
		x  node
	}
	binaryNode struct {
		op   string
		l, r node
	}
	callNode struct {
		name string
		arg  node
	}
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokRef
	tokIdent
	tokOp
)

type token struct {
	kind tokenKind
	text string
	num  float64
}

var operators = []string{"**", "||", "&&", "==", "!=", "<=", ">=", "+", "-", "*", "/", "%", "<", ">", "!", "(", ")"}

func tokenize(text string) ([]token, error) {
	var out []token
	for i := 0; i < len(text); {
		c := text[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		tok, end, err := nextToken(text, i)
		if err != nil {
			return nil, err
		}
		out = append(out, tok)
		i = end
	}
	return append(out, token{kind: tokEOF}), nil
}

func nextToken(text string, i int) (token, int, error) {
	c := text[i]
	switch {
	case isDigit(c) || c == '.':
		end := scanNumber(text, i)
		n, err := strconv.ParseFloat(text[i:end], 64)
		if err != nil {
			return token{}, 0, fmt.Errorf("%q is not a number", text[i:end])
		}
		return token{kind: tokNumber, num: n, text: text[i:end]}, end, nil
	case c == '$':
		name, end, ok := scanRef(text, i)
		if !ok {
			return token{}, 0, fmt.Errorf("a $ must be followed by a query ref at position %d", i)
		}
		return token{kind: tokRef, text: name}, end, nil
	case isIdentStart(c):
		end := i
		for end < len(text) && (isIdentStart(text[end]) || isDigit(text[end])) {
			end++
		}
		return token{kind: tokIdent, text: text[i:end]}, end, nil
	}
	op := matchOperator(text[i:])
	if op == "" {
		return token{}, 0, fmt.Errorf("unexpected character %q at position %d", string(c), i)
	}
	return token{kind: tokOp, text: op}, i + len(op), nil
}

func scanNumber(text string, start int) int {
	end := start
	for end < len(text) && (isDigit(text[end]) || text[end] == '.') {
		end++
	}
	return scanExponent(text, end)
}

func scanExponent(text string, end int) int {
	if end >= len(text) || (text[end] != 'e' && text[end] != 'E') {
		return end
	}
	next := end + 1
	if next < len(text) && (text[next] == '+' || text[next] == '-') {
		next++
	}
	if next >= len(text) || !isDigit(text[next]) {
		return end
	}
	for next < len(text) && isDigit(text[next]) {
		next++
	}
	return next
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func scanRef(text string, at int) (string, int, bool) {
	start, braced := at+1, false
	if start < len(text) && text[start] == '{' {
		start, braced = start+1, true
	}
	end := start
	for end < len(text) && isRefByte(text[end], end == start) {
		end++
	}
	if end == start {
		return "", 0, false
	}
	if braced {
		if end >= len(text) || text[end] != '}' {
			return "", 0, false
		}
		return text[start:end], end + 1, true
	}
	return text[start:end], end, true
}

func isRefByte(c byte, first bool) bool {
	letter := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
	return letter || (!first && (c >= '0' && c <= '9' || c == '_'))
}

func isIdentStart(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'
}

func matchOperator(rest string) string {
	for _, op := range operators {
		if strings.HasPrefix(rest, op) {
			return op
		}
	}
	return ""
}

type parser struct {
	tokens []token
	at     int
}

func parseMath(text string) (node, error) {
	tokens, err := tokenize(text)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	n, err := p.binary(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tokEOF {
		return nil, fmt.Errorf("unexpected %q", t.text)
	}
	return n, nil
}

func (p *parser) peek() token { return p.tokens[p.at] }

func (p *parser) next() token {
	t := p.tokens[p.at]
	if t.kind != tokEOF {
		p.at++
	}
	return t
}

var precedence = map[string]int{
	"||": 1,
	"&&": 2,
	"==": 3, "!=": 3,
	"<": 4, "<=": 4, ">": 4, ">=": 4,
	"+": 5, "-": 5,
	"*": 6, "/": 6, "%": 6,
}

func (p *parser) binary(minPrec int) (node, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		prec, ok := precedence[t.text]
		if t.kind != tokOp || !ok || prec < minPrec {
			return left, nil
		}
		p.next()
		right, err := p.binary(prec + 1)
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: t.text, l: left, r: right}
	}
}

func (p *parser) unary() (node, error) {
	if t := p.peek(); t.kind == tokOp && (t.text == "-" || t.text == "!" || t.text == "+") {
		p.next()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		if t.text == "+" {
			return x, nil
		}
		return unaryNode{op: t.text, x: x}, nil
	}
	return p.power()
}

func (p *parser) power() (node, error) {
	base, err := p.primary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tokOp && t.text == "**" {
		p.next()
		exp, err := p.unary()
		if err != nil {
			return nil, err
		}
		return binaryNode{op: "**", l: base, r: exp}, nil
	}
	return base, nil
}

func (p *parser) primary() (node, error) {
	t := p.next()
	switch t.kind {
	case tokNumber:
		return numberNode{value: t.num}, nil
	case tokRef:
		return refNode{name: t.text}, nil
	case tokIdent:
		if _, known := functions[t.text]; !known {
			return nil, fmt.Errorf("unknown function %q", t.text)
		}
		if open := p.next(); open.text != "(" {
			return nil, fmt.Errorf("function %q needs parentheses", t.text)
		}
		arg, err := p.binary(0)
		if err != nil {
			return nil, err
		}
		if closing := p.next(); closing.text != ")" {
			return nil, fmt.Errorf("function %q takes one argument and needs a closing parenthesis", t.text)
		}
		return callNode{name: t.text, arg: arg}, nil
	case tokOp:
		if t.text == "(" {
			inner, err := p.binary(0)
			if err != nil {
				return nil, err
			}
			if closing := p.next(); closing.text != ")" {
				return nil, fmt.Errorf("missing closing parenthesis")
			}
			return inner, nil
		}
	}
	if t.kind == tokEOF {
		return nil, fmt.Errorf("the expression ends too early")
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}
