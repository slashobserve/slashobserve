package runsession

import (
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/revisions"
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
