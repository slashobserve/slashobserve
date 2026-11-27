package runsession

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var promGuide = []spec.DatasourceType{spec.Prometheus}

const (
	firstLedger  = `{"hypotheses":[{"id":"pool","text":"The checkout pool is exhausted","status":"open","next_check":"read pool_in_use"},{"id":"deploy","text":"The 10:00 deploy broke it","status":"open"}]}`
	secondLedger = `{"hypotheses":[{"id":"pool","text":"The checkout pool is exhausted","status":"supported","evidence":["pool_in_use 50 of 50 at 10:05"]},{"id":"deploy","text":"The 10:00 deploy broke it","status":"ruled_out","evidence":["no deploy annotation in the window"]}]}`
)

type script struct {
	t    *testing.T
	sess *session
	reg  *tools.Registry
}

func newScript(t *testing.T, loop bool) *script {
	t.Helper()
	state := State{Guard: guard.New(tools.ModeInvestigate), Loop: loop}
	if loop {
		state.Guard.WatchLedger()
	}
	return &script{
		t: t, reg: tools.New(tools.Deps{LedgerTool: true}),
		sess: &session{
			s:     &Service{},
			req:   revisions.Request{ID: "run", Attempt: 1, Prompt: "why is checkout slow", Investigation: true},
			state: state,
		},
	}
}

func (s *script) run(name, input string, res func() tools.Result, turn *Turn) {
	s.t.Helper()
	ctx := context.Background()
	call := guard.Call{Name: name, Input: json.RawMessage(input)}
	admitted, err := s.sess.admit(ctx, call, turn)
	if err != nil || admitted.answer != nil {
		s.t.Fatalf("admit %s: %+v, %v", name, admitted.answer, err)
	}
	result := res()
	content, err := json.Marshal(result.Content)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.sess.settle(ctx, call, admitted, dispatched{res: result, content: content}); err != nil {
		s.t.Fatalf("settle %s: %v", name, err)
	}
}

func (s *script) note(input string, turn *Turn) {
	s.t.Helper()
	s.run("note_hypotheses", input, func() tools.Result {
		ctx := tools.WithRun(auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer}), tools.Run{Mode: tools.ModeInvestigate, RequestID: "run"})
		res, err := s.reg.Call(ctx, "note_hypotheses", json.RawMessage(input))
		if err != nil || res.IsError {
			s.t.Fatalf("note_hypotheses: %+v, %v", res, err)
		}
		return res
	}, turn)
}

func (s *script) read(n int, turn *Turn) {
	s.t.Helper()
	s.run("list_metrics", `{"q":"`+strconv.Itoa(n)+`"}`, func() tools.Result {
		return tools.Result{Content: map[string]any{"ok": true}, Step: "Listed 1 metrics on prom", Discovered: &tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"pool_in_use"}}}
	}, turn)
}

func (s *script) brief(reason brief.Reason) tools.BriefAnswer {
	s.t.Helper()
	answer, err := s.sess.brief(context.Background(), reason, []datasource.Status{{UID: "prom", Type: "prometheus", Healthy: true}})
	if err != nil {
		s.t.Fatal(err)
	}
	return answer
}

func (s *script) messages() []string {
	var out []string
	for _, step := range s.sess.steps {
		out = append(out, step.Phase+": "+step.Message)
	}
	return out
}

