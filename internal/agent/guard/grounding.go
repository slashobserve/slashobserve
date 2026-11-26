package guard

import "github.com/slashobserve/slashobserve/internal/agent/tools"

type GroundingWatch struct {
	Asked bool `json:"asked,omitempty"`
}

func (s *State) WatchGrounding() {
	if s.Mode == tools.ModeInvestigate && s.Grounding == nil {
		s.Grounding = &GroundingWatch{}
	}
}

func (s *State) groundingBehind(c Call) (fault, bool) {
	if s.Grounding == nil || s.Grounding.Asked || c.Ungrounded == "" {
		return fault{}, false
	}
	s.Grounding.Asked = true
	s.Turn.Rejected = true
	return fault{c.Ungrounded, "it named a figure, service or time that the run never read"}, true
}
