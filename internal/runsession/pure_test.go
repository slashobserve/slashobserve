package runsession

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestAnswerTextAppendsEachNoteAfterTheContent(t *testing.T) {
	a := Answer{Content: []byte(`{"ok":true}`), Notes: []string{"one", "two"}}
	if got, want := a.Text(), "{\"ok\":true}\n\n<note>one</note>\n\n<note>two</note>"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if got := (Answer{Content: []byte(`{}`)}).Text(); got != "{}" {
		t.Fatalf("text = %q", got)
	}
	a.Text()
	if string(a.Content) != `{"ok":true}` {
		t.Fatalf("Text changed the content: %s", a.Content)
	}
}

func TestMetaNamesTheSettledStateAndHidesTheBuildOfAnEndedRun(t *testing.T) {
	build := &tools.BuildProgress{Saves: 1, Panels: 1, PlannedPanels: 2}
	cases := []struct {
		name      string
		state     State
		restarted bool
		settled   SettledKind
		build     bool
	}{
		{"nothing settled", State{Build: build}, false, "", true},
		{"saved", State{Build: build, Settled: Settled{Kind: settledSaved}}, false, "saved", true},
		{"restarted", State{}, true, "restarted", false},
		{"ended", State{Build: build, Settled: Settled{Kind: settledEnded}}, false, "ended", false},
	}
	for _, tc := range cases {
		got := tc.state.meta(tc.restarted)
		gotSettled := SettledKind("")
		if got.Settled != nil {
			gotSettled = *got.Settled
		}
		if gotSettled != tc.settled || (got.Build != nil) != tc.build {
			t.Errorf("%s: meta = %+v", tc.name, got)
		}
	}
}

func TestNoteModelKeepsTheLargestOfEachCumulativeCount(t *testing.T) {
	var s State
	s.noteModel(revisions.Usage{Provider: "p", Model: "m", InputTokens: 100, ModelCalls: 3, CostUSD: floatPtr(0.5)})
	s.noteModel(revisions.Usage{InputTokens: 40, OutputTokens: 9, ModelCalls: 1, CostUSD: floatPtr(0.2)})
	s.noteModel(revisions.Usage{InputTokens: -5, CostUSD: floatPtr(-1)})
	got := s.Usage
	if got.Provider != "p" || got.Model != "m" || got.InputTokens != 100 || got.OutputTokens != 9 || got.ModelCalls != 3 || got.CostUSD == nil || *got.CostUSD != 0.5 {
		t.Fatalf("usage = %+v", got)
	}
}

func TestNoteModelKeepsTheLargestFailedCalls(t *testing.T) {
	var s State
	s.noteModel(revisions.Usage{FailedCalls: 2})
	s.noteModel(revisions.Usage{FailedCalls: 1})
	s.noteModel(revisions.Usage{FailedCalls: -4})
	if s.Usage.FailedCalls != 2 {
		t.Fatalf("failed calls = %d, want 2", s.Usage.FailedCalls)
	}
}

func TestStoredUsageKeepsARunWhoseOnlyModelCallFailed(t *testing.T) {
	var s State
	s.noteModel(revisions.Usage{Provider: "p", Model: "m", FailedCalls: 1})
	got := s.storedUsage(pricing.Table{})
	if got == nil || got.FailedCalls != 1 || got.ModelCalls != 0 {
		t.Fatalf("usage = %+v, want failed_calls 1 and model_calls 0", got)
	}
}

func TestStoredUsageIsNilForARunThatDidNothingAndCarriesTheSessionCounts(t *testing.T) {
	var s State
	if s.storedUsage(pricing.Table{}) != nil {
		t.Fatal("an idle run stored usage")
	}
	s.Counts.ToolCalls, s.Counts.Repairs, s.Counts.BuildSaves = 4, 1, 2
	got := s.storedUsage(pricing.Table{})
	if got == nil || got.ToolCalls != 4 || got.RepairRounds != 1 || got.BuildSaves != 2 {
		t.Fatalf("usage = %+v", got)
	}
}

