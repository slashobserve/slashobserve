package grounding

import (
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const minCutID = 8

type hexToken struct {
	text       string
	start, end int
}

func idChar(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' || b == '-'
}

func hexTokens(text string) []hexToken {
	var out []hexToken
	for i := 0; i < len(text); {
		if !idChar(text[i]) {
			i++
			continue
		}
		j := i
		for j < len(text) && idChar(text[j]) {
			j++
		}
		if token := text[i:j]; looksLikeID(token) {
			out = append(out, hexToken{text: token, start: i, end: j})
		}
		i = j
	}
	return out
}

func looksLikeID(token string) bool {
	if len(token) < minCutID || len(token) > 32 {
		return false
	}
	digit, letter := false, false
	for i := range len(token) {
		switch c := token[i] | 0x20; {
		case token[i] >= '0' && token[i] <= '9':
			digit = true
		case c >= 'a' && c <= 'f':
			letter = true
		default:
			return false
		}
	}
	return digit && letter
}

func blankIDs(text string) string {
	tokens := hexTokens(text)
	if len(tokens) == 0 {
		return text
	}
	b := []byte(text)
	for _, t := range tokens {
		for i := t.start; i < t.end; i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

func (r *Record) knownIDs(l *Ledger) []string {
	ids := slices.Clone(r.IDs)
	if l != nil {
		for _, f := range l.Facts {
			if f.Kind == FactTraceID {
				ids = append(ids, f.Text)
			}
		}
	}
	return ids
}

func traceIDProblems(text string, known []string) []Problem {
	var problems []Problem
	for _, t := range hexTokens(blankWith(text, fencedCode, linkTarget, angleLink, bareURL)) {
		id := strings.ToLower(t.text)
		if slices.Contains(known, id) {
			continue
		}
		if full := slices.IndexFunc(known, func(k string) bool { return strings.HasPrefix(k, id) }); full >= 0 {
			problems = append(problems, Problem{Kind: findings.UnverifiedTraceID, Text: t.text,
				Fix: fmt.Sprintf("an id cut short: write the whole id %s, or cite its fact", known[full])})
			continue
		}
		if len(id) == 16 || len(id) == 32 {
			problems = append(problems, Problem{Kind: findings.UnverifiedTraceID, Text: t.text,
				Fix: "not an id this run read: read the trace, cite an id a result gave, or remove it"})
		}
	}
	return problems
}
