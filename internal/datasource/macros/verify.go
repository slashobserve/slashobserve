package macros

import (
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type unit struct {
	start, end int
	kind       int
	quote      byte
	container  bool
	codeRun    bool
}

func clickHouseUnits(q string, tokens []Token) []unit {
	units := make([]unit, len(tokens))
	for i, tok := range tokens {
		u := unit{start: tok.Start, end: tok.End, kind: int(tok.Kind)}
		switch tok.Kind {
		case TokenString, TokenQuotedIdent:
			u.quote = q[tok.Start]
			u.container = true
		case TokenComment, TokenHeredoc:
			u.container = true
		}
		units[i] = u
	}
	return units
}

func promUnits(regions []region) []unit {
	units := make([]unit, len(regions))
	for i, r := range regions {
		units[i] = unit{
			start:     r.start,
			end:       r.end,
			kind:      int(r.ctx.kind),
			quote:     r.ctx.quote,
			container: r.ctx.kind != ctxCode,
			codeRun:   r.ctx.kind == ctxCode,
		}
	}
	return units
}

func unitIndex(units []unit, n int) []int {
	idx := make([]int, n)
	for i, u := range units {
		for j := u.start; j < u.end; j++ {
			idx[j] = i
		}
	}
	return idx
}

func expandedUnits(d Dialect, text string) ([]unit, error) {
	if d == ClickHouse {
		tokens, err := TokenizeClickHouse(text)
		if err != nil {
			return nil, err
		}
		return clickHouseUnits(text, tokens), nil
	}
	l, err := newLexer(d, text, nil)
	if err != nil {
		return nil, err
	}
	return l.units, nil
}

func verifyExpansion(d Dialect, query string, orig []unit, exp Expansion) error {
	units, err := expandedUnits(d, exp.Text)
	if err != nil {
		return datasource.Errorf(datasource.ErrInvalidQuery, "expanded query does not tokenize: %s", err)
	}
	fail := func(offset int) error {
		return offsetErrorf(offset, "expansion changes how the query around offset ", " is tokenized")
	}
	origAt, expAt := unitIndex(orig, len(query)), unitIndex(units, len(exp.Text))
	i, j, next := 0, 0, 0
	for i < len(query) {
		if next < len(exp.spans) && exp.spans[next].origStart == i {
			i, j = exp.spans[next].origEnd, exp.spans[next].expEnd
			next++
			continue
		}
		a, b := orig[origAt[i]], units[expAt[j]]
		if a.kind != b.kind || a.quote != b.quote || (!a.codeRun && (i == a.start) != (j == b.start)) {
			return fail(i)
		}
		i++
		j++
	}
	for _, s := range exp.spans {
		if s.expEnd == s.expStart {
			continue
		}
		if err := verifySpan(s, orig[origAt[s.origStart]], units, expAt); err != nil {
			return fail(s.origStart)
		}
	}
	return nil
}

func verifySpan(s span, container unit, units []unit, expAt []int) error {
	first, last := units[expAt[s.expStart]], units[expAt[s.expEnd-1]]
	if container.container && first == last {
		if first.kind != container.kind || first.quote != container.quote {
			return errFuse
		}
		return nil
	}
	for k := expAt[s.expStart]; k <= expAt[s.expEnd-1]; k++ {
		u := units[k]
		if !u.codeRun && (u.start < s.expStart || u.end > s.expEnd) {
			return errFuse
		}
	}
	return nil
}

var errFuse = datasource.Errorf(datasource.ErrInvalidQuery, "expansion fuses with adjacent text")

func isQuotingFormat(format string) bool {
	return format == "singlequote" || format == "doublequote" || format == "sqlstring"
}

func fusesWithNeighbors(d Dialect, query string, ref spec.Reference) bool {
	return (ref.Start > 0 && isFusingByte(d, query[ref.Start-1])) || (ref.End < len(query) && isFusingByte(d, query[ref.End]))
}

func isFusingByte(d Dialect, c byte) bool {
	if isChWord(c) || c == '.' || c == '$' {
		return true
	}
	return d == PromQL && c == ':'
}

func unquotedCode(d Dialect, ref spec.Reference, c ctx) bool {
	if c.kind != ctxCode {
		return false
	}
	if ref.Kind == spec.MacroRef {
		return true
	}
	format := ref.Format
	if format == "" {
		format = defaultFormat(d, c)
	}
	return !isQuotingFormat(format)
}
