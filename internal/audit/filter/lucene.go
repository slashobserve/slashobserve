package filter

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const maxGroupDepth = 50

type modifier int

const (
	modNone modifier = iota
	modMust
	modMustNot
)

type luceneLeaf struct {
	field string
	wild  bool
	start int
	end   int
}

type luceneParser struct {
	src    string
	pos    int
	depth  int
	leaves []luceneLeaf
}

func parseLucene(body string, offset int) (Query, error) {
	p := &luceneParser{src: body}
	clause, err := p.parse()
	if err != nil {
		var perr *Error
		if errors.As(err, &perr) {
			perr.Pos += offset
			perr.End += offset
			return Query{}, perr
		}
		return Query{}, err
	}
	q := Query{Form: FormLucene}
	if clause != nil {
		if q.Clause, err = normalizeClause(clause); err != nil {
			return Query{}, &Error{Pos: offset, End: offset + len(body), Msg: err.Error()}
		}
	}
	if err := q.dryRun(); err != nil {
		start, end := p.span(err)
		return Query{}, &Error{Pos: offset + start, End: offset + end, Msg: err.Error()}
	}
	return q, nil
}

func (p *luceneParser) span(err error) (int, int) {
	var ce *clauseError
	if errors.As(err, &ce) {
		for _, l := range p.leaves {
			if (ce.free && l.wild) || (!ce.free && ce.field != "" && l.field == ce.field) {
				return l.start, l.end
			}
		}
	}
	return 0, len(p.src)
}

func (p *luceneParser) fail(start, end int, format string, args ...any) *Error {
	return &Error{Pos: start, End: end, Msg: fmt.Sprintf(format, args...)}
}

func (p *luceneParser) parse() (map[string]any, error) {
	clause, err := p.orExpr("")
	if err != nil {
		return nil, err
	}
	if !p.done() {
		return nil, p.fail(p.pos, p.pos+1, "unmatched )")
	}
	return clause, nil
}

func (p *luceneParser) done() bool { return p.pos >= len(p.src) }

func (p *luceneParser) peek() byte { return p.src[p.pos] }

func (p *luceneParser) skipSpace() {
	for !p.done() && isLuceneSpace(p.peek()) {
		p.pos++
	}
}

func isLuceneSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func (p *luceneParser) ahead(words ...string) (string, bool) {
	p.skipSpace()
	rest := p.src[p.pos:]
	for _, w := range words {
		if !strings.HasPrefix(rest, w) {
			continue
		}
		if w == "&&" || w == "||" {
			return w, true
		}
		if len(rest) == len(w) || isLuceneSpace(rest[len(w)]) || rest[len(w)] == '(' || rest[len(w)] == '"' {
			return w, true
		}
	}
	return "", false
}

func (p *luceneParser) take(words ...string) (string, bool) {
	w, ok := p.ahead(words...)
	if ok {
		p.pos += len(w)
	}
	return w, ok
}

func (p *luceneParser) orExpr(field string) (map[string]any, error) {
	var must, mustNot, should []any
	for {
		p.skipSpace()
		if p.done() || p.peek() == ')' {
			break
		}
		item, mod, err := p.andGroup(field)
		if err != nil {
			return nil, err
		}
		switch mod {
		case modMust:
			must = append(must, item)
		case modMustNot:
			mustNot = append(mustNot, item)
		default:
			should = append(should, item)
		}
		if w, ok := p.take("OR", "||"); ok {
			p.skipSpace()
			if p.done() || p.peek() == ')' {
				return nil, p.fail(p.pos-len(w), p.pos, "%s needs a clause after it", w)
			}
		}
	}
	return orClause(must, mustNot, should), nil
}

func orClause(must, mustNot, should []any) map[string]any {
	switch {
	case len(must)+len(mustNot)+len(should) == 0:
		return nil
	case len(must)+len(mustNot) == 0 && len(should) == 1:
		return should[0].(map[string]any)
	case len(must) == 1 && len(mustNot)+len(should) == 0:
		return must[0].(map[string]any)
	}
	b := map[string]any{}
	for key, list := range map[string][]any{"must": must, "must_not": mustNot, "should": should} {
		if len(list) > 0 {
			b[key] = list
		}
	}
	return map[string]any{"bool": b}
}

func (p *luceneParser) andGroup(field string) (map[string]any, modifier, error) {
	first, mod, err := p.unary(field)
	if err != nil {
		return nil, modNone, err
	}
	if _, ok := p.ahead("AND", "&&"); !ok {
		return first, mod, nil
	}
	var must, mustNot []any
	add := func(c map[string]any, m modifier) {
		if m == modMustNot {
			mustNot = append(mustNot, c)
		} else {
			must = append(must, c)
		}
	}
	add(first, mod)
	for {
		w, ok := p.take("AND", "&&")
		if !ok {
			break
		}
		p.skipSpace()
		if p.done() || p.peek() == ')' {
			return nil, modNone, p.fail(p.pos-len(w), p.pos, "%s needs a clause after it", w)
		}
		c, m, err := p.unary(field)
		if err != nil {
			return nil, modNone, err
		}
		add(c, m)
	}
	return orClause(must, mustNot, nil), modNone, nil
}

