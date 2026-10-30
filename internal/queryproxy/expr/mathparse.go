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
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9' || c == '.':
			end := scanNumber(text, i)
			n, err := strconv.ParseFloat(text[i:end], 64)
			if err != nil {
				return nil, fmt.Errorf("%q is not a number", text[i:end])
			}
			out = append(out, token{kind: tokNumber, num: n, text: text[i:end]})
			i = end
		case c == '$':
			name, end, ok := scanRef(text, i)
			if !ok {
				return nil, fmt.Errorf("a $ must be followed by a query ref at position %d", i)
			}
			out = append(out, token{kind: tokRef, text: name})
			i = end
		case isIdentStart(c):
			end := i
			for end < len(text) && (isIdentStart(text[end]) || text[end] >= '0' && text[end] <= '9') {
				end++
			}
			out = append(out, token{kind: tokIdent, text: text[i:end]})
			i = end
		default:
			op := matchOperator(text[i:])
			if op == "" {
				return nil, fmt.Errorf("unexpected character %q at position %d", string(c), i)
			}
			out = append(out, token{kind: tokOp, text: op})
			i += len(op)
		}
	}
	return append(out, token{kind: tokEOF}), nil
}

func scanNumber(text string, start int) int {
	end := start
	for end < len(text) && (text[end] >= '0' && text[end] <= '9' || text[end] == '.') {
		end++
	}
	if end < len(text) && (text[end] == 'e' || text[end] == 'E') {
		next := end + 1
		if next < len(text) && (text[next] == '+' || text[next] == '-') {
			next++
		}
		if next < len(text) && text[next] >= '0' && text[next] <= '9' {
			for next < len(text) && text[next] >= '0' && text[next] <= '9' {
				next++
			}
			end = next
		}
	}
	return end
}

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
