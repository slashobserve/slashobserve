package runsession

import (
	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/findings"
)

const toolSaveFinding = "save_finding"

func groundsFindings(c guard.Call, res tools.Result) bool {
	return guard.ReadsData(c.Name) || res.Discovered != nil
}

type groundingCheck struct {
	unverified []findings.Unverified
	ledger     *grounding.Ledger
	signals    []findings.Signals
	scored     bool
}

func (s *State) checkGrounding(c *guard.Call, question string) groundingCheck {
	if s.Grounding == nil || c.Name != toolSaveFinding {
		return groundingCheck{}
	}
	problems := s.Grounding.Review(c.Input, question)
	if len(problems) > 0 && s.Grounding.Enforced() {
		c.Ungrounded = grounding.SendBackProblems(problems)
	}
	return groundingCheck{
		unverified: grounding.Unverified(problems),
		ledger:     s.Grounding.Ledger(),
		signals:    s.Grounding.CauseSignals(c.Input),
		scored:     true,
	}
}

func (s *State) noteGrounding(c guard.Call, d dispatched) {
	if s.Grounding == nil || d.res.IsError || !groundsFindings(c, d.res) {
		return
	}
	var names []string
	if d.res.Discovered != nil {
		names = d.res.Discovered.Names
	}
	s.Grounding.AddRead(d.content, names, rangesOf(d))
}

func (s *State) issueFacts(c guard.Call, d dispatched) []byte {
	if s.Grounding == nil || d.res.IsError {
		return d.content
	}
	return s.Grounding.IssueFacts(c.Name, c.Input, d.content, rangesOf(d))
}

func rangesOf(d dispatched) []grounding.Range {
	ranges := make([]grounding.Range, len(d.ranges))
	for i, r := range d.ranges {
		ranges[i] = grounding.Range{From: r.From, To: r.To}
	}
	return ranges
}
