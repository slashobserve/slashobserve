package runsession

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	unclosedLead = "the finding cannot conclude while a triage candidate is unaccounted for: "
	unclosedTail = ". Close each one in note_hypotheses as cause, knock_on with knock_on_of, background, transient or unrelated, with the evidence you read, then save the finding again"

	minReadsForTransient = 2
	maxCandidateText     = 120
)

func worksCandidates(task route.Task) bool {
	return task == route.Incident || task == route.Full
}

func (s *State) leads() []triage.Lead {
	if !s.Loop || !worksCandidates(s.Task) || s.Triage == nil {
		return nil
	}
	return s.Triage.Leads
}

func (s *State) seedLedger() {
	leads := s.leads()
	if len(leads) == 0 || len(s.Ledger) > 0 {
		return
	}
	ledger := make([]revisions.Hypothesis, len(leads))
	for i, l := range leads {
		ledger[i] = revisions.Hypothesis{ID: l.ID, Text: l.Text, Status: revisions.HypothesisOpen, Next: l.Next}
	}
	s.Ledger = ledger
	s.Guard.SeedLedger(ledger)
}

func (s *State) noteTransient(next []revisions.Hypothesis) {
	for _, l := range s.leads() {
		i := slices.IndexFunc(next, func(h revisions.Hypothesis) bool { return h.ID == l.ID })
		if i < 0 || next[i].Closing != revisions.ClosingTransient {
			delete(s.TransientAt, l.ID)
			continue
		}
		if _, ok := s.TransientAt[l.ID]; ok {
			continue
		}
		if s.TransientAt == nil {
			s.TransientAt = map[string]int{}
		}
		s.TransientAt[l.ID] = s.DataReads
	}
}

func (s *State) checkCandidates(c *guard.Call) []findings.Unverified {
	leads := s.leads()
	if c.Name != toolSaveFinding || len(leads) == 0 {
		return nil
	}
	problems, open := candidateProblems(leads, s.Ledger, s.TransientAt, concludedOf(c.Input))
	if len(problems) > 0 {
		c.Unclosed = unclosedLead + strings.Join(problems, "; ") + unclosedTail
	}
	var items []findings.Unverified
	for _, l := range open {
		items = append(items, findings.Unverified{Kind: findings.UnverifiedCandidate, Text: queryproxy.ClipBytes(l.ID+" "+l.Service, maxCandidateText)})
	}
	return items
}

type concluded struct {
	noIncident bool
	names      []string
}

func concludedOf(input json.RawMessage) concluded {
	var in struct {
		NoIncident bool `json:"no_incident"`
		RootCause  *struct {
			Component string `json:"component"`
		} `json:"root_cause"`
		Causes []struct {
			Component struct {
				Name string `json:"name"`
			} `json:"component"`
		} `json:"causes"`
	}
	if json.Unmarshal(input, &in) != nil {
		return concluded{}
	}
	out := concluded{noIncident: in.NoIncident}
	for _, c := range in.Causes {
		out.names = append(out.names, strings.ToLower(strings.TrimSpace(c.Component.Name)))
	}
	if in.RootCause != nil && len(in.Causes) == 0 {
		out.names = append(out.names, strings.ToLower(strings.TrimSpace(in.RootCause.Component)))
	}
	return out
}

func (c concluded) blames(service string) bool {
	return slices.Contains(c.names, strings.ToLower(service))
}

func candidateProblems(leads []triage.Lead, ledger []revisions.Hypothesis, transientAt map[string]int, f concluded) (problems []string, open []triage.Lead) {
	byService := map[string][]revisions.Hypothesis{}
	for _, l := range leads {
		if i := hypothesisOf(ledger, l.ID); i >= 0 {
			key := strings.ToLower(l.Service)
			byService[key] = append(byService[key], ledger[i])
		}
	}
	for _, l := range leads {
		name := l.ID + " (" + l.Service + ")"
		i := hypothesisOf(ledger, l.ID)
		if problem := openProblem(name, l, ledger, i, transientAt); problem != "" {
			problems, open = append(problems, problem), append(open, l)
			continue
		}
		if problem := closedProblem(name, l, ledger[i], byService[strings.ToLower(l.Service)], f); problem != "" {
			problems = append(problems, problem)
		}
	}
	return problems, open
}

func hypothesisOf(ledger []revisions.Hypothesis, id string) int {
	return slices.IndexFunc(ledger, func(h revisions.Hypothesis) bool { return h.ID == id })
}

func openProblem(name string, l triage.Lead, ledger []revisions.Hypothesis, i int, transientAt map[string]int) string {
	switch {
	case i < 0:
		return name + " is missing from the ledger; send it again with the others"
	case ledger[i].Status == revisions.HypothesisOpen:
		return name + " is still open; next: " + cmp.Or(ledger[i].Next, l.Next)
	case ledger[i].Closing == "":
		return name + " is " + ledger[i].Status + " with no closing"
	case ledger[i].Closing == revisions.ClosingTransient && transientAt[l.ID] < minReadsForTransient:
		return name + " is closed as transient, but no read after the first one shows it gone; read it again and cite that later read"
	}
	return ""
}

func closedProblem(name string, l triage.Lead, h revisions.Hypothesis, sameService []revisions.Hypothesis, f concluded) string {
	counted := slices.ContainsFunc(sameService, func(o revisions.Hypothesis) bool {
		return o.Closing == revisions.ClosingCause || o.Closing == revisions.ClosingKnockOn
	})
	switch {
	case f.noIncident && (h.Closing == revisions.ClosingCause || h.Closing == revisions.ClosingKnockOn):
		return "no_incident is set, but " + name + " is closed as " + h.Closing
	case f.noIncident:
		return ""
	case h.Closing == revisions.ClosingCause && !f.blames(l.Service):
		return name + " is closed as cause, but causes does not name " + l.Service
	case h.Excluded() && !counted && f.blames(l.Service):
		return "causes names " + l.Service + ", which " + l.ID + " closed as " + h.Closing
	}
	return ""
}