func TestOverCostLimitNeedsALimitAndACost(t *testing.T) {
	cost := func(v float64) revisions.Usage { return revisions.Usage{CostUSD: &v} }
	cases := []struct {
		name  string
		limit float64
		usage revisions.Usage
		want  bool
	}{
		{"no limit", 0, cost(100), false},
		{"under", 1, cost(0.99), false},
		{"at the limit", 1, cost(1), true},
		{"over", 1, cost(2), true},
		{"no cost known", 1, revisions.Usage{InputTokens: 1 << 40}, false},
	}
	for _, tc := range cases {
		s := &Service{maxRunCost: tc.limit}
		if got := s.overCostLimit(tc.usage); got != tc.want {
			t.Errorf("%s: over = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSessionStateRoundTripsThroughJSON(t *testing.T) {
	s := State{Mode: tools.ModeRevise, ParentVersion: 2, Rejected: 1, Counts: Counts{ToolCalls: 3, Repairs: 1}}
	s.Settled = Settled{Kind: settledSaved, Version: 3}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(back)
	if string(again) != string(raw) {
		t.Fatalf("round trip changed the state:\n%s\n%s", raw, again)
	}
}

func TestSlotsBlockWhenFullAndFreeOnRelease(t *testing.T) {
	l := make(slots, 1)
	release, err := l.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := l.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second acquire = %v, want the context's deadline while the slot is held", err)
	}
	release()
	if _, err := l.acquire(context.Background()); err != nil {
		t.Fatalf("acquire after release = %v", err)
	}
}

func TestBriefTaskPointsAtTheAnchorOfTheRequest(t *testing.T) {
	pointed := &anchor.Anchor{Kind: anchor.Title, Pos: anchor.Pos{X: 0.1, Y: 0.1}}
	asked := briefTask(revisions.Request{Anchor: pointed, BlockID: "b1", SubjectObservationID: "o1", ObservationID: "own"}, tools.ModeInvestigate)
	if asked.Pointer != (brief.Pointer{Anchor: pointed, BlockID: "b1", ObservationID: "o1"}) {
		t.Fatalf("pointer = %+v", asked.Pointer)
	}
	own := briefTask(revisions.Request{Anchor: pointed, ObservationID: "own"}, tools.ModeInvestigate)
	if own.Pointer.ObservationID != "own" {
		t.Fatalf("pointer = %+v", own.Pointer)
	}
	if none := briefTask(revisions.Request{}, tools.ModeInvestigate); none.Pointer != (brief.Pointer{}) {
		t.Fatalf("pointer = %+v", none.Pointer)
	}
}

func TestBriefTaskMarksOnlyAThreadAskAsAskedOnItsObservation(t *testing.T) {
	chat := revisions.Request{SubjectObservationID: "o1"}
	if got := briefTask(chat, tools.ModeInvestigate).Pointer; got.ThreadAsk || got.ObservationID != "o1" {
		t.Fatalf("chat turn pointer = %+v", got)
	}
	thread := revisions.Request{SubjectObservationID: "o1", ThreadAsk: true}
	if got := briefTask(thread, tools.ModeInvestigate).Pointer; !got.ThreadAsk || got.ObservationID != "o1" {
		t.Fatalf("thread ask pointer = %+v", got)
	}
}

func TestCreatedIDsAreKeptOnceAndWithinTheirCap(t *testing.T) {
	var s State
	s.noteCreated([]string{"a", "", "b", "a"})
	if len(s.Created) != 2 || s.Created[0] != "a" || s.Created[1] != "b" {
		t.Fatalf("created = %v", s.Created)
	}
	for i := range tools.MaxCreatedIDs * 2 {
		s.noteCreated([]string{strconv.Itoa(i)})
	}
	if len(s.Created) != tools.MaxCreatedIDs {
		t.Fatalf("created = %d ids", len(s.Created))
	}
}

func TestWriteScopeIsACopyOfTheStoredState(t *testing.T) {
	s := State{Created: []string{"a"}, Committed: "d"}
	scope := s.writeScope()
	scope.Created[0] = "changed"
	if s.Created[0] != "a" || scope.Committed != "d" {
		t.Fatalf("state = %+v scope = %+v", s, scope)
	}
}

func TestACommitRecordsTheDashboardTheRunMayStillWriteTo(t *testing.T) {
	sess := &session{}
	sess.settleByCommit(dashboards.Commit{Dashboard: dashboards.Dashboard{ID: "d1"}, Version: dashboards.Version{Number: 3}})
	if sess.state.Settled != (Settled{Kind: settledSaved, Version: 3}) || sess.state.Committed != "d1" {
		t.Fatalf("state = %+v", sess.state)
	}
	proposed := &session{}
	proposed.settleByCommit(dashboards.Commit{Dashboard: dashboards.Dashboard{ID: "d2"}, Proposal: &dashboards.ProposalRef{}})
	if proposed.state.Settled.Kind != settledProposed || proposed.state.Committed != "" {
		t.Fatalf("proposed = %+v", proposed.state)
	}
}

func TestCreatedAndCommittedSurviveAStoredStateRoundTrip(t *testing.T) {
	raw, err := json.Marshal(State{Created: []string{"a"}, Committed: "d"})
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil || back.Committed != "d" || len(back.Created) != 1 {
		t.Fatalf("back = %+v, err = %v", back, err)
	}
}

func TestAWriteThatCreatedObjectsLetsTheNextCallWriteOnThem(t *testing.T) {
	sess := &session{}
	done := dispatched{res: tools.Result{Created: []string{"obs", "blk"}}, content: []byte(`{}`)}
	if _, err := sess.settleOutcome(context.Background(), "create_observation", done); err != nil {
		t.Fatal(err)
	}
	if got := sess.state.writeScope().Created; len(got) != 2 || got[0] != "obs" || got[1] != "blk" {
		t.Fatalf("created = %v", got)
	}
	failed := dispatched{res: tools.Result{IsError: true, Created: []string{"nope"}}, content: []byte(`{}`)}
	if _, err := sess.settleOutcome(context.Background(), "create_observation", failed); err != nil {
		t.Fatal(err)
	}
	if len(sess.state.Created) != 2 {
		t.Fatalf("a failed write recorded ids: %v", sess.state.Created)
	}
}

func TestOnlyASuccessfulWriteMarksTheRunAsHavingWritten(t *testing.T) {
	sess := &session{s: &Service{}}
	failed := dispatched{res: tools.Result{IsError: true}, content: []byte(`{}`)}
	if _, err := sess.settleOutcome(context.Background(), "create_observation", failed); err != nil {
		t.Fatal(err)
	}
	if sess.state.Wrote || sess.endingOf("", tools.RunFinish{Outcome: outcomeDone}).Wrote {
		t.Fatal("a failed write marked the run as having written")
	}
	done := dispatched{res: tools.Result{Created: []string{"obs"}}, content: []byte(`{}`)}
	if _, err := sess.settleOutcome(context.Background(), "create_observation", done); err != nil {
		t.Fatal(err)
	}
	if !sess.endingOf("", tools.RunFinish{Outcome: outcomeDone}).Wrote {
		t.Fatal("the ending of a run that wrote does not say so")
	}
	var back State
	raw, err := json.Marshal(sess.state)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &back); err != nil || !back.Wrote {
		t.Fatalf("wrote did not survive a stored state round trip: %v %s", err, raw)
	}
}

func TestAnUnchangedLedgerIsNotAChange(t *testing.T) {
	var s State
	ledger := []revisions.Hypothesis{{ID: "a", Text: "t", Status: revisions.HypothesisOpen}}
	if !s.noteLedger(ledger) {
		t.Fatal("the first ledger is not a change")
	}
	if s.noteLedger([]revisions.Hypothesis{{ID: "a", Text: "t", Status: revisions.HypothesisOpen}}) {
		t.Fatal("an equal ledger is a change")
	}
	if !s.noteLedger([]revisions.Hypothesis{{ID: "a", Text: "t", Status: revisions.HypothesisSupport, Evidence: []string{"e"}}}) {
		t.Fatal("a new status is not a change")
	}
}
