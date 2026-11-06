package kql

import (
	"strconv"
	"strings"
)

const (
	maxNodes = 200
	maxDepth = 20

	tooComplex = "query too complex"
)

type parser struct {
	text  string
	toks  []token
	pos   int
	nodes int
	depth int
}

func Parse(text string) (*Node, []Diagnostic) {
	toks, diag := lex(text)
	if diag != nil {
		return nil, []Diagnostic{*diag}
	}
	p := &parser{text: text, toks: toks}
	if p.peek().kind == tokEOF {
		return nil, nil
	}
	root, diag := p.query()
	if diag == nil && p.peek().kind != tokEOF {
		diag = p.unexpected()
	}
	if diag != nil {
		return nil, []Diagnostic{*diag}
	}
	return root, nil
}

func (p *parser) peek() token {
	return p.peekAt(0)
}

func (p *parser) peekAt(n int) token {
	if p.pos+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[p.pos+n]
}

func (p *parser) advance() token {
	tok := p.peek()
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return tok
}

func (p *parser) query() (*Node, *Diagnostic) {
	return p.or()
}

func (p *parser) or() (*Node, *Diagnostic) {
	return p.list(Or, tokOr, p.and)
}

func (p *parser) and() (*Node, *Diagnostic) {
	return p.list(And, tokAnd, p.not)
}

func (p *parser) list(kind Kind, sep tokenKind, item func() (*Node, *Diagnostic)) (*Node, *Diagnostic) {
	first, diag := item()
	if diag != nil {
		return nil, diag
	}
	children := []*Node{first}
	for {
		switch {
		case p.peek().kind == sep:
			p.advance()
		case kind == And && startsPrimary(p.peek()):
		default:
			if len(children) == 1 {
				return first, nil
			}
			last := children[len(children)-1]
			return p.node(&Node{Kind: kind, Children: children, Start: first.Start, End: last.End})
		}
		next, diag := item()
		if diag != nil {
			return nil, diag
		}
		children = append(children, next)
	}
}

func (p *parser) not() (*Node, *Diagnostic) {
	if p.peek().kind != tokNot {
		return p.primary()
	}
	return p.negation(p.not)
}

func (p *parser) negation(operand func() (*Node, *Diagnostic)) (*Node, *Diagnostic) {
	tok := p.advance()
	if !startsPrimary(p.peek()) {
		return nil, errorAt("expected a value after NOT", tok.start, tok.end)
	}
	child, diag := p.nested(tok, operand)
	if diag != nil {
		return nil, diag
	}
	return p.node(&Node{Kind: Not, Children: []*Node{child}, Start: tok.start, End: child.End})
}

func (p *parser) primary() (*Node, *Diagnostic) {
	tok := p.peek()
	switch tok.kind {
	case tokLParen:
		child, end, diag := p.parens(p.query)
		if diag != nil {
			return nil, diag
		}
		return p.node(&Node{Kind: Group, Children: []*Node{child}, Start: tok.start, End: end})
	case tokWord:
		if next := p.peekAt(1).kind; next == tokColon || next == tokRangeOp {
			return p.fieldQuery()
		}
		return p.literal(Free, p.advance(), token{})
	case tokQuoted:
		return p.literal(Free, p.advance(), token{})
	case tokLBrace:
		return nil, p.nestedQuery(tok)
	}
	return nil, p.unexpected()
}

func (p *parser) fieldQuery() (*Node, *Diagnostic) {
	field := p.advance()
	op := p.advance()
	if diag := checkField(field, op); diag != nil {
		return nil, diag
	}
	if op.kind == tokRangeOp {
		return p.rangeQuery(field, op)
	}
	val := p.peek()
	switch val.kind {
	case tokLParen:
		child, end, diag := p.parens(func() (*Node, *Diagnostic) { return p.valueOr(field) })
		if diag != nil {
			return nil, diag
		}
		node := &Node{Kind: Group, Field: field.text, Children: []*Node{child}, Start: field.start, End: end}
		node.fieldStart, node.fieldEnd = field.start, field.end
		return p.node(node)
	case tokWord, tokQuoted:
		node, diag := p.literal(Match, p.advance(), field)
		if diag != nil {
			return nil, diag
		}
		node.Start = field.start
		return node, nil
	case tokLBrace:
		return nil, p.nestedQuery(val)
	}
	return nil, errorAt(`expected a value after ":"`, op.start, op.end)
}

