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

func (s *State) groundingBehind(c Call) string {
	if s.Grounding == nil || s.Grounding.Asked || c.Ungrounded == "" {
		return ""
	}
	s.Grounding.Asked = true
	s.Turn.Rejected = true
	return c.Ungrounded
}
