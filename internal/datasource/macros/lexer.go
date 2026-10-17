package macros

import (
	"sort"
	"strings"
)

type ctxKind int

const (
	ctxCode ctxKind = iota + 1
	ctxString
	ctxComment
	ctxQuotedIdent
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
}

type region struct {
	start, end int
	ctx        ctx
}

type lexer struct {
	regions []region
}

func newLexer(d Dialect, query string) lexer {
	var l lexer
	codeStart := 0
	add := func(start, end int, c ctx) {
		c.region = len(l.regions)
		l.regions = append(l.regions, region{start: start, end: end, ctx: c})
	}
	for i := 0; i < len(query); {
		end, c, ok := lexRegion(d, query, i)
		if !ok {
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
	return l
}

func (l lexer) contextAt(offset int) ctx {
	i := sort.Search(len(l.regions), func(i int) bool { return l.regions[i].end > offset })
	if i == len(l.regions) {
		return ctx{kind: ctxCode, region: -1}
	}
	return l.regions[i].ctx
}

func lexRegion(d Dialect, q string, i int) (int, ctx, bool) {
	switch d {
	case PromQL:
		return lexPromQL(q, i)
	case ClickHouse:
		return lexClickHouse(q, i)
	}
	return 0, ctx{}, false
}

func lexPromQL(q string, i int) (int, ctx, bool) {
	switch q[i] {
	case '"', '\'':
		return scanQuoted(q, i, false), ctx{kind: ctxString, quote: q[i], matcher: promMatcher(q, i)}, true
	case '`':
		end := len(q)
		if n := strings.IndexByte(q[i+1:], '`'); n >= 0 {
			end = i + 1 + n + 1
		}
		return end, ctx{kind: ctxString, quote: '`', matcher: promMatcher(q, i)}, true
	case '#':
		return lineEnd(q, i), ctx{kind: ctxComment}, true
	}
	return 0, ctx{}, false
}

func lexClickHouse(q string, i int) (int, ctx, bool) {
	switch q[i] {
	case '\'':
		return scanQuoted(q, i, true), ctx{kind: ctxString, quote: '\''}, true
	case '"', '`':
		return scanQuoted(q, i, true), ctx{kind: ctxQuotedIdent, quote: q[i]}, true
	case '-':
		if i+1 < len(q) && q[i+1] == '-' {
			return lineEnd(q, i), ctx{kind: ctxComment}, true
		}
	case '/':
		if i+1 < len(q) && q[i+1] == '*' {
			end := len(q)
			if n := strings.Index(q[i+2:], "*/"); n >= 0 {
				end = i + 2 + n + 2
			}
			return end, ctx{kind: ctxComment}, true
		}
	}
	return 0, ctx{}, false
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
	return len(q)
}

func lineEnd(q string, i int) int {
	if n := strings.IndexByte(q[i:], '\n'); n >= 0 {
		return i + n
	}
	return len(q)
}

func promMatcher(q string, open int) matcherKind {
	k := open - 1
	for k >= 0 && (q[k] == ' ' || q[k] == '\t' || q[k] == '\n' || q[k] == '\r') {
		k--
	}
	if k < 0 {
		return matcherNone
	}
	var prev byte
	if k > 0 {
		prev = q[k-1]
	}
	switch q[k] {
	case '~':
		if prev == '=' || prev == '!' {
			return matcherRegex
		}
	case '=':
		switch prev {
		case '=', '<', '>', '~':
			return matcherNone
		}
		return matcherEquality
	}
	return matcherNone
}
