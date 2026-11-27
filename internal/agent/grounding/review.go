package grounding

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const reviewLead = "Some figures, facts, ids or names in this finding do not match what this run read. Fix each item below as it says, reading again first if you need to, then call save_finding again."

type reviewed struct {
	saved
	Answers   []Answer `json:"answers"`
	Title     string   `json:"title"`
	RuledOut  []string `json:"ruled_out"`
	NextSteps []string `json:"next_steps"`
}

func (in reviewed) texts() []string {
	texts := []string{in.Title, in.Markdown, in.Impact}
	if in.RootCause != nil {
		texts = append(texts, in.RootCause.Summary)
	}
	return slices.Concat(texts, in.RuledOut, in.NextSteps)
}

type problems []Problem

func (ps *problems) add(more ...Problem) {
	for _, p := range more {
		p.Text = clip(p.Text)
		if len(*ps) < findings.MaxUnverified && !slices.ContainsFunc(*ps, func(q Problem) bool { return q.Kind == p.Kind && q.Text == p.Text }) {
			*ps = append(*ps, p)
		}
	}
}

func (r *Record) Review(input json.RawMessage) []Problem {
	if r == nil {
		return nil
	}
	var in reviewed
	if json.Unmarshal(input, &in) != nil {
		return nil
	}
	ledger := r.Ledger()
	var out problems
	for _, text := range in.texts() {
		_, found := ledger.Render(text)
		out.add(found...)
	}
	out.add(ledger.AnswerProblems(in.Answers)...)
	out.add(ledger.unstated(in.Answers, in.Markdown, in.Impact)...)
	out.add(ledger.conflicts(in.Markdown, in.Impact)...)
	for _, item := range r.Check(input) {
		out.add(Problem{Kind: item.Kind, Text: item.Text, Fix: itemAction})
	}
	return out
}

func Unverified(ps []Problem) []findings.Unverified {
	if len(ps) == 0 {
		return nil
	}
	out := make([]findings.Unverified, len(ps))
	for i, p := range ps {
		out[i] = findings.Unverified{Kind: p.Kind, Text: p.Text}
	}
	return out
}

func SendBackProblems(ps []Problem) string {
	lines := []string{reviewLead}
	for _, p := range ps {
		lines = append(lines, fmt.Sprintf("- %s %q: %s", describe(p.Kind), p.Text, p.Fix))
	}
	return strings.Join(lines, "\n")
}

func (l *Ledger) matching(f Figure) (exact, near *Fact) {
	if l == nil {
		return nil, nil
	}
	spread := min(max(tolerance*f.Value, f.Step/2), maxTolerance*f.Value)
	for i := range l.Facts {
		fact := &l.Facts[i]
		if fact.Kind == FactTraceID {
			continue
		}
		v := math.Abs(fact.Value)
		for _, scale := range scalesOf(f.Unit) {
			switch off := math.Abs(f.Value*scale - v); {
			case off <= f.Step/2*scale && off <= renderTolerance*v:
				return fact, nil
			case near == nil && off <= spread*scale:
				near = fact
			}
		}
	}
	return nil, near
}

func (l *Ledger) unstated(answers []Answer, texts ...string) []Problem {
	var out []Problem
	for _, a := range answers {
		f, ok := l.fact(strings.TrimSpace(a.Fact))
		if !ok || stated(f, texts) {
			continue
		}
		out = append(out, Problem{Kind: findings.UnverifiedFact, Text: fmt.Sprintf("answers: %s = %s", a.Quantity, f.ID),
			Fix: fmt.Sprintf("the markdown never gives this answer: cite {{%s}} where it answers %q", f.ID, a.Quantity)})
	}
	return out
}

func stated(f Fact, texts []string) bool {
	one := &Ledger{Facts: []Fact{f}}
	for _, text := range texts {
		if slices.ContainsFunc(citations(text), func(c citation) bool { return c.id == f.ID }) {
			return true
		}
		if f.Kind == FactTraceID {
			if strings.Contains(strings.ToLower(text), f.Text) {
				return true
			}
			continue
		}
		for _, figure := range scan(blank(withoutCitations(text))) {
			if exact, _ := one.matching(figure); exact != nil {
				return true
			}
		}
	}
	return false
}

func withoutCitations(text string) string {
	return placeholder.ReplaceAllStringFunc(text, func(s string) string { return strings.Repeat(" ", len(s)) })
}
