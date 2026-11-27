package runsession

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestReportedGuardCountsMergeWithTheServersOwnAsTheMax(t *testing.T) {
	s := State{Guard: guard.New(tools.ModeInvestigate)}
	s.Guard.Counts.Nudges, s.Guard.Counts.Repairs = 3, 1
	ledger := []revisions.Hypothesis{{ID: "c1", Text: "the pool ran out", Status: revisions.HypothesisOpen}}
	s.noteModel(revisions.Usage{Nudges: 1, SaveNowRefusals: 2, StalledTurns: 1, OnePanelRefusals: 4, RepairRounds: 2, Hypotheses: ledger, CallLimit: 34, StageAtSave: guard.StageConclude})
	s.noteModel(revisions.Usage{SaveNowRefusals: 1, CallLimit: 20})
	got := s.usage()
	if got.Nudges != 3 || got.RepairRounds != 2 || got.SaveNowRefusals != 2 || got.StalledTurns != 1 || got.OnePanelRefusals != 4 {
		t.Fatalf("counts = %+v", got)
	}
	if len(got.Hypotheses) != 1 || got.CallLimit != 34 || got.StageAtSave != guard.StageConclude {
		t.Fatalf("ledger %+v, limit %d, stage %q", got.Hypotheses, got.CallLimit, got.StageAtSave)
	}

	s.Ledger = []revisions.Hypothesis{{ID: "own", Text: "the server's ledger", Status: revisions.HypothesisOpen}}
	s.Guard.StartLoop(40)
	s.Guard.Loop.SavedStage = guard.StageTest
	got = s.usage()
	if got.Hypotheses[0].ID != "own" || got.CallLimit != 34 || got.StageAtSave != guard.StageTest {
		t.Fatalf("with the server's own: ledger %+v, limit %d, stage %q", got.Hypotheses, got.CallLimit, got.StageAtSave)
	}
}

func TestCleanUsageDropsReportedValuesOutsideTheLedgerRules(t *testing.T) {
	long := make([]revisions.Hypothesis, tools.MaxHypotheses+1)
	for i := range long {
		long[i] = revisions.Hypothesis{ID: "h", Text: "t", Status: revisions.HypothesisOpen}
	}
	big := []revisions.Hypothesis{{ID: "h", Text: strings.Repeat("x", tools.MaxLedgerBytes), Status: revisions.HypothesisOpen}}
	for name, u := range map[string]revisions.Usage{
		"too many hypotheses": {Hypotheses: long},
		"too large a ledger":  {Hypotheses: big},
		"an unknown stage":    {StageAtSave: "wander"},
		"a negative count":    {Nudges: -3, CallLimit: -1},
	} {
		got := cleanUsage(u)
		if got.Hypotheses != nil || got.StageAtSave != "" || got.Nudges != 0 || got.CallLimit != 0 {
			t.Errorf("%s: cleaned = %+v", name, got)
		}
	}
	if got := cleanUsage(revisions.Usage{CallLimit: 1 << 20}); got.CallLimit != maxReportedCallLimit {
		t.Fatalf("call limit = %d", got.CallLimit)
	}
}
