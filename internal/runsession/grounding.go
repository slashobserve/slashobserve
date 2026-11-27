package runsession

import (
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/findings"
)

const (
	toolSaveFinding = "save_finding"
	toolCallMCPTool = "call_mcp_tool"
)

func answersAFigure(task route.Task) bool {
	return task == route.Measure || task == route.Logs || task == route.Traces
}

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
	sendBack := problems
	if missing, ok := grounding.MissingAnswers(c.Input, question); ok && answersAFigure(s.Task) {
		sendBack = append(slices.Clone(problems), missing)
	}
	if len(sendBack) > 0 && s.Grounding.Enforced() {
		c.Ungrounded = grounding.SendBackProblems(sendBack)
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
	if c.Name == toolCallMCPTool {
		s.Grounding.AddSourced(d.content)
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