func (p *luceneParser) unary(field string) (map[string]any, modifier, error) {
	p.skipSpace()
	if w, ok := p.ahead("AND", "&&", "OR", "||"); ok {
		return nil, modNone, p.fail(p.pos, p.pos+len(w), "%s needs a clause before it", w)
	}
	mod := modNone
	if _, ok := p.take("NOT"); ok {
		mod = modMustNot
		p.skipSpace()
	} else if !p.done() && strings.IndexByte("+-!", p.peek()) >= 0 {
		mod = modMust
		if p.peek() != '+' {
			mod = modMustNot
		}
		p.pos++
		if p.done() || isLuceneSpace(p.peek()) || p.peek() == ')' {
			return nil, modNone, p.fail(p.pos-1, p.pos, "a %c must touch the clause it applies to", p.src[p.pos-1])
		}
	}
	if mod != modNone && p.done() {
		return nil, modNone, p.fail(p.pos-3, p.pos, "NOT needs a clause after it")
	}
	c, err := p.primary(field)
	return c, mod, err
}

func (p *luceneParser) primary(field string) (map[string]any, error) {
	if p.done() {
		return nil, p.fail(p.pos, p.pos, "expected a clause")
	}
	start := p.pos
	switch c := p.peek(); c {
	case '(':
		return p.group(field)
	case '"':
		return p.quoted(field, start)
	case '[', '{':
		return p.rangeOf(field, start, start)
	case '/':
		return nil, p.regex()
	case ')', ']', '}', '~', '^', ':':
		return nil, p.fail(p.pos, p.pos+1, "unexpected %q", c)
	}
	pieces, err := p.word(true)
	if err != nil {
		return nil, err
	}
	if p.done() || p.peek() != ':' {
		return p.after(p.value(field, pieces, start), "term")
	}
	name, err := fieldName(pieces)
	if err != nil {
		return nil, p.fail(start, p.pos, "%s", err.Error())
	}
	p.pos++
	return p.fieldValue(name, start)
}

func fieldName(pieces []wildcardPiece) (string, error) {
	var b strings.Builder
	for _, piece := range pieces {
		switch piece.kind {
		case pieceMany:
			b.WriteByte('*')
		case pieceOne:
			return "", errors.New("a field name can use * but not ?")
		default:
			b.WriteString(piece.text)
		}
	}
	if b.Len() == 0 {
		return "", errors.New("a field name is missing before the colon")
	}
	return b.String(), nil
}

func (p *luceneParser) group(field string) (map[string]any, error) {
	start := p.pos
	if p.depth++; p.depth > maxGroupDepth {
		return nil, p.fail(start, start+1, "groups are nested more than %d deep", maxGroupDepth)
	}
	p.pos++
	inner, err := p.orExpr(field)
	if err != nil {
		return nil, err
	}
	if p.done() {
		return nil, p.fail(start, start+1, "missing ) for this (")
	}
	p.pos++
	p.depth--
	if inner == nil {
		return nil, p.fail(start, p.pos, "empty group")
	}
	return p.after(inner, "group")
}

func (p *luceneParser) after(clause map[string]any, what string) (map[string]any, error) {
	if p.done() {
		return clause, nil
	}
	start := p.pos
	switch p.peek() {
	case '~':
		p.suffix()
		if what == "phrase" {
			return nil, p.fail(start, p.pos, "proximity searches (\"a b\"~n) are not supported")
		}
		return nil, p.fail(start, p.pos, "fuzzy searches (term~n) are not supported")
	case '^':
		p.suffix()
		return nil, p.fail(start, p.pos, "boosts (^n) are not supported")
	}
	return clause, nil
}

func (p *luceneParser) suffix() {
	p.pos++
	for !p.done() && (p.peek() >= '0' && p.peek() <= '9' || p.peek() == '.') {
		p.pos++
	}
}

func (p *luceneParser) regex() error {
	start := p.pos
	p.pos++
	for !p.done() && p.peek() != '/' {
		if p.peek() == '\\' {
			p.pos++
		}
		p.pos++
	}
	if p.pos < len(p.src) {
		p.pos++
	} else {
		p.pos = len(p.src)
	}
	return p.fail(start, p.pos, "regular expressions (/.../) are not supported")
}

