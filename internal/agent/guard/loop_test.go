package guard

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func hyp(id, status string, evidence ...string) revisions.Hypothesis {
	h := revisions.Hypothesis{ID: id, Text: "text of " + id, Status: status, Evidence: evidence}
	if status == revisions.HypothesisOpen {
		h.Next = "read something"
	}
	return h
}

func open(id string) revisions.Hypothesis { return hyp(id, revisions.HypothesisOpen) }

func ruledOut(id string, lines ...string) revisions.Hypothesis {
	return hyp(id, revisions.HypothesisRuledOut, lines...)
}

func supported(id string, lines ...string) revisions.Hypothesis {
	return hyp(id, revisions.HypothesisSupport, lines...)
}

type driver struct {
	t      *testing.T
	s      *State
	ledger []revisions.Hypothesis
	reads  int
}

func newDriver(t *testing.T, loop bool, calls, callsMax int) *driver {
	t.Helper()
	s := New(tools.ModeInvestigate)
	s.InvestigationCalls = calls
	if loop {
		s.StartLoop(callsMax)
	}
	return &driver{t: t, s: &s}
}

type outcome struct {
	nudge    string
	stop     *Stop
	refused  int
	stageNow string
	limit    int
}

func (d *driver) turn(ledger []revisions.Hypothesis, reads int) outcome {
	d.t.Helper()
	s := d.s
	s.BeginTurn(false)
	out := outcome{}
	position := 0
	if ledger != nil {
		call := jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`)
		if adm := s.Admit(position, call); adm.Verdict != Dispatch {
			d.t.Fatalf("ledger call = %+v", adm)
		}
		position++
		s.NoteLedger(d.ledger, ledger)
		d.ledger = ledger
	}
	for range reads {
		d.reads++
		call := jsonCall("list_metrics", `{"n":`+strconv.Itoa(d.reads)+`}`)
		adm := s.Admit(position, call)
		position++
		if adm.Verdict == Refuse {
			out.refused++
			continue
		}
		if _, err := s.Settle(call, tools.Result{}, []byte(`{}`)); err != nil {
			d.t.Fatal(err)
		}
		s.EndRead(call)
	}
	out.nudge, out.stop = s.EndTurn()
	out.stageNow, out.limit = s.Stage(), s.windowLimit()
	return out
}

func (d *driver) readTurns(n int) {
	d.t.Helper()
	for range n {
		if out := d.turn(nil, 1); out.refused > 0 || out.stop != nil {
			d.t.Fatalf("read turn: %+v", out)
		}
	}
}

func TestWithNoLedgerTheLimitIsTheOneOfToday(t *testing.T) {
	on := newDriver(t, true, 12, 30)
	off := newDriver(t, false, 12, 30)
	for i := 1; i <= 13; i++ {
		got, want := on.turn(nil, 1), off.turn(nil, 1)
		if got.nudge != want.nudge || got.refused != want.refused || got.limit != 12 || (got.stop == nil) != (want.stop == nil) {
			t.Fatalf("turn %d: with the loop %+v, without %+v", i, got, want)
		}
		if i == 11 && got.nudge != messageFindingNext {
			t.Fatalf("turn 11 nudge = %q", got.nudge)
		}
		if i >= 12 && got.refused != 1 {
			t.Fatalf("turn %d refused %d reads", i, got.refused)
		}
	}
}

func TestWithTheLoopOffAProvingLedgerChangesNothing(t *testing.T) {
	plain := newDriver(t, false, 12, 30)
	noted := newDriver(t, false, 12, 30)
	script := [][]revisions.Hypothesis{
		{open("a"), open("b")},
		{ruledOut("a", "e1"), open("b")},
		{ruledOut("a", "e1"), supported("b", "e2")},
	}
	for i := 1; i <= 13; i++ {
		var ledger []revisions.Hypothesis
		if i <= len(script) {
			ledger = script[i-1]
		}
		got, want := noted.turn(ledger, 1), plain.turn(nil, 1)
		if got.nudge != want.nudge || got.refused != want.refused || got.limit != 12 || got.stageNow != "" {
			t.Fatalf("turn %d: with a ledger %+v, without %+v", i, got, want)
		}
	}
	if noted.s.Stage() != "" || noted.s.StageLine() != "" || noted.s.Loop != nil {
		t.Fatalf("loop state with the loop off: %+v", noted.s.Loop)
	}
	if _, changed := noted.s.StageChange(); changed {
		t.Fatal("a stage change with the loop off")
	}
	if noted.s.Turn.Reads != 0 || noted.s.Turn.LedgerChanged {
		t.Fatalf("turn state with the loop off: %+v", noted.s.Turn)
	}
}

func TestProgressBackedByAReadGrowsTheLimit(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 1)
	if out := d.turn([]revisions.Hypothesis{ruledOut("a", "e1"), open("b")}, 1); out.limit != 15 {
		t.Fatalf("limit after progress = %d", out.limit)
	}
}

func TestTheLimitGrowsByAFixedStepUpToTheCeiling(t *testing.T) {
	d := newDriver(t, true, 12, 20)
	d.turn([]revisions.Hypothesis{open("a"), open("b"), open("c"), open("d")}, 1)
	steps := [][]revisions.Hypothesis{
		{ruledOut("a", "e1"), open("b"), open("c"), open("d")},
		{ruledOut("a", "e1"), ruledOut("b", "e2"), open("c"), open("d")},
		{ruledOut("a", "e1"), ruledOut("b", "e2"), ruledOut("c", "e3"), open("d")},
	}
	for i, want := range []int{15, 18, 20} {
		if out := d.turn(steps[i], 1); out.limit != want {
			t.Fatalf("step %d: limit = %d, want %d", i+1, out.limit, want)
		}
	}
}

func TestACeilingEqualToTheCallsNeverGrows(t *testing.T) {
	d := newDriver(t, true, 12, 12)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 1)
	if out := d.turn([]revisions.Hypothesis{ruledOut("a", "e1"), open("b")}, 1); out.limit != 12 {
		t.Fatalf("limit = %d", out.limit)
	}
}

func TestTwoProvingLedgersWithNoReadBetweenThemGrowTheLimitOnce(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{open("a"), open("b"), open("c")}, 1)
	first := []revisions.Hypothesis{ruledOut("a", "e1"), open("b"), open("c")}
	second := []revisions.Hypothesis{ruledOut("a", "e1"), ruledOut("b", "e2"), open("c")}
	d.s.NoteLedger(d.ledger, first)
	d.s.NoteLedger(first, second)
	if d.s.Loop.Limit != 15 {
		t.Fatalf("limit = %d", d.s.Loop.Limit)
	}
}

func TestEachProvingLedgerWithAReadSinceTheLastGrowthGrowsTheLimit(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{open("a"), open("b"), open("c")}, 1)
	d.turn([]revisions.Hypothesis{ruledOut("a", "e1"), open("b"), open("c")}, 1)
	if out := d.turn([]revisions.Hypothesis{ruledOut("a", "e1"), ruledOut("b", "e2"), open("c")}, 1); out.limit != 18 {
		t.Fatalf("limit = %d", out.limit)
	}
}

func TestALedgerWithNoReadAtAllEarnsNothing(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	if out := d.turn([]revisions.Hypothesis{ruledOut("a", "e1"), open("b")}, 0); out.limit != 12 {
		t.Fatalf("limit = %d", out.limit)
	}
}

func TestOnlyAMoveToSupportedOrRuledOutWithANewLineIsProgress(t *testing.T) {
	tests := []struct {
		name string
		prev []revisions.Hypothesis
		next []revisions.Hypothesis
		want bool
	}{
		{"open to ruled out", []revisions.Hypothesis{open("a")}, []revisions.Hypothesis{ruledOut("a", "e")}, true},
		{"open to supported", []revisions.Hypothesis{open("a")}, []revisions.Hypothesis{supported("a", "e")}, true},
		{"a new hypothesis already ruled out", []revisions.Hypothesis{open("a")}, []revisions.Hypothesis{open("a"), ruledOut("b", "e")}, true},
		{"a text edit", []revisions.Hypothesis{open("a")}, []revisions.Hypothesis{{ID: "a", Text: "other", Status: "open"}}, false},
		{"a new next check", []revisions.Hypothesis{open("a")}, []revisions.Hypothesis{{ID: "a", Text: "text of a", Status: "open", Next: "else"}}, false},
		{"evidence added to an open hypothesis", []revisions.Hypothesis{open("a")}, []revisions.Hypothesis{hyp("a", "open", "e")}, false},
		{"a ruled out hypothesis gets another line", []revisions.Hypothesis{ruledOut("a", "e")}, []revisions.Hypothesis{ruledOut("a", "e", "f")}, false},
		{"ruled out to supported on the same line", []revisions.Hypothesis{ruledOut("a", "e")}, []revisions.Hypothesis{supported("a", "e")}, false},
		{"ruled out to supported on a new line", []revisions.Hypothesis{ruledOut("a", "e")}, []revisions.Hypothesis{supported("a", "f")}, true},
		{"dropping a hypothesis", []revisions.Hypothesis{open("a"), open("b")}, []revisions.Hypothesis{open("a")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := provedSomething(tt.prev, tt.next); got != tt.want {
				t.Fatalf("provedSomething = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStagesFollowTheLedger(t *testing.T) {
	tests := []struct {
		name   string
		ledger []revisions.Hypothesis
		calls  int
		want   string
	}{
		{"no ledger", nil, 1, StageOrient},
		{"hypotheses with no evidence", []revisions.Hypothesis{open("a"), open("b")}, 1, StageHypothesize},
		{"one open with evidence", []revisions.Hypothesis{open("a"), hyp("b", "open", "e")}, 1, StageTest},
		{"one ruled out and one open", []revisions.Hypothesis{ruledOut("a", "e"), open("b")}, 1, StageTest},
		{"all ruled out", []revisions.Hypothesis{ruledOut("a", "e")}, 1, StageTest},
		{"supported and one still open", []revisions.Hypothesis{supported("a", "e"), open("b")}, 1, StageTest},
		{"supported and none open", []revisions.Hypothesis{supported("a", "e"), ruledOut("b", "f")}, 1, StageConclude},
		{"the budget is ending with no ledger", nil, 11, StageConclude},
		{"the budget is ending with hypotheses", []revisions.Hypothesis{open("a")}, 11, StageConclude},
		{"one call before the ending", []revisions.Hypothesis{open("a")}, 10, StageHypothesize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDriver(t, true, 12, 30)
			if tt.ledger != nil {
				d.s.NoteLedger(nil, tt.ledger)
			}
			d.s.Window.Calls = tt.calls
			if got := d.s.Stage(); got != tt.want {
				t.Fatalf("stage = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTheStageLineNamesTheStageAndTheCalls(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn(nil, 1)
	if got, want := d.s.StageLine(), "Stage: orient. Model call 1 of 12."; got != want {
		t.Fatalf("line = %q, want %q", got, want)
	}
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	if got, want := d.s.StageLine(), "Stage: hypothesize. Model call 2 of 12."; got != want {
		t.Fatalf("line = %q, want %q", got, want)
	}
}

func TestAStageChangeIsReportedOnce(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn(nil, 1)
	var seen []string
	note := func() {
		if stage, changed := d.s.StageChange(); changed {
			seen = append(seen, stage)
		}
	}
	note()
	note()
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	note()
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	note()
	d.turn([]revisions.Hypothesis{hyp("a", "open", "e")}, 1)
	note()
	if got := strings.Join(seen, ","); got != "orient,hypothesize,test" {
		t.Fatalf("stages = %s", got)
	}
}

func TestAConcludeStageSendsTheSaveMessageAtOnceAndOnlyOnce(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 1)
	out := d.turn([]revisions.Hypothesis{supported("a", "e1"), ruledOut("b", "e2")}, 1)
	if out.nudge != messageConclude || out.stageNow != StageConclude {
		t.Fatalf("outcome = %+v", out)
	}
	if out.limit != 15 {
		t.Fatalf("the same ledger earned growth: limit %d", out.limit)
	}
	if again := d.turn(nil, 1); again.nudge != "" {
		t.Fatalf("second turn nudge = %q", again.nudge)
	}
	if d.s.Counts.Nudges != 0 {
		t.Fatalf("the early message counted as a nudge: %d", d.s.Counts.Nudges)
	}
}

func TestAReopenedLedgerCanConcludeAgain(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{supported("a", "e1")}, 1)
	d.turn([]revisions.Hypothesis{supported("a", "e1"), open("b")}, 1)
	out := d.turn([]revisions.Hypothesis{supported("a", "e1"), ruledOut("b", "e2")}, 1)
	if out.nudge != messageConclude {
		t.Fatalf("nudge = %q", out.nudge)
	}
}

func TestTheLimitNudgeComesBeforeTheConcludeMessage(t *testing.T) {
	d := newDriver(t, true, 12, 12)
	d.readTurns(9)
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	out := d.turn([]revisions.Hypothesis{supported("a", "e1")}, 1)
	if out.nudge != messageFindingNext {
		t.Fatalf("nudge = %q", out.nudge)
	}
}

func TestTwoReadTurnsWithNoLedgerChangeGetOneReminder(t *testing.T) {
	d := newDriver(t, true, 20, 20)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 1)
	if out := d.turn(nil, 1); out.nudge != "" {
		t.Fatalf("first quiet turn nudge = %q", out.nudge)
	}
	if out := d.turn(nil, 1); out.nudge != messageReminder {
		t.Fatalf("second quiet turn nudge = %q", out.nudge)
	}
	for i := range 3 {
		if out := d.turn(nil, 1); out.nudge != "" || out.stop != nil {
			t.Fatalf("turn %d after the reminder: %+v", i+1, out)
		}
	}
}

func TestAnUnchangedLedgerDoesNotCountAsAChange(t *testing.T) {
	d := newDriver(t, true, 20, 20)
	ledger := []revisions.Hypothesis{open("a")}
	d.turn(ledger, 1)
	d.turn(ledger, 1)
	if out := d.turn(ledger, 1); out.nudge != messageReminder {
		t.Fatalf("nudge = %q", out.nudge)
	}
}

func TestAChangeEndsTheStreakAndAllowsALaterReminder(t *testing.T) {
	d := newDriver(t, true, 30, 30)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 1)
	d.turn(nil, 1)
	if out := d.turn([]revisions.Hypothesis{open("a"), hyp("b", "open", "e")}, 1); out.nudge != "" {
		t.Fatalf("a changed ledger got %q", out.nudge)
	}
	if out := d.turn(nil, 1); out.nudge != "" {
		t.Fatalf("first quiet turn after the change got %q", out.nudge)
	}
	if out := d.turn(nil, 1); out.nudge != messageReminder {
		t.Fatalf("second quiet turn after the change got %q", out.nudge)
	}
}

func TestNoReminderWithoutALedger(t *testing.T) {
	d := newDriver(t, true, 20, 20)
	for i := range 4 {
		if out := d.turn(nil, 1); out.nudge != "" {
			t.Fatalf("turn %d: %q", i+1, out.nudge)
		}
	}
}

func TestATurnWithNoReadDoesNotCountAsQuiet(t *testing.T) {
	d := newDriver(t, true, 20, 20)
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	d.turn(nil, 1)
	d.s.BeginTurn(false)
	d.s.Admit(0, jsonCall(toolNoteHypotheses, `{"hypotheses":[]}`))
	d.s.EndTurn()
	if out := d.turn(nil, 1); out.nudge != "" {
		t.Fatalf("nudge = %q", out.nudge)
	}
}

func TestTheReminderNeverRefusesAnything(t *testing.T) {
	d := newDriver(t, true, 20, 20)
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	d.turn(nil, 1)
	d.turn(nil, 1)
	out := d.turn(nil, 2)
	if out.refused != 0 || out.stop != nil {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestTurnsOfOnlyLedgerCallsStillStopTheRunWithTheLoopOn(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 0)
	out := d.turn([]revisions.Hypothesis{open("a"), open("c")}, 0)
	if out.stop == nil || out.stop.Failure != revisions.FailureNoProgress {
		t.Fatalf("stop = %+v", out.stop)
	}
}

func TestAnAnswerTurnAtTheGrownLimitStillRefusesReadsPastIt(t *testing.T) {
	d := newDriver(t, true, 12, 15)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 1)
	d.turn([]revisions.Hypothesis{ruledOut("a", "e1"), open("b")}, 1)
	d.readTurns(12)
	if d.s.Window.Calls != 14 {
		t.Fatalf("calls = %d", d.s.Window.Calls)
	}
	if out := d.turn(nil, 1); out.refused != 1 || out.limit != 15 {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestTheStageAtSaveAndTheFinalLimitAreKept(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{open("a"), open("b")}, 1)
	d.turn([]revisions.Hypothesis{supported("a", "e1"), ruledOut("b", "e2")}, 1)
	d.s.BeginTurn(true)
	call := jsonCall("save_finding", `{"title":"t","markdown":"m"}`)
	if adm := d.s.Admit(0, call); adm.Verdict != Dispatch {
		t.Fatalf("save = %+v", adm)
	}
	if _, err := d.s.Settle(call, tools.Result{}, []byte(`{"finding_id":"f1"}`)); err != nil {
		t.Fatal(err)
	}
	if d.s.Loop.SavedStage != StageConclude || d.s.Loop.Limit != 15 {
		t.Fatalf("loop = %+v", d.s.Loop)
	}
}

func TestAFailedReadEarnsNoGrowth(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.s.NoteLedger(nil, []revisions.Hypothesis{open("a")})
	d.s.BeginTurn(false)
	call := jsonCall("list_metrics", `{}`)
	d.s.Admit(0, call)
	if _, err := d.s.Settle(call, tools.Result{IsError: true}, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	d.s.EndTurn()
	d.s.BeginTurn(false)
	d.s.NoteLedger([]revisions.Hypothesis{open("a")}, []revisions.Hypothesis{ruledOut("a", "e")})
	if d.s.Loop.Limit != 12 {
		t.Fatalf("limit = %d", d.s.Loop.Limit)
	}
}

func TestStartLoopOnlyForInvestigations(t *testing.T) {
	for _, mode := range []tools.Mode{tools.ModeCreate, tools.ModeRevise, tools.ModeSuggest} {
		s := New(mode)
		s.StartLoop(30)
		if s.Loop != nil {
			t.Errorf("%s got a loop", mode)
		}
	}
	s := New(tools.ModeInvestigate)
	s.StartLoop(30)
	s.Loop.Limit = 15
	s.StartLoop(40)
	if s.Loop.Limit != 15 || s.Loop.Max != 30 {
		t.Fatalf("a second start changed the loop: %+v", s.Loop)
	}
}

func TestAnAskedCeilingBelowTheCallsIsRaisedToThem(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.InvestigationCalls = 20
	s.StartLoop(5)
	if s.Loop.Max != 20 || s.Loop.Limit != 20 {
		t.Fatalf("loop = %+v", s.Loop)
	}
}

func TestTheLimitMessagesNameTheGrownLimit(t *testing.T) {
	d := newDriver(t, true, 12, 30)
	d.turn([]revisions.Hypothesis{open("a")}, 1)
	d.turn([]revisions.Hypothesis{ruledOut("a", "e1")}, 1)
	if got := d.s.rule(guardFindingNow).message; !strings.Contains(got, fmt.Sprintf("%d model calls", 15)) {
		t.Fatalf("message = %q", got)
	}
}
