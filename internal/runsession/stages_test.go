package runsession

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func hypothesesJSON(resolved int, open ...string) string {
	var items []string
	for i := range resolved {
		id := "r" + strconv.Itoa(i)
		items = append(items, `{"id":"`+id+`","text":"cause `+id+`","status":"ruled_out","evidence":["read `+id+` showed nothing"]}`)
	}
	for _, id := range open {
		items = append(items, `{"id":"`+id+`","text":"cause `+id+`","status":"open","next_check":"read it"}`)
	}
	return `{"hypotheses":[` + strings.Join(items, ",") + `]}`
}

func (s *script) call(name, input string, turn *Turn, res func() tools.Result) Answer {
	s.t.Helper()
	ctx := context.Background()
	call := guard.Call{Name: name, Input: json.RawMessage(input)}
	admitted, err := s.sess.admit(ctx, call, turn)
	if err != nil {
		s.t.Fatalf("admit %s: %v", name, err)
	}
	if admitted.answer != nil {
		return *admitted.answer
	}
	result := res()
	content, err := json.Marshal(result.Content)
	if err != nil {
		s.t.Fatal(err)
	}
	answer, err := s.sess.settle(ctx, call, admitted, dispatched{res: result, content: content})
	if err != nil {
		s.t.Fatalf("settle %s: %v", name, err)
	}
	return answer
}

func (s *script) noted(input string, turn *Turn) Answer {
	s.t.Helper()
	return s.call("note_hypotheses", input, turn, func() tools.Result {
		res, err := s.reg.Call(s.ctx(), "note_hypotheses", json.RawMessage(input))
		if err != nil || res.IsError {
			s.t.Fatalf("note_hypotheses: %+v, %v", res, err)
		}
		return res
	})
}

func (s *script) ctx() context.Context {
	return tools.WithRun(auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer}), tools.Run{Mode: tools.ModeInvestigate, RequestID: "run"})
}

func (s *script) readCall(n int, turn *Turn) Answer {
	s.t.Helper()
	return s.call("list_metrics", `{"q":"`+strconv.Itoa(n)+`"}`, turn, func() tools.Result {
		return tools.Result{Content: map[string]any{"ok": true}, Step: "Listed 1 metrics on prom"}
	})
}

func stageNotes(a Answer) []string {
	var out []string
	for _, n := range a.Notes {
		if strings.HasPrefix(n, "Stage: ") {
			out = append(out, n)
		}
	}
	return out
}

func stageSteps(s *script) []string {
	var out []string
	for _, step := range s.sess.steps {
		if step.Phase == guard.PhaseStage {
			out = append(out, step.Message)
		}
	}
	return out
}

func newStagesScript(t *testing.T, callsMax int) *script {
	t.Helper()
	s := newScript(t, true)
	s.sess.s.investigationMax = callsMax
	s.sess.state.Guard.StartLoop(callsMax)
	return s
}

func TestARunThatKeepsProvingReachesTheCeilingAndEndsWithAFinding(t *testing.T) {
	s := newStagesScript(t, 18)
	turn := func(n int) *Turn { return &Turn{ID: "t" + strconv.Itoa(n), Calls: 2} }

	s.noted(hypothesesJSON(0, "h1", "h2", "h3"), turn(1))
	s.readCall(1, turn(1))
	for n := 2; n <= 4; n++ {
		s.noted(hypothesesJSON(n-1, "h1", "h2", "h3"), turn(n))
		s.readCall(n, turn(n))
	}
	if limit := s.sess.state.Guard.Loop.Limit; limit != 18 {
		t.Fatalf("limit = %d, want the ceiling 18", limit)
	}

	n := 5
	for ; s.sess.state.Guard.Window.Calls < 17; n++ {
		a := s.call("list_metrics", `{"q":"`+strconv.Itoa(n)+`"}`, &Turn{ID: "t" + strconv.Itoa(n), Calls: 1}, func() tools.Result {
			return tools.Result{Content: map[string]any{"ok": true}}
		})
		if a.IsError {
			t.Fatalf("read at call %d was refused: %s", s.sess.state.Guard.Window.Calls, a.Content)
		}
	}

	refused := s.call("list_metrics", `{"q":"late"}`, &Turn{ID: "late", Calls: 1}, func() tools.Result { return tools.Result{} })
	if !refused.IsError || !strings.Contains(string(refused.Content), "18 model calls") {
		t.Fatalf("a read past the ceiling = %s", refused.Content)
	}

	saved := s.call("save_finding", `{"title":"t","markdown":"m"}`, &Turn{ID: "save", Calls: 1, Saves: 1}, func() tools.Result {
		return tools.Result{Content: map[string]any{"finding_id": "f1"}}
	})
	if saved.IsError || s.sess.state.Settled.Kind != settledFinding {
		t.Fatalf("finding: %+v, settled %+v", saved, s.sess.state.Settled)
	}

	usage := s.sess.state.usage()
	if usage.CallLimit != 18 || usage.StageAtSave != guard.StageConclude {
		t.Fatalf("usage limit %d, stage at save %q", usage.CallLimit, usage.StageAtSave)
	}
}