func (p *luceneParser) fieldValue(name string, start int) (map[string]any, error) {
	if p.done() || isLuceneSpace(p.peek()) {
		return nil, p.fail(p.pos-1, p.pos, "expected a value after the :")
	}
	switch c := p.peek(); c {
	case '(':
		return p.group(name)
	case '"':
		return p.quoted(name, start)
	case '[', '{':
		return p.rangeOf(name, start, p.pos)
	case '/':
		return nil, p.regex()
	case '>', '<':
		return p.comparison(name, start)
	case ')', ']', '}', '~', '^':
		return nil, p.fail(p.pos, p.pos+1, "unexpected %q", c)
	}
	pieces, err := p.word(false)
	if err != nil {
		return nil, err
	}
	if name == "_exists_" {
		return p.after(p.exists(text(pieces), start), "term")
	}
	return p.after(p.value(name, pieces, start), "term")
}

func text(pieces []wildcardPiece) string {
	var b strings.Builder
	for _, piece := range pieces {
		switch piece.kind {
		case pieceMany:
			b.WriteByte('*')
		case pieceOne:
			b.WriteByte('?')
		default:
			b.WriteString(piece.text)
		}
	}
	return b.String()
}

func (p *luceneParser) word(colonEnds bool) ([]wildcardPiece, error) {
	var pieces []wildcardPiece
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			pieces = append(pieces, wildcardPiece{text: lit.String()})
			lit.Reset()
		}
	}
	for !p.done() {
		c := p.peek()
		if isLuceneSpace(c) || strings.IndexByte(`()"[]{}^~`, c) >= 0 || (colonEnds && c == ':') || p.operatorAt() {
			break
		}
		switch c {
		case '\\':
			if p.pos+1 >= len(p.src) {
				return nil, p.fail(p.pos, p.pos+1, "a backslash needs a character after it")
			}
			p.pos++
			lit.WriteByte(p.peek())
		case '*':
			flush()
			pieces = append(pieces, wildcardPiece{kind: pieceMany})
		case '?':
			flush()
			pieces = append(pieces, wildcardPiece{kind: pieceOne})
		default:
			lit.WriteByte(c)
		}
		p.pos++
	}
	flush()
	return pieces, nil
}

func (p *luceneParser) operatorAt() bool {
	rest := p.src[p.pos:]
	return strings.HasPrefix(rest, "&&") || strings.HasPrefix(rest, "||")
}

func (p *luceneParser) quotedText() (string, error) {
	open := p.pos
	p.pos++
	var b strings.Builder
	for {
		if p.done() {
			return "", p.fail(open, len(p.src), "missing the closing quote")
		}
		c := p.peek()
		if c == '"' {
			break
		}
		if c == '\\' && p.pos+1 < len(p.src) {
			p.pos++
			c = p.peek()
		}
		b.WriteByte(c)
		p.pos++
	}
	p.pos++
	return b.String(), nil
}

func (p *luceneParser) quoted(field string, start int) (map[string]any, error) {
	phrase, err := p.quotedText()
	if err != nil {
		return nil, err
	}
	return p.after(p.phrase(field, phrase, start), "phrase")
}

func (p *luceneParser) comparison(field string, start int) (map[string]any, error) {
	op := string(p.peek())
	p.pos++
	if !p.done() && p.peek() == '=' {
		op += "="
		p.pos++
	}
	bound, err := p.bound("a comparison")
	if err != nil {
		return nil, err
	}
	key := map[string]string{">": "gt", ">=": "gte", "<": "lt", "<=": "lte"}[op]
	return p.rangeClause(field, map[string]any{key: bound}, start), nil
}

func (p *luceneParser) bound(what string) (string, error) {
	if p.done() || isLuceneSpace(p.peek()) {
		return "", p.fail(p.pos-1, p.pos, "%s needs a value", what)
	}
	if p.peek() == '"' {
		return p.quotedText()
	}
	start := p.pos
	pieces, err := p.word(false)
	if err != nil {
		return "", err
	}
	for _, piece := range pieces {
		if piece.kind != pieceText {
			return "", p.fail(start, p.pos, "%s cannot use wildcards in its value", what)
		}
	}
	return text(pieces), nil
}

func (p *luceneParser) rangeOf(field string, start, open int) (map[string]any, error) {
	if field == "" {
		return nil, p.fail(open, open+1, "a range needs a field, as in status:[400 TO 499]")
	}
	lowerOpen := p.peek() == '{'
	p.pos++
	lower, lowerSet, err := p.rangeEnd()
	if err != nil {
		return nil, err
	}
	if _, ok := p.take("TO"); !ok {
		return nil, p.fail(open, p.pos, "a range is written [a TO b] or {a TO b}")
	}
	upper, upperSet, err := p.rangeEnd()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.done() || (p.peek() != ']' && p.peek() != '}') {
		return nil, p.fail(open, p.pos, "a range is written [a TO b] or {a TO b}")
	}
	upperOpen := p.peek() == '}'
	p.pos++
	bounds := map[string]any{}
	if lowerSet {
		bounds[map[bool]string{false: "gte", true: "gt"}[lowerOpen]] = lower
	}
	if upperSet {
		bounds[map[bool]string{false: "lte", true: "lt"}[upperOpen]] = upper
	}
	if len(bounds) == 0 {
		return p.after(p.exists(field, start), "term")
	}
	return p.after(p.rangeClause(field, bounds, start), "term")
}

