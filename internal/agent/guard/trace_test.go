package guard

import (
	"bytes"
	"cmp"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

var traceFiles = []string{"admission", "windows", "rejections", "writes", "answers", "grounding", "ledger", "loop", "churn"}

type traceFile struct {
	Traces []trace `json:"traces"`
}

type trace struct {
	Name   string      `json:"name"`
	TSOnly string      `json:"ts_only"`
	GoOnly string      `json:"go_only"`
	Start  traceStart  `json:"start"`
	Steps  []traceStep `json:"steps"`
}

type traceStart struct {
	Mode          tools.Mode   `json:"mode"`
	Agent         traceAgent   `json:"agent"`
	Task          string       `json:"task"`
	Investigation *bool        `json:"investigation"`
	Building      bool         `json:"building"`
	ParentVersion int          `json:"parent_version"`
	WindowCalls   int          `json:"window_calls"`
	Streak        *traceStreak `json:"streak"`
}

type traceAgent struct {
	InvestigationCalls    int    `json:"investigation_calls"`
	InvestigationCallsMax int    `json:"investigation_calls_max"`
	ChatCalls             int    `json:"chat_calls"`
	Loop                  bool   `json:"loop"`
	Grounding             string `json:"grounding"`
}

type traceStreak struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type traceCall struct {
	Name       string          `json:"name"`
	Input      json.RawMessage `json:"input"`
	Raw        *string         `json:"raw"`
	Ungrounded string          `json:"ungrounded"`
	Unclosed   string          `json:"unclosed"`
}

func (c traceCall) call() Call {
	input := c.Input
	switch {
	case c.Raw != nil:
		input = json.RawMessage(*c.Raw)
	case len(input) == 0:
		input = json.RawMessage(`{}`)
	}
	return Call{Name: c.Name, Input: input, Ungrounded: c.Ungrounded, Unclosed: c.Unclosed}
}

type traceResult struct {
	IsError bool            `json:"is_error"`
	Content json.RawMessage `json:"content"`
	Settled *string         `json:"settled"`
	Call    struct {
		Edits *struct {
			Panels   []string `json:"panels"`
			Added    []string `json:"added"`
			Deleted  []string `json:"deleted"`
			Settings bool     `json:"settings"`
		} `json:"edits"`
		Refusal   string `json:"refusal"`
		Unchecked string `json:"unchecked"`
	} `json:"call"`
}

type traceAdmission struct {
	Verdict      *string            `json:"verdict"`
	Phase        *string            `json:"phase"`
	Step         *string            `json:"step"`
	Code         *string            `json:"code"`
	Message      *string            `json:"message"`
	MessageHas   json.RawMessage    `json:"message_has"`
	Note         *string            `json:"note"`
	ReadsRefused *int               `json:"reads_refused"`
	Unchecked    *[]tools.Unchecked `json:"unchecked"`
	ReuseOf      *int               `json:"reuse_of"`
}

type traceSettled struct {
	Note *string         `json:"note"`
	Stop json.RawMessage `json:"stop"`
}

type traceStop struct {
	Failure string `json:"failure"`
	Step    string `json:"step"`
	Message string `json:"message"`
}

type traceEnd struct {
	Nudge     *string         `json:"nudge"`
	StageLine *string         `json:"stage_line"`
	StageStep *string         `json:"stage_step"`
	Stop      json.RawMessage `json:"stop"`
}

type traceCheck struct {
	Limit  *int                       `json:"limit"`
	Stage  *string                    `json:"stage"`
	Writes *int                       `json:"writes"`
	Phase  map[string]string          `json:"phase"`
	Streak json.RawMessage            `json:"streak"`
	Usage  map[string]json.RawMessage `json:"usage"`
}

type traceStep struct {
	Turn      *struct{ Saves int }     `json:"turn"`
	Call      *traceCall               `json:"call"`
	Expect    *traceAdmission          `json:"expect"`
	Result    *traceResult             `json:"result"`
	Settled   *traceSettled            `json:"settled"`
	Settle    *int                     `json:"settle"`
	EndRead   *int                     `json:"end_read"`
	EndTurn   *traceEnd                `json:"end_turn"`
	Check     *traceCheck              `json:"check"`
	Set       *traceSet                `json:"set"`
	Session   *struct{ Building bool } `json:"session"`
	Restart   *struct{}                `json:"restart"`
	SeedLedge []revisions.Hypothesis   `json:"seed_ledger"`
	Forget    *traceCall               `json:"forget"`
}

type traceSet struct {
	WindowCalls *int  `json:"window_calls"`
	Saved       *bool `json:"saved"`
}

func readSpecExample(t *testing.T, name string) []byte {
	t.Helper()
	root, err := os.OpenRoot("../../../spec/examples/agent/guard")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	data, err := root.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGuardTraces(t *testing.T) {
	for _, file := range traceFiles {
		var f traceFile
		if err := json.Unmarshal(readSpecExample(t, file+".json"), &f); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, tr := range f.Traces {
			t.Run(file+"/"+tr.Name, func(t *testing.T) {
				if tr.TSOnly != "" {
					t.Skip("ts_only: " + tr.TSOnly)
				}
				runTrace(t, tr)
			})
		}
	}
}

func startState(t *testing.T, st traceStart) State {
	t.Helper()
	s := New(st.Mode)
	s.BeginSession(st.Building, st.ParentVersion)
	switch st.Mode {
	case tools.ModeInvestigate:
		s.InvestigationCalls = cmp.Or(st.Agent.InvestigationCalls, InvestigationCallsFor(route.Task(st.Task)))
	case tools.ModeCreate:
		s.InvestigationCalls = st.Agent.ChatCalls
	}
	if st.Agent.Loop && st.Mode == tools.ModeInvestigate && (st.Investigation == nil || *st.Investigation) {
		s.WatchLedger()
		s.StartLoop(cmp.Or(st.Agent.InvestigationCallsMax, DefaultInvestigationCallsMax))
	}
	if st.Agent.Grounding == "enforce" {
		s.WatchGrounding()
	}
	s.Window.Calls = st.WindowCalls
	if k := st.Streak; k != nil {
		switch k.Kind {
		case "generic":
			s.Rejections = k.Count
		case "length":
			s.LengthRejections = k.Count
		case "restricted":
			s.RestrictedRefusals = k.Count
		default:
			t.Fatalf("streak kind %q", k.Kind)
		}
	}
	return s
}

type traceRun struct {
	t      *testing.T
	s      State
	ledger []revisions.Hypothesis
	calls  []Call
	adms   []Admission
}

func runTrace(t *testing.T, tr trace) {
	r := &traceRun{t: t, s: startState(t, tr.Start)}
	for i, step := range tr.Steps {
		r.step(i, step)
	}
}

func (r *traceRun) step(i int, step traceStep) {
	t, s := r.t, &r.s
	switch {
	case step.Turn != nil:
		s.BeginTurn(step.Turn.Saves > 0)
		r.calls, r.adms = nil, nil
	case step.Call != nil:
		c := step.Call.call()
		adm := s.Admit(len(r.calls), c)
		r.calls, r.adms = append(r.calls, c), append(r.adms, adm)
		if step.Expect != nil {
			checkAdmission(t, i, adm, *step.Expect)
		}
		if step.Result != nil {
			r.settle(i, len(r.calls)-1, *step.Result, step.Settled)
		}
	case step.Settle != nil:
		r.settle(i, *step.Settle, *step.Result, step.Settled)
	case step.EndRead != nil:
		s.EndRead(r.calls[*step.EndRead])
	case step.EndTurn != nil:
		r.endTurn(i, *step.EndTurn)
	case step.Check != nil:
		r.check(i, *step.Check)
	case step.Set != nil:
		if step.Set.WindowCalls != nil {
			s.Window.Calls = *step.Set.WindowCalls
		}
		if step.Set.Saved != nil {
			s.Saved = *step.Set.Saved
		}
	case step.Session != nil:
		s.BeginSession(step.Session.Building, s.ParentVersion)
	case step.Restart != nil:
		s.Restart()
		s.BeginSession(false, s.ParentVersion)
	case step.SeedLedge != nil:
		r.ledger = step.SeedLedge
		s.SeedLedger(step.SeedLedge)
	case step.Forget != nil:
		c := step.Forget.call()
		switch tools.Phase(c.Name) {
		case tools.PhaseWrite:
			s.ForgetWrite(c)
		case tools.PhaseSave:
			s.ForgetSave(c)
		}
	default:
		t.Fatalf("step %d: no known operation", i)
	}
}

func (r *traceRun) settle(i, position int, res traceResult, want *traceSettled) {
	t, s := r.t, &r.s
	c, adm := r.calls[position], r.adms[position]
	if adm.Verdict != Refuse {
		s.EndRead(c)
	}
	if adm.Verdict != Dispatch {
		return
	}
	result := tools.Result{IsError: res.IsError, Refusal: tools.Refusal(res.Call.Refusal), Unchecked: res.Call.Unchecked}
	if e := res.Call.Edits; e != nil {
		result.Edits = tools.Edits{Panels: e.Panels, Added: e.Added, Deleted: e.Deleted, Settings: e.Settings}
	}
	if tools.Phase(c.Name) == tools.PhaseLedger && !res.IsError {
		next := ledgerOfInput(t, c.Input)
		result.Ledger = next
		s.NoteLedger(r.ledger, next)
		r.ledger = next
	}
	content := []byte(res.Content)
	if len(content) == 0 {
		content = []byte(`{"ok":true}`)
	}
	settled, err := s.Settle(c, result, content)
	if err != nil {
		if res.Settled != nil && *res.Settled == "ended" {
			return
		}
		t.Fatalf("step %d: settle: %v", i, err)
	}
	if want == nil {
		return
	}
	if want.Note != nil && settled.Note != *want.Note {
		t.Errorf("step %d: note = %q, want %q", i, settled.Note, *want.Note)
	}
	checkStop(t, i, settled.Stop, want.Stop)
}

func ledgerOfInput(t *testing.T, input json.RawMessage) []revisions.Hypothesis {
	t.Helper()
	var in struct {
		Hypotheses []struct {
			ID        string   `json:"id"`
			Text      string   `json:"text"`
			Status    string   `json:"status"`
			Closing   string   `json:"closing"`
			KnockOnOf string   `json:"knock_on_of"`
			Evidence  []string `json:"evidence"`
			Next      string   `json:"next_check"`
		} `json:"hypotheses"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		t.Fatal(err)
	}
	var out []revisions.Hypothesis
	for _, h := range in.Hypotheses {
		item := revisions.Hypothesis{ID: h.ID, Text: h.Text, Status: h.Status, Closing: h.Closing, KnockOnOf: h.KnockOnOf, Evidence: h.Evidence}
		if h.Status == revisions.HypothesisOpen {
			item.Next = h.Next
		}
		out = append(out, item)
	}
	return out
}

var verdictNames = map[Verdict]string{Dispatch: "dispatch", Replay: "replay", Reuse: "reuse", Refuse: "refuse"}

func checkAdmission(t *testing.T, i int, adm Admission, want traceAdmission) {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if len(adm.Content) > 0 {
		if err := json.Unmarshal(adm.Content, &body); err != nil {
			t.Fatalf("step %d: content %s: %v", i, adm.Content, err)
		}
	}
	same := func(field, got string, want *string) {
		if want != nil && got != *want {
			t.Errorf("step %d: %s = %q, want %q", i, field, got, *want)
		}
	}
	same("verdict", verdictNames[adm.Verdict], want.Verdict)
	same("phase", adm.Phase, want.Phase)
	same("step", adm.Step, want.Step)
	same("code", body.Code, want.Code)
	same("message", body.Message, want.Message)
	same("note", adm.Note, want.Note)
	for _, part := range textsOf(t, want.MessageHas) {
		if !strings.Contains(body.Message, part) {
			t.Errorf("step %d: message %q does not contain %q", i, body.Message, part)
		}
	}
	if want.ReadsRefused != nil && adm.ReadsRefused != *want.ReadsRefused {
		t.Errorf("step %d: reads refused = %d, want %d", i, adm.ReadsRefused, *want.ReadsRefused)
	}
	if want.Unchecked != nil && !slices.Equal(adm.Unchecked, *want.Unchecked) {
		t.Errorf("step %d: unchecked = %+v, want %+v", i, adm.Unchecked, *want.Unchecked)
	}
	if want.ReuseOf != nil && adm.ReuseOf != *want.ReuseOf {
		t.Errorf("step %d: reuse of = %d, want %d", i, adm.ReuseOf, *want.ReuseOf)
	}
}

func textsOf(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		t.Fatal(err)
	}
	return many
}

func checkStop(t *testing.T, i int, got *Stop, raw json.RawMessage) {
	t.Helper()
	if len(raw) == 0 {
		return
	}
	if bytes.Equal(raw, []byte("null")) {
		if got != nil {
			t.Errorf("step %d: stop = %+v, want none", i, got)
		}
		return
	}
	var want traceStop
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Errorf("step %d: no stop, want %+v", i, want)
		return
	}
	if have := (traceStop{Failure: string(got.Failure), Step: got.Step, Message: got.Message}); have != want {
		t.Errorf("step %d: stop = %+v, want %+v", i, have, want)
	}
}

func (r *traceRun) endTurn(i int, want traceEnd) {
	t, s := r.t, &r.s
	nudge, stop := s.EndTurn()
	line, step := "", ""
	if stop == nil {
		if line = s.StageLine(); line != "" {
			if stage, changed := s.StageChange(); changed {
				step = "Stage: " + stage
			}
		}
	}
	for _, c := range []struct {
		field string
		got   string
		want  *string
	}{{"nudge", nudge, want.Nudge}, {"stage line", line, want.StageLine}, {"stage step", step, want.StageStep}} {
		if c.want != nil && c.got != *c.want {
			t.Errorf("step %d: %s = %q, want %q", i, c.field, c.got, *c.want)
		}
	}
	checkStop(t, i, stop, want.Stop)
}

func (r *traceRun) streak() any {
	s := &r.s
	kinds := map[string]int{"generic": s.Rejections, "length": s.LengthRejections, "restricted": s.RestrictedRefusals}
	var streak any
	for kind, n := range kinds {
		if n == 0 {
			continue
		}
		if streak != nil {
			r.t.Fatalf("two rejection runs at once: %+v", kinds)
		}
		streak = map[string]any{"kind": kind, "count": float64(n)}
	}
	return streak
}

func (r *traceRun) check(i int, want traceCheck) {
	t, s := r.t, &r.s
	if want.Limit != nil && s.windowLimit() != *want.Limit {
		t.Errorf("step %d: limit = %d, want %d", i, s.windowLimit(), *want.Limit)
	}
	if want.Stage != nil && s.Stage() != *want.Stage {
		t.Errorf("step %d: stage = %q, want %q", i, s.Stage(), *want.Stage)
	}
	if want.Writes != nil && s.Writes != *want.Writes {
		t.Errorf("step %d: writes = %d, want %d", i, s.Writes, *want.Writes)
	}
	for name, phase := range want.Phase {
		if got := s.Phase(name); got != phase {
			t.Errorf("step %d: phase of %s = %q, want %q", i, name, got, phase)
		}
	}
	if len(want.Streak) > 0 {
		var streak any
		if err := json.Unmarshal(want.Streak, &streak); err != nil {
			t.Fatal(err)
		}
		if got := r.streak(); !reflect.DeepEqual(got, streak) {
			t.Errorf("step %d: streak = %v, want %v", i, got, streak)
		}
	}
	usage := map[string]any{
		"nudges":             float64(s.Counts.Nudges),
		"save_now_refusals":  float64(s.Counts.SaveNowRefusals),
		"stalled_turns":      float64(s.Counts.StalledTurns),
		"one_panel_refusals": float64(s.Counts.OnePanelRefusals),
		"repair_rounds":      float64(s.Counts.Repairs),
		"call_limit":         nil,
		"stage_at_save":      nil,
	}
	if s.Loop != nil {
		usage["call_limit"] = float64(s.Loop.Limit)
		if s.Loop.SavedStage != "" {
			usage["stage_at_save"] = s.Loop.SavedStage
		}
	}
	for name, raw := range want.Usage {
		got, known := usage[name]
		if !known {
			t.Fatalf("step %d: unknown usage field %q", i, name)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, value) {
			t.Errorf("step %d: usage %s = %v, want %v", i, name, got, value)
		}
	}
}

type guardCases struct {
	WriteTools []string `json:"write_tools"`
	Keys       []struct {
		Name          string `json:"name"`
		Tool          string `json:"tool"`
		OtherTool     string `json:"other_tool"`
		A             string `json:"a"`
		B             string `json:"b"`
		Same          bool   `json:"same"`
		GoReadsDiffer bool   `json:"go_reads_differ"`
	} `json:"keys"`
	CountClaims struct {
		Match   []string `json:"match"`
		NoMatch []string `json:"no_match"`
	} `json:"count_claims"`
	TitleCauses []struct {
		Name  string `json:"name"`
		Title string `json:"title"`
		Text  string `json:"text"`
		Want  bool   `json:"want"`
	} `json:"title_causes"`
	Conclusions []struct {
		Input json.RawMessage `json:"input"`
		Want  bool            `json:"want"`
	} `json:"conclusions"`
	Proved []struct {
		Name string                 `json:"name"`
		Prev []revisions.Hypothesis `json:"prev"`
		Next []revisions.Hypothesis `json:"next"`
		Want bool                   `json:"want"`
	} `json:"proved"`
	PeopleText []struct {
		Reasons []string `json:"reasons"`
		Want    string   `json:"want"`
	} `json:"people_text"`
	DataReads struct {
		Yes []string `json:"yes"`
		No  []string `json:"no"`
	} `json:"data_reads"`
}

func readGuardCases(t *testing.T) guardCases {
	t.Helper()
	var cases guardCases
	if err := json.Unmarshal(readSpecExample(t, "cases.json"), &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestGuardCases(t *testing.T) {
	cases := readGuardCases(t)
	t.Run("write tools", func(t *testing.T) {
		for _, name := range cases.WriteTools {
			if tools.Phase(name) != tools.PhaseWrite {
				t.Errorf("%s is not a write", name)
			}
		}
	})
	t.Run("keys", func(t *testing.T) {
		for _, c := range cases.Keys {
			other := cmp.Or(c.OtherTool, c.Tool)
			a, b := writeCall(c.Tool, c.A), writeCall(other, c.B)
			if got := writeKey(a) == writeKey(b); got != c.Same {
				t.Errorf("%s: write keys same = %v, want %v", c.Name, got, c.Same)
			}
			if c.GoReadsDiffer {
				continue
			}
			if got := Key(c.Tool, json.RawMessage(c.A)) == Key(other, json.RawMessage(c.B)); got != c.Same {
				t.Errorf("%s: read keys same = %v, want %v", c.Name, got, c.Same)
			}
		}
	})
	t.Run("count claims", func(t *testing.T) {
		for _, text := range cases.CountClaims.Match {
			if !countClaim.MatchString(text) {
				t.Errorf("%q is not a count claim", text)
			}
		}
		for _, text := range cases.CountClaims.NoMatch {
			if countClaim.MatchString(text) {
				t.Errorf("%q is a count claim", text)
			}
		}
	})
	t.Run("title causes", func(t *testing.T) {
		for _, c := range cases.TitleCauses {
			if got := doubtsTitleCause(c.Title, c.Text); got != c.Want {
				t.Errorf("%s: doubtsTitleCause = %v, want %v", c.Name, got, c.Want)
			}
		}
	})
	t.Run("conclusions", func(t *testing.T) {
		for _, c := range cases.Conclusions {
			if got := namesConclusion(c.Input); got != c.Want {
				t.Errorf("%s: namesConclusion = %v, want %v", c.Input, got, c.Want)
			}
		}
	})
	t.Run("proved", func(t *testing.T) {
		for _, c := range cases.Proved {
			if got := provedSomething(c.Prev, c.Next); got != c.Want {
				t.Errorf("%s: provedSomething = %v, want %v", c.Name, got, c.Want)
			}
		}
	})
	t.Run("people text", func(t *testing.T) {
		for _, c := range cases.PeopleText {
			if got := peopleText(c.Reasons); got != c.Want {
				t.Errorf("peopleText(%v) = %q, want %q", c.Reasons, got, c.Want)
			}
		}
	})
	t.Run("data reads", func(t *testing.T) {
		for _, name := range cases.DataReads.Yes {
			if !ReadsData(name) {
				t.Errorf("%s is not a data read", name)
			}
		}
		for _, name := range cases.DataReads.No {
			if ReadsData(name) {
				t.Errorf("%s is a data read", name)
			}
		}
	})
}

func TestEveryTraceToolIsAReadUnlessItIsASaveAWriteOrTheLedger(t *testing.T) {
	writes := readGuardCases(t).WriteTools
	for _, file := range traceFiles {
		var f traceFile
		if err := json.Unmarshal(readSpecExample(t, file+".json"), &f); err != nil {
			t.Fatal(err)
		}
		for _, tr := range f.Traces {
			for _, step := range tr.Steps {
				c := cmp.Or(step.Call, step.Forget)
				if c == nil {
					continue
				}
				if (tools.Phase(c.Name) == tools.PhaseWrite) != slices.Contains(writes, c.Name) {
					t.Errorf("%s: %s is phase %s, and the write tools are %v", tr.Name, c.Name, tools.Phase(c.Name), writes)
				}
			}
		}
	}
}
