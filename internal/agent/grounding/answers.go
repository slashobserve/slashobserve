package grounding

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/slashobserve/slashobserve/internal/findings"
)

type Answer struct {
	Quantity string `json:"quantity"`
	Fact     string `json:"fact"`
}

func normalQuantity(q string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

func agree(a, b Fact) bool {
	if a.ID == b.ID {
		return true
	}
	if a.Kind == FactTraceID || b.Kind == FactTraceID {
		return a.Text == b.Text
	}
	return relative(a.Value, b.Value) <= renderTolerance && relative(b.Value, a.Value) <= renderTolerance
}

func (l *Ledger) AnswerProblems(answers []Answer) []Problem {
	var problems []Problem
	type entry struct {
		quantity string
		fact     Fact
	}
	var seen []entry
	for _, a := range answers {
		quantity := normalQuantity(a.Quantity)
		f, ok := l.fact(strings.TrimSpace(a.Fact))
		if !ok {
			problems = append(problems, Problem{Kind: findings.UnverifiedFact, Text: fmt.Sprintf("answers: %s = %s", clip(a.Quantity), a.Fact),
				Fix: a.Fact + " is not a fact this run read: name the fact id a result gave for this quantity"})
			continue
		}
		i := slices.IndexFunc(seen, func(e entry) bool { return e.quantity == quantity })
		if i >= 0 && !agree(seen[i].fact, f) {
			problems = append(problems, Problem{Kind: findings.UnverifiedFact, Text: fmt.Sprintf("answers: %s = %s or %s", clip(a.Quantity), seen[i].fact.ID, f.ID),
				Fix: "one quantity takes one fact: make the read that settles which value is right and list only that fact"})
			continue
		}
		if i < 0 {
			seen = append(seen, entry{quantity: quantity, fact: f})
		}
	}
	return problems
}

func quantityKey(f Fact) string {
	if f.Kind == FactDuration && f.Labels["trace_id"] != "" && strings.HasSuffix(f.Of, "duration_ms") {
		return "trace " + strings.ToLower(f.Labels["trace_id"]) + " duration"
	}
	if f.Kind == FactTraceID {
		return ""
	}
	labels := slices.Sorted(maps.Keys(f.Labels))
	parts := []string{f.Of, f.From, f.To, f.At, f.question()}
	for _, k := range labels {
		parts = append(parts, k+"="+f.Labels[k])
	}
	return strings.Join(parts, "\x00")
}

func (f Fact) question() string {
	if f.Asked != "" {
		return f.Asked
	}
	return f.Query
}

func (l *Ledger) conflicts(texts ...string) []Problem {
	var cited []Fact
	for _, text := range texts {
		for _, c := range citations(text) {
			if f, ok := l.fact(c.id); ok && !slices.ContainsFunc(cited, func(g Fact) bool { return g.ID == f.ID }) {
				cited = append(cited, f)
			}
		}
	}
	var problems []Problem
	for i, a := range cited {
		key := quantityKey(a)
		if key == "" {
			continue
		}
		for _, b := range cited[:i] {
			if quantityKey(b) == key && !agree(a, b) {
				problems = append(problems, Problem{Kind: findings.UnverifiedFact, Text: fmt.Sprintf("{{%s}} and {{%s}}", b.ID, a.ID),
					Fix: fmt.Sprintf("both are %s for the same labels and window, %s and %s: make one more read that settles it and cite only that fact", a.Of, b.Text, a.Text)})
				break
			}
		}
	}
	return problems
}
