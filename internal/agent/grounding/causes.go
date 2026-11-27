package grounding

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
)

const onsetAgreement = 5 * time.Minute

type causesSaved struct {
	findings.StructuredInput
	Markdown string `json:"markdown"`
}

func parseCauses(input json.RawMessage) (causesSaved, bool) {
	var in causesSaved
	if json.Unmarshal(input, &in) != nil {
		return causesSaved{}, false
	}
	if len(in.Causes) == 0 && in.RootCause != nil {
		in.LegacyFacts = CitedFacts(in.Markdown, in.Impact, in.RootCause.Summary)
	}
	return in, true
}

func CitedFacts(texts ...string) []string {
	var ids []string
	for _, text := range texts {
		for _, c := range citations(text) {
			if !slices.Contains(ids, c.id) && len(ids) < findings.MaxCauseFacts {
				ids = append(ids, c.id)
			}
		}
	}
	return ids
}

func (r *Record) CauseSignals(input json.RawMessage) []findings.Signals {
	if r == nil {
		return nil
	}
	in, ok := parseCauses(input)
	if !ok {
		return nil
	}
	ledger := r.Ledger()
	known := r.knownIDs(ledger)
	ruledOut := slices.ContainsFunc(in.RuledOut, func(line string) bool {
		return slices.ContainsFunc(citations(line), func(c citation) bool {
			_, ok := ledger.fact(c.id)
			return ok
		})
	})
	causes := in.CauseInputs()
	out := make([]findings.Signals, len(causes))
	for i, c := range causes {
		out[i] = r.signalsOf(c, ledger, known)
		out[i].RuledOut = ruledOut
	}
	return out
}

func (r *Record) signalsOf(c findings.CauseInput, ledger *Ledger, known []string) findings.Signals {
	var s findings.Signals
	addKind := func(kind string) {
		if !slices.Contains(s.Kinds, kind) {
			s.Kinds = append(s.Kinds, kind)
		}
	}
	var cited []Fact
	for _, id := range c.Evidence.Facts {
		if f, ok := ledger.fact(strings.TrimSpace(id)); ok {
			cited = append(cited, f)
			addKind(signalOf(f))
		}
	}
	for _, id := range c.Evidence.TraceIDs {
		if slices.Contains(known, strings.ToLower(strings.TrimSpace(id))) {
			addKind(findings.SignalTraces)
		}
	}
	s.Component = strings.TrimSpace(c.Component.Name) != "" && r.sawName(c.Component.Name)
	if t := c.Trigger; t != nil && strings.TrimSpace(t.Key) != "" {
		s.TriggerRead = r.sawName(t.Key)
	}
	if at, err := time.Parse(time.RFC3339, strings.TrimSpace(c.Onset)); err == nil {
		s.OnsetRead = r.queried(at)
		s.OnsetFact = slices.ContainsFunc(cited, func(f Fact) bool { return agreesWith(f, at) })
	}
	return s
}

func signalOf(f Fact) string {
	tool, _, _ := strings.Cut(f.Of, " ")
	switch {
	case f.Kind == FactTraceID, strings.HasPrefix(tool, "trace_"), tool == "get_trace":
		return findings.SignalTraces
	case strings.HasPrefix(tool, "log"):
		return findings.SignalLogs
	}
	return findings.SignalMetrics
}

func agreesWith(f Fact, onset time.Time) bool {
	if at, err := time.Parse(time.RFC3339Nano, f.At); err == nil {
		return at.Sub(onset).Abs() <= onsetAgreement
	}
	from, errFrom := time.Parse(time.RFC3339Nano, f.From)
	to, errTo := time.Parse(time.RFC3339Nano, f.To)
	return errFrom == nil && errTo == nil && !onset.Before(from) && !onset.After(to)
}

func (r *Record) causeProblems(input json.RawMessage) []Problem {
	in, ok := parseCauses(input)
	if !ok || len(in.Causes) == 0 {
		return nil
	}
	ledger := r.Ledger()
	known := r.knownIDs(ledger)
	var out []Problem
	for i, c := range in.Causes {
		out = append(out, r.namedProblems(c)...)
		out = append(out, evidenceProblems(fmt.Sprintf("causes[%d]", i), c.Evidence, ledger, known)...)
	}
	return out
}

func (r *Record) namedProblems(c findings.CauseInput) []Problem {
	var out []Problem
	if name := strings.TrimSpace(c.Component.Name); name != "" && !r.sawName(name) {
		out = append(out, Problem{Kind: findings.UnverifiedComponent, Text: name, Fix: itemAction})
	}
	if t := c.Trigger; t != nil && strings.TrimSpace(t.Key) != "" && !r.sawName(t.Key) {
		out = append(out, Problem{Kind: findings.UnverifiedTrigger, Text: strings.TrimSpace(t.Key), Fix: "read where it is set, or name it exactly as a result spelled it, or leave the trigger out"})
	}
	if onset := strings.TrimSpace(c.Onset); onset != "" {
		if when, err := time.Parse(time.RFC3339, onset); err == nil && !r.queried(when) {
			out = append(out, Problem{Kind: findings.UnverifiedOnset, Text: onset, Fix: itemAction})
		}
	}
	return out
}

func evidenceProblems(at string, e findings.EvidenceInput, ledger *Ledger, known []string) []Problem {
	var out []Problem
	for _, id := range e.Facts {
		if _, ok := ledger.fact(strings.TrimSpace(id)); !ok {
			out = append(out, Problem{Kind: findings.UnverifiedFact, Text: at + ".evidence.facts: " + strings.TrimSpace(id),
				Fix: "not a fact id a result of this run gave: cite one that was, or remove it"})
		}
	}
	for _, id := range e.TraceIDs {
		if !slices.Contains(known, strings.ToLower(strings.TrimSpace(id))) {
			out = append(out, Problem{Kind: findings.UnverifiedTraceID, Text: strings.TrimSpace(id),
				Fix: "not an id this run read: read the trace, cite an id a result gave, or remove it"})
		}
	}
	return out
}