func TestAShortenedSessionGetsTheLedgerBackInItsNextBrief(t *testing.T) {
	s := newScript(t, true)
	s.note(firstLedger, &Turn{ID: "t1", Calls: 2})
	s.read(1, &Turn{ID: "t1", Calls: 2})
	s.note(secondLedger, &Turn{ID: "t2", Calls: 2})
	s.read(2, &Turn{ID: "t2", Calls: 2})
	s.note(secondLedger, &Turn{ID: "t3", Calls: 2})
	s.read(3, &Turn{ID: "t3", Calls: 2})

	wantSteps := []string{
		"ledger: 2 hypotheses: 0 supported, 0 ruled out, 2 open",
		"discover: Listed 1 metrics on prom",
		"ledger: 2 hypotheses: 1 supported, 1 ruled out, 0 open",
		"discover: Listed 1 metrics on prom",
		"ledger: Hypotheses unchanged",
		"discover: Listed 1 metrics on prom",
	}
	if got := s.messages(); !slices.Equal(got, wantSteps) {
		t.Fatalf("steps:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantSteps, "\n"))
	}

	next := s.brief(brief.NoFinding)
	if next.System != prompts.InvestigateFor(prompts.Variant{Ledger: true}, promGuide) {
		t.Error("the next brief lacks the ledger section of the prompt")
	}
	block := next.Message[strings.Index(next.Message, "<hypotheses>") : strings.Index(next.Message, "</hypotheses>")+len("</hypotheses>")]
	for _, want := range []string{"- pool [supported] The checkout pool is exhausted", "evidence: pool_in_use 50 of 50 at 10:05", "- deploy [ruled_out] The 10:00 deploy broke it", "evidence: no deploy annotation in the window"} {
		if !strings.Contains(block, want) {
			t.Errorf("the block lacks %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "open]") || strings.Contains(block, "read pool_in_use") {
		t.Errorf("the block carries the replaced ledger:\n%s", block)
	}
	if strings.Index(next.Message, "<discovered>") > strings.Index(next.Message, "<hypotheses>") {
		t.Error("the ledger block comes before the digest")
	}

	usage := s.sess.state.usage()
	if len(usage.Hypotheses) != 2 || usage.Hypotheses[0].Status != revisions.HypothesisSupport {
		t.Errorf("usage ledger = %+v", usage.Hypotheses)
	}
}

func TestTheLedgerSurvivesAStoredStateRoundTrip(t *testing.T) {
	s := newScript(t, true)
	s.note(secondLedger, &Turn{ID: "t1", Calls: 1})
	raw, err := json.Marshal(s.sess.state)
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Ledger) != 2 || back.Ledger[1].Status != revisions.HypothesisRuledOut || back.Ledger[1].Evidence[0] != "no deploy annotation in the window" {
		t.Fatalf("ledger = %+v", back.Ledger)
	}
	if len(back.usage().Hypotheses) != 2 {
		t.Fatal("the stored usage lacks the ledger")
	}
}

func TestWithTheLoopOffTheBriefIsTheOneARunWithNoLedgerGets(t *testing.T) {
	fresh := newScript(t, false)
	want := fresh.brief(brief.NoFinding)

	off := newScript(t, false)
	off.sess.state.Ledger = []revisions.Hypothesis{{ID: "pool", Text: "kept from a run made with the loop on", Status: revisions.HypothesisOpen}}
	got := off.brief(brief.NoFinding)

	if got.System != prompts.InvestigateFor(prompts.Variant{}, promGuide) || got != want {
		t.Fatalf("brief with the loop off differs:\n%+v\nwant\n%+v", got, want)
	}
	if strings.Contains(got.Message, "<hypotheses>") || strings.Contains(got.System, "note_hypotheses") {
		t.Fatal("the loop text reached a run with the loop off")
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

func TestOnlyAnInvestigationRunWithTheLoopOnStartsWithTheLoopState(t *testing.T) {
	ask := revisions.Request{ID: "run", Attempt: 1, Intent: revisions.IntentAsk, Prompt: "why is checkout slow"}
	investigation := ask
	investigation.Investigation = true
	cases := []struct {
		name string
		loop bool
		req  revisions.Request
		want bool
	}{
		{"an investigation with the loop on", true, investigation, true},
		{"a plain ask with the loop on", true, ask, false},
		{"an investigation with the loop off", false, investigation, false},
	}
	for _, c := range cases {
		state, err := (&Service{investigationLoop: c.loop}).initial(context.Background(), c.req, pgtype.UUID{})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if state.Loop != c.want {
			t.Errorf("%s: loop = %t, want %t", c.name, state.Loop, c.want)
		}
	}
}

func TestAPlainAskGetsTheBriefARunWithNoLedgerGets(t *testing.T) {
	plain := newScript(t, false)
	plain.sess.state.Ledger = []revisions.Hypothesis{{ID: "pool", Text: "noted by a tool call", Status: revisions.HypothesisOpen}}
	got := plain.brief(brief.NoFinding)
	if got.System != prompts.InvestigateFor(prompts.Variant{}, promGuide) || strings.Contains(got.Message, "<hypotheses>") {
		t.Fatal("the ledger reached a plain ask")
	}
}

func (s *script) save(input string, turn *Turn) Answer {
	s.t.Helper()
	ctx := context.Background()
	call := guard.Call{Name: "save_finding", Input: json.RawMessage(input)}
	admitted, err := s.sess.admit(ctx, call, turn)
	if err != nil {
		s.t.Fatalf("admit save_finding: %v", err)
	}
	if admitted.answer == nil {
		s.t.Fatalf("the save was admitted: %+v", admitted.adm)
	}
	return *admitted.answer
}

func TestAFindingSavedOverAnUntouchedLedgerIsSentBackToUpdateIt(t *testing.T) {
	s := newScript(t, true)
	s.note(firstLedger, &Turn{ID: "t1", Calls: 2})
	s.read(1, &Turn{ID: "t1", Calls: 2})

	answer := s.save(`{"markdown":"Pool exhausted.","ruled_out":["deploy"]}`, &Turn{ID: "t2", Calls: 1})
	if !answer.IsError || !strings.Contains(string(answer.Content), "note_hypotheses") {
		t.Fatalf("answer = %+v %s", answer, answer.Content)
	}
	if got := s.sess.state.Guard.Counts.Nudges; got != 1 {
		t.Errorf("nudges = %d", got)
	}

	s.note(secondLedger, &Turn{ID: "t3", Calls: 2})
	s.read(2, &Turn{ID: "t3", Calls: 2})
	if !s.sess.state.Guard.Ledger.Resolved {
		t.Fatal("the guard did not see the updated ledger")
	}
}
