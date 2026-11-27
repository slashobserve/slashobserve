package grounding

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const reviewLead = "Some figures, facts, ids or names in this finding do not match what this run read. Fix each item below as it says, reading again first if you need to, then call save_finding again."

type reviewed struct {
	saved
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
