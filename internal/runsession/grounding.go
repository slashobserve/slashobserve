package runsession

import (
	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/findings"
)

const toolSaveFinding = "save_finding"

func groundsFindings(name string) bool {
	switch tools.Phase(name) {
	case tools.PhaseDiscover, tools.PhaseQuery:
		return true
	}
	return false
}

func (s *State) checkGrounding(c *guard.Call) []findings.Unverified {
	if s.Grounding == nil || c.Name != toolSaveFinding {
		return nil
	}
	items := s.Grounding.Check(c.Input)
	if len(items) > 0 && s.Grounding.Enforced() {
		c.Ungrounded = grounding.SendBack(items)
	}
	return items
}

func (s *State) noteGrounding(c guard.Call, d dispatched) {
	if s.Grounding == nil || d.res.IsError || !groundsFindings(c.Name) {
		return
	}
	var names []string
	if d.res.Discovered != nil {
		names = d.res.Discovered.Names
	}
	ranges := make([]grounding.Range, len(d.ranges))
	for i, r := range d.ranges {
		ranges[i] = grounding.Range{From: r.From, To: r.To}
	}
	s.Grounding.AddRead(d.content, names, ranges)
}