func (p *luceneParser) rangeEnd() (string, bool, error) {
	p.skipSpace()
	if p.done() {
		return "", false, p.fail(p.pos, p.pos, "a range is written [a TO b] or {a TO b}")
	}
	if p.peek() == '"' {
		v, err := p.quotedText()
		return v, true, err
	}
	var b strings.Builder
	bare := true
	for !p.done() && !isLuceneSpace(p.peek()) && p.peek() != ']' && p.peek() != '}' {
		c := p.peek()
		if c == '\\' && p.pos+1 < len(p.src) {
			p.pos++
			c = p.peek()
			bare = false
		}
		b.WriteByte(c)
		p.pos++
	}
	if bare && b.String() == "*" {
		return "", false, nil
	}
	return b.String(), true, nil
}

func (p *luceneParser) record(field string, wild bool, start int) {
	p.leaves = append(p.leaves, luceneLeaf{field: field, wild: wild, start: start, end: p.pos})
}

func (p *luceneParser) exists(field string, start int) map[string]any {
	p.record(field, false, start)
	return map[string]any{"exists": map[string]any{"field": field}}
}

func (p *luceneParser) rangeClause(field string, bounds map[string]any, start int) map[string]any {
	p.record(field, false, start)
	return map[string]any{"range": map[string]any{field: bounds}}
}

func (p *luceneParser) phrase(field, phrase string, start int) map[string]any {
	p.record(field, false, start)
	switch {
	case field == "":
		return map[string]any{"multi_match": map[string]any{"query": phrase, "type": "phrase"}}
	case strings.Contains(field, "*"):
		return map[string]any{"multi_match": map[string]any{"query": phrase, "fields": []any{field}, "type": "phrase", "lenient": true}}
	}
	return map[string]any{"match_phrase": map[string]any{field: phrase}}
}

func (p *luceneParser) value(field string, pieces []wildcardPiece, start int) map[string]any {
	wild := slices.ContainsFunc(pieces, func(w wildcardPiece) bool { return w.kind != pieceText })
	p.record(field, wild && field == "", start)
	if field == "" {
		return freeTerm(pieces, wild)
	}
	return fieldTerm(field, pieces, wild)
}

func freeTerm(pieces []wildcardPiece, wild bool) map[string]any {
	if wild {
		return map[string]any{"query_string": map[string]any{"query": queryStringText(pieces)}}
	}
	return map[string]any{"multi_match": map[string]any{"query": text(pieces)}}
}

func fieldTerm(field string, pieces []wildcardPiece, wild bool) map[string]any {
	glob := strings.Contains(field, "*")
	anything := len(pieces) == 1 && pieces[0].kind == pieceMany
	switch {
	case anything && field == "*":
		return map[string]any{"match_all": map[string]any{}}
	case anything:
		return map[string]any{"exists": map[string]any{"field": field}}
	case glob && wild:
		return map[string]any{"query_string": map[string]any{"query": queryStringText(pieces), "fields": []any{field}, "lenient": true, "analyze_wildcard": true}}
	case glob:
		return map[string]any{"multi_match": map[string]any{"query": text(pieces), "fields": []any{field}, "lenient": true}}
	case wild:
		return map[string]any{"wildcard": map[string]any{field: map[string]any{"value": elasticsearchPattern(pieces)}}}
	}
	return map[string]any{"match": map[string]any{field: text(pieces)}}
}

var patternEscaper = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`)

func elasticsearchPattern(pieces []wildcardPiece) string {
	var b strings.Builder
	for _, piece := range pieces {
		switch piece.kind {
		case pieceMany:
			b.WriteByte('*')
		case pieceOne:
			b.WriteByte('?')
		default:
			b.WriteString(patternEscaper.Replace(piece.text))
		}
	}
	return b.String()
}

func queryStringText(pieces []wildcardPiece) string {
	var b strings.Builder
	for _, piece := range pieces {
		switch piece.kind {
		case pieceMany:
			b.WriteByte('*')
		case pieceOne:
			b.WriteByte('?')
		default:
			for i := 0; i < len(piece.text); i++ {
				if c := piece.text[i]; c == '\\' || strings.IndexByte(queryStringTerm, c) >= 0 {
					b.WriteByte('\\')
				}
				b.WriteByte(piece.text[i])
			}
		}
	}
	return b.String()
}
