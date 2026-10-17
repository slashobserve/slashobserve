package macros

import (
	"sort"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type ctxKind int

const (
	ctxCode ctxKind = iota + 1
	ctxString
	ctxComment
	ctxQuotedIdent
	ctxOpaque
)

type matcherKind int

const (
	matcherNone matcherKind = iota
	matcherEquality
	matcherRegex
)

type ctx struct {
	kind    ctxKind
	quote   byte
	matcher matcherKind
	region  int
	tagLen  int
}

type region struct {
	start, end int
	ctx        ctx
}

type lexer struct {
	regions []region
	units   []unit
}

type refIndex struct {
	covered []bool
	endsAt  map[int]bool
}

func newRefIndex(query string, refs []spec.Reference, skip func(spec.Reference) bool) refIndex {
	ri := refIndex{covered: make([]bool, len(query)+1), endsAt: map[int]bool{}}
	for _, ref := range refs {
		if skip != nil && skip(ref) {
			continue
		}
		for j := ref.Start; j < ref.End; j++ {
			ri.covered[j] = true
		}
		ri.endsAt[ref.End] = true
	}
	return ri
}

func newLexer(d Dialect, query string, refs []spec.Reference) (lexer, error) {
	if d == ClickHouse {
		return newClickHouseLexer(query, refs)
	}
	var l lexer
	codeStart := 0
	add := func(start, end int, c ctx) {
		c.region = len(l.regions)
		l.regions = append(l.regions, region{start: start, end: end, ctx: c})
	}
	for i := 0; i < len(query); {
		end, c, err := lexPromQL(query, i)
		if err != nil {
			return lexer{}, err
		}
		if c.kind == 0 {
			i++
			continue
		}
		if i > codeStart {
			add(codeStart, i, ctx{kind: ctxCode})
		}
		add(i, end, c)
		i, codeStart = end, end
	}
	if len(query) > codeStart {
		add(codeStart, len(query), ctx{kind: ctxCode})
	}
	for i := range l.regions {
		if l.regions[i].ctx.kind == ctxString {
			l.regions[i].ctx.matcher = l.promMatcher(query, i)
		}
	}
	l.units = promUnits(l.regions)
	return l, nil
}

func newClickHouseLexer(query string, refs []spec.Reference) (lexer, error) {
	tokens, err := TokenizeClickHouse(query)
	if err != nil {
		return lexer{}, err
	}
	var l lexer
	add := func(start, end int, c ctx) {
		c.region = len(l.regions)
		l.regions = append(l.regions, region{start: start, end: end, ctx: c})
	}
	codeStart := -1
	flushCode := func(end int) {
		if codeStart >= 0 {
			add(codeStart, end, ctx{kind: ctxCode})
			codeStart = -1
		}
	}
	for _, tok := range tokens {
		c, ok := regionCtx(query, tok)
		if !ok {
			if codeStart < 0 {
				codeStart = tok.Start
			}
			continue
		}
		flushCode(tok.Start)
		add(tok.Start, tok.End, c)
	}
	flushCode(len(query))
	l.units = clickHouseUnits(query, tokens)
	ri := newRefIndex(query, refs, l.startsInDelimiter)
	if err := rejectStrayDollars(query, tokens, ri); err != nil {
		return lexer{}, err
	}
	for _, r := range l.regions {
		if r.ctx.kind == ctxOpaque && r.ctx.tagLen > 0 && ri.endsAt[r.start] {
			return lexer{}, datasource.Errorf(datasource.ErrInvalidQuery, "heredoc follows a reference at offset %d", r.start)
		}
	}
	return l, nil
}

func rejectStrayDollars(query string, tokens []Token, ri refIndex) error {
	for _, tok := range tokens {
		if tok.Kind != TokenWord && tok.Kind != TokenPunct {
			continue
		}
		for j := tok.Start; j < tok.End; j++ {
			if query[j] == '$' && !ri.covered[j] {
				return datasource.Errorf(datasource.ErrInvalidQuery, "unexpected \"$\" at offset %d", j)
			}
		}
	}
	return nil
}

func regionCtx(q string, tok Token) (ctx, bool) {
	switch tok.Kind {
	case TokenComment:
		return ctx{kind: ctxComment}, true
	case TokenString:
		if q[tok.Start] == 0xE2 {
			return ctx{kind: ctxOpaque}, true
		}
		return ctx{kind: ctxString, quote: '\''}, true
	case TokenQuotedIdent:
		if q[tok.Start] == 0xE2 {
			return ctx{kind: ctxOpaque}, true
		}
		return ctx{kind: ctxQuotedIdent, quote: q[tok.Start]}, true
	case TokenHeredoc:
		return ctx{kind: ctxOpaque, tagLen: strings.IndexByte(q[tok.Start+1:], '$') + 2}, true
	}
	return ctx{}, false
}

func (l lexer) startsInDelimiter(ref spec.Reference) bool {
	c := l.contextAt(ref.Start)
	if c.kind != ctxOpaque || c.tagLen == 0 {
		return false
	}
	r := l.regions[c.region]
	return ref.Start < r.start+c.tagLen || ref.Start >= r.end-c.tagLen
}

func (l lexer) contextAt(offset int) ctx {
	i := sort.Search(len(l.regions), func(i int) bool { return l.regions[i].end > offset })
	if i == len(l.regions) {
		return ctx{kind: ctxCode, region: -1}
	}
	return l.regions[i].ctx
}

func unterminated(what string, offset int) error {
	return datasource.Errorf(datasource.ErrInvalidQuery, "unterminated %s at offset %d", what, offset)
}

func lexPromQL(q string, i int) (int, ctx, error) {
	switch q[i] {
	case '"', '\'':
		end := scanQuoted(q, i, false)
		if end < 0 || strings.IndexByte(q[i:end], '\n') >= 0 {
			return 0, ctx{}, unterminated("string", i)
		}
		return end, ctx{kind: ctxString, quote: q[i]}, nil
	case '`':
		n := strings.IndexByte(q[i+1:], '`')
		if n < 0 {
			return 0, ctx{}, unterminated("raw string", i)
		}
		return i + 1 + n + 1, ctx{kind: ctxString, quote: '`'}, nil
	case '#':
		return lineEnd(q, i, true), ctx{kind: ctxComment}, nil
	}
	return 0, ctx{}, nil
}

func scanQuoted(q string, open int, doubling bool) int {
	quote := q[open]
	for j := open + 1; j < len(q); {
		switch {
		case q[j] == '\\':
			j += 2
		case q[j] == quote && doubling && j+1 < len(q) && q[j+1] == quote:
			j += 2
		case q[j] == quote:
			return j + 1
		default:
			j++
		}
	}
	return -1
}

func lineEnd(q string, i int, cr bool) int {
	chars := "\n"
	if cr {
		chars = "\r\n"
	}
	if n := strings.IndexAny(q[i:], chars); n >= 0 {
		return i + n
	}
	return len(q)
}

func (l lexer) promMatcher(q string, idx int) matcherKind {
	k := -1
	for r := idx - 1; r >= 0 && k < 0; r-- {
		reg := l.regions[r]
		if reg.ctx.kind != ctxCode {
			if reg.ctx.kind == ctxComment {
				continue
			}
			return matcherNone
		}
		k = reg.end - 1
		for k >= reg.start && isSpace(q[k]) {
			k--
		}
		if k < reg.start {
			k = -1
			continue
		}
		var prev byte
		if k > reg.start {
			prev = q[k-1]
		}
		return promOperator(q[k], prev)
	}
	return matcherNone
}

func promOperator(op, prev byte) matcherKind {
	switch op {
	case '~':
		return matcherRegex
	case '=':
		switch prev {
		case '~':
			return matcherRegex
		case '=', '<', '>':
			return matcherNone
		}
		return matcherEquality
	}
	return matcherNone
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