func (p *parser) rangeQuery(field, op token) (*Node, *Diagnostic) {
	val := p.peek()
	if val.kind != tokWord && val.kind != tokQuoted {
		return nil, errorAt("expected a value after "+strconv.Quote(op.text), op.start, op.end)
	}
	p.advance()
	node := &Node{Kind: Range, Field: field.text, Op: op.text, Value: val.text, Quoted: val.kind == tokQuoted, Start: field.start, End: val.end}
	node.fieldStart, node.fieldEnd = field.start, field.end
	return p.node(node)
}

func (p *parser) valueOr(field token) (*Node, *Diagnostic) {
	return p.list(Or, tokOr, func() (*Node, *Diagnostic) { return p.valueAnd(field) })
}

func (p *parser) valueAnd(field token) (*Node, *Diagnostic) {
	return p.list(And, tokAnd, func() (*Node, *Diagnostic) { return p.valueNot(field) })
}

func (p *parser) valueNot(field token) (*Node, *Diagnostic) {
	tok := p.peek()
	switch tok.kind {
	case tokNot:
		return p.negation(func() (*Node, *Diagnostic) { return p.valueNot(field) })
	case tokLParen:
		child, end, diag := p.parens(func() (*Node, *Diagnostic) { return p.valueOr(field) })
		if diag != nil {
			return nil, diag
		}
		return p.node(&Node{Kind: Group, Children: []*Node{child}, Start: tok.start, End: end})
	case tokWord, tokQuoted:
		return p.literal(Match, p.advance(), field)
	case tokLBrace:
		return nil, p.nestedQuery(tok)
	}
	return nil, p.unexpected()
}

func (p *parser) literal(kind Kind, val, field token) (*Node, *Diagnostic) {
	node := &Node{Kind: kind, Field: field.text, Value: val.text, Quoted: val.kind == tokQuoted, Start: val.start, End: val.end}
	node.fieldStart, node.fieldEnd = field.start, field.end
	if val.kind == tokWord {
		node.Wildcard = hasStar(val.segments)
		node.segments = val.segments
		if kind == Match && len(val.segments) == 1 && val.segments[0].star {
			node.Kind, node.Value, node.Wildcard, node.segments = Exists, "", false, nil
		}
	}
	return p.node(node)
}

func (p *parser) parens(inner func() (*Node, *Diagnostic)) (*Node, int, *Diagnostic) {
	open := p.peek()
	child, diag := p.nested(open, func() (*Node, *Diagnostic) {
		p.advance()
		return inner()
	})
	if diag != nil {
		return nil, 0, diag
	}
	closer := p.peek()
	if closer.kind != tokRParen {
		if closer.kind == tokEOF {
			return nil, 0, errorAt(`expected ")"`, open.start, open.end)
		}
		return nil, 0, errorAt(`expected ")"`, closer.start, closer.end)
	}
	p.advance()
	return child, closer.end, nil
}

func (p *parser) nested(at token, inner func() (*Node, *Diagnostic)) (*Node, *Diagnostic) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxDepth {
		return nil, errorAt(tooComplex, at.start, at.end)
	}
	return inner()
}

func (p *parser) node(n *Node) (*Node, *Diagnostic) {
	p.nodes++
	if p.nodes > maxNodes {
		return nil, errorAt(tooComplex, n.Start, n.End)
	}
	return n, nil
}

func (p *parser) unexpected() *Diagnostic {
	tok := p.peek()
	if tok.kind == tokEOF {
		prev := p.toks[p.pos-1]
		return errorAt("expected a value after "+strconv.Quote(prev.text), prev.start, prev.end)
	}
	return errorAt("unexpected "+strconv.Quote(tok.text), tok.start, tok.end)
}

func (p *parser) nestedQuery(open token) *Diagnostic {
	end := len(p.text)
	depth := 0
	for i := open.start; i < len(p.text); i++ {
		switch p.text[i] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return errorAt("nested queries are not supported", open.start, i+1)
			}
		}
	}
	return errorAt("nested queries are not supported", open.start, end)
}

func checkField(field, op token) *Diagnostic {
	switch {
	case strings.HasPrefix(field.text, "*"):
		return errorAt("a field name cannot start with *", field.start, field.end)
	case field.text == "kql" && op.kind == tokColon:
		return errorAt("kql: is a form prefix, not a query", field.start, op.end)
	}
	return nil
}

func startsPrimary(tok token) bool {
	switch tok.kind {
	case tokWord, tokQuoted, tokLParen, tokNot, tokLBrace:
		return true
	}
	return false
}
