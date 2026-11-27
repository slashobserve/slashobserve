package guard

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

func TestASecondStartLoopChangesNothing(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.StartLoop(30)
	s.Loop.Limit = 15
	s.StartLoop(40)
	if s.Loop.Limit != 15 || s.Loop.Max != 30 {
		t.Fatalf("a second start changed the loop: %+v", s.Loop)
	}
}