func TestTheStageIsInEachTurnsFeedbackAndOnTheStepsWhenItChanges(t *testing.T) {
	s := newStagesScript(t, 30)

	a := s.readCall(1, &Turn{ID: "t1", Calls: 1})
	if got := stageNotes(a); len(got) != 1 || got[0] != "Stage: orient. Model call 1 of 12." {
		t.Fatalf("first turn notes = %q", a.Notes)
	}

	s.noted(hypothesesJSON(0, "h1", "h2"), &Turn{ID: "t2", Calls: 2})
	a = s.readCall(2, &Turn{ID: "t2", Calls: 2})
	if got := stageNotes(a); len(got) != 1 || got[0] != "Stage: hypothesize. Model call 2 of 12." {
		t.Fatalf("second turn notes = %q", a.Notes)
	}

	s.noted(hypothesesJSON(1, "h1"), &Turn{ID: "t3", Calls: 2})
	s.readCall(3, &Turn{ID: "t3", Calls: 2})
	s.readCall(4, &Turn{ID: "t4", Calls: 1})
	s.readCall(5, &Turn{ID: "t5", Calls: 1})

	want := []string{"Stage: orient", "Stage: hypothesize", "Stage: test"}
	if got := stageSteps(s); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("stage steps = %q, want %q", got, want)
	}
}

func TestAnEarlyConcludeMessageReachesTheAgentAtOnce(t *testing.T) {
	s := newStagesScript(t, 30)
	s.noted(hypothesesJSON(0, "h1", "h2"), &Turn{ID: "t1", Calls: 2})
	s.readCall(1, &Turn{ID: "t1", Calls: 2})
	concluded := `{"hypotheses":[{"id":"h1","text":"cause h1","status":"supported","evidence":["read showed it"]},{"id":"h2","text":"cause h2","status":"ruled_out","evidence":["read ruled it out"]}]}`
	s.noted(concluded, &Turn{ID: "t2", Calls: 2})
	a := s.readCall(2, &Turn{ID: "t2", Calls: 2})
	var found bool
	for _, n := range a.Notes {
		found = found || strings.Contains(n, "Call save_finding now")
	}
	if !found {
		t.Fatalf("notes = %q", a.Notes)
	}
}

func TestWithTheLoopOffNoStageReachesTheAgentOrTheSteps(t *testing.T) {
	off := newScript(t, false)
	for n := 1; n <= 3; n++ {
		a := off.readCall(n, &Turn{ID: "t" + strconv.Itoa(n), Calls: 1})
		for _, note := range a.Notes {
			if strings.HasPrefix(note, "Stage:") {
				t.Fatalf("a stage line with the loop off: %q", note)
			}
		}
	}
	if len(stageSteps(off)) != 0 || off.sess.state.Guard.Loop != nil {
		t.Fatalf("loop state with the loop off: %+v, steps %q", off.sess.state.Guard.Loop, stageSteps(off))
	}
	usage := off.sess.state.usage()
	if usage.CallLimit != 0 || usage.StageAtSave != "" {
		t.Fatalf("usage with the loop off: %+v", usage)
	}
	raw, err := json.Marshal(off.sess.state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"loop"`) {
		t.Fatalf("stored state with the loop off carries a loop: %s", raw)
	}
}

func TestTheLoopStateSurvivesAStoredStateRoundTrip(t *testing.T) {
	s := newStagesScript(t, 18)
	s.noted(hypothesesJSON(0, "h1"), &Turn{ID: "t1", Calls: 2})
	s.readCall(1, &Turn{ID: "t1", Calls: 2})
	s.noted(hypothesesJSON(1, "h1"), &Turn{ID: "t2", Calls: 2})
	s.readCall(2, &Turn{ID: "t2", Calls: 2})
	raw, err := json.Marshal(s.sess.state)
	if err != nil {
		t.Fatal(err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Guard.Loop == nil || back.Guard.Loop.Limit != 15 || back.Guard.Loop.Max != 18 {
		t.Fatalf("loop = %+v", back.Guard.Loop)
	}
	usage := back.usage()
	if usage.CallLimit != 15 {
		t.Fatalf("usage limit = %d", usage.CallLimit)
	}
}

func TestTheStageLoopStartsOnlyWithANewSessionOfABuiltInInvestigationWithTheLoopOn(t *testing.T) {
	investigation := revisions.Request{ID: "run", Attempt: 1, Intent: revisions.IntentAsk, Prompt: "why", Investigation: true, Assignee: revisions.AssignBuiltin}
	connected := investigation
	connected.Assignee = revisions.AssignGrant
	ask := investigation
	ask.Investigation = false
	cases := []struct {
		name string
		on   bool
		req  revisions.Request
		want bool
	}{
		{"built-in with the loop on", true, investigation, true},
		{"built-in with the loop off", false, investigation, false},
		{"connected agent with the loop on", true, connected, false},
		{"plain ask with the loop on", true, ask, false},
	}
	for _, c := range cases {
		svc := &Service{investigationLoop: c.on, investigationCalls: 14, investigationMax: 30}
		state, err := svc.initial(context.Background(), c.req, pgtype.UUID{})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := state.Guard.Loop != nil; got != c.want {
			t.Errorf("%s: guard loop = %t, want %t", c.name, got, c.want)
		}
		if c.want && (state.Guard.Loop.Limit != 14 || state.Guard.Loop.Max != 30) {
			t.Errorf("%s: loop = %+v", c.name, state.Guard.Loop)
		}
	}
}
