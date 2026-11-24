package guard

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func readTurn(s *State, name, input string, isError bool) Admission {
	s.Window = Window{}
	s.BeginTurn(false)
	c := Call{Name: name, Input: json.RawMessage(input)}
	adm := s.Admit(0, c)
	if adm.Verdict == Dispatch {
		_, _ = s.Settle(c, tools.Result{IsError: isError}, []byte(`{"ok":true}`))
	}
	return adm
}

func TestBudgetLimits(t *testing.T) {
	if MaxRejectionsInRow != 3 || maxIdenticalReads != 3 || maxCallsBeforeSave != 6 || maxCallsBeforeProposal != 12 || DefaultInvestigationCalls != 12 || maxStalledTurns != 2 {
		t.Fatalf("budgets = %d %d %d %d %d %d", MaxRejectionsInRow, maxIdenticalReads, maxCallsBeforeSave, maxCallsBeforeProposal, DefaultInvestigationCalls, maxStalledTurns)
	}
}

func TestStateRefusesARepeatedSave(t *testing.T) {
	var s State
	if s.noteSave("a") || s.noteSave("b") {
		t.Fatal("a first save was reported as a repeat")
	}
	if !s.noteSave("a") {
		t.Fatal("the second save of a key was not reported as a repeat")
	}
	if !s.noteSave("a") {
		t.Fatal("the third save of a key was not reported as a repeat")
	}
	s.Restart()
	if s.noteSave("a") {
		t.Fatal("a save after a reset was reported as a repeat")
	}
}

func TestStateReplaysASuccessAndRefusesTheThird(t *testing.T) {
	var s State
	if adm := readTurn(&s, "get_panels", `{}`, false); adm.Verdict != Dispatch {
		t.Fatalf("first read = %v", adm.Verdict)
	}
	adm := readTurn(&s, "get_panels", `{}`, false)
	if adm.Verdict != Replay || adm.Note != messageReplayed || adm.Step != StepReusedRead {
		t.Fatalf("second read = %+v", adm)
	}
	for i := range 2 {
		if adm := readTurn(&s, "get_panels", `{}`, false); adm.Verdict != Refuse {
			t.Fatalf("read %d after the replay = %v, want a refusal", i+3, adm.Verdict)
		}
	}
}

func TestStateNeverReplaysAnError(t *testing.T) {
	var s State
	readTurn(&s, "list_metrics", `{}`, true)
	if adm := readTurn(&s, "list_metrics", `{}`, false); adm.Verdict != Dispatch {
		t.Fatalf("second read of an error = %v, want a live call", adm.Verdict)
	}
	if adm := readTurn(&s, "list_metrics", `{}`, false); adm.Verdict != Refuse {
		t.Fatalf("third read = %v, want a refusal", adm.Verdict)
	}
}

func TestStateNeverReplaysAQueryResult(t *testing.T) {
	for _, tool := range []string{"query_range", "query_instant"} {
		var s State
		readTurn(&s, tool, `{}`, false)
		if adm := readTurn(&s, tool, `{}`, false); adm.Verdict != Dispatch {
			t.Fatalf("%s: second read = %v, want a live call", tool, adm.Verdict)
		}
	}
}

func TestStateForgetsReadsOnANewSession(t *testing.T) {
	var s State
	readTurn(&s, "get_panels", `{}`, false)
	s.BeginSession(false, 0)
	if adm := readTurn(&s, "get_panels", `{}`, false); adm.Verdict != Dispatch {
		t.Fatalf("read after reset = %v", adm.Verdict)
	}
}

func TestReadRefusal(t *testing.T) {
	for _, c := range []struct {
		name    string
		mode    tools.Mode
		window  Window
		hasSave bool
		want    string
	}{
		{"first call", tools.ModeCreate, Window{Calls: 1}, false, ""},
		{"fifth call without a save", tools.ModeCreate, Window{Calls: 5}, false, ""},
		{"sixth call without a save", tools.ModeCreate, Window{Calls: 6}, false, guardSaveNow},
		{"sixth call of a revision", tools.ModeRevise, Window{Calls: 6}, false, guardSaveNow},
		{"a save beside the reads lets them run", tools.ModeCreate, Window{Calls: 9}, true, ""},
		{"the diagnose turn lets reads run", tools.ModeRevise, Window{Calls: 7, Diagnose: true}, false, ""},
		{"suggest runs reads up to the twelfth call", tools.ModeSuggest, Window{Calls: 11}, false, ""},
		{"suggest refuses from the twelfth call", tools.ModeSuggest, Window{Calls: 12}, false, guardProposeNow},
		{"suggest lets a proposal beside reads run", tools.ModeSuggest, Window{Calls: 13}, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := State{Mode: c.mode, Window: c.window}
			if got := s.readRefusal(c.hasSave); got != c.want {
				t.Fatalf("readRefusal(%v, %+v, %v) = %q, want %q", c.mode, c.window, c.hasSave, got, c.want)
			}
		})
	}
}

func TestAnInvestigationReadsUpToItsConfiguredCalls(t *testing.T) {
	for _, c := range []struct {
		name   string
		limit  int
		window Window
		want   string
	}{
		{"unset keeps twelve", 0, Window{Calls: 11}, ""},
		{"unset refuses the twelfth", 0, Window{Calls: 12}, guardFindingNow},
		{"raised runs the twelfth", 20, Window{Calls: 12}, ""},
		{"raised runs the nineteenth", 20, Window{Calls: 19}, ""},
		{"raised refuses the twentieth", 20, Window{Calls: 20}, guardFindingNow},
		{"lowest refuses the sixth", 6, Window{Calls: 6}, guardFindingNow},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := State{Mode: tools.ModeInvestigate, Window: c.window, InvestigationCalls: c.limit}
			if got := s.readRefusal(false); got != c.want {
				t.Fatalf("readRefusal = %q, want %q", got, c.want)
			}
		})
	}
}

func TestASuggestRunKeepsTwelveCallsWhateverTheInvestigationSetting(t *testing.T) {
	s := State{Mode: tools.ModeSuggest, Window: Window{Calls: 12}, InvestigationCalls: 40}
	if got := s.readRefusal(false); got != guardProposeNow {
		t.Fatalf("readRefusal = %q, want %q", got, guardProposeNow)
	}
	s.Mode, s.Window.Calls = tools.ModeCreate, 6
	if got := s.readRefusal(false); got != guardSaveNow {
		t.Fatalf("readRefusal = %q, want %q", got, guardSaveNow)
	}
}

func TestAnInvestigationWithTwentyCallsIsLimitedAtTheTwentiethAndNotBefore(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.InvestigationCalls = 20
	var refusedAt, nudgedAfter int
	for turn := 1; turn <= 25 && refusedAt == 0; turn++ {
		s.BeginTurn(false)
		adm := s.Admit(0, jsonCall("query_range", `{"q": `+strconv.Itoa(turn)+`}`))
		if adm.Verdict == Refuse {
			refusedAt = turn
			if !strings.HasPrefix(adm.Step, "Refused query_range: 20 model calls passed without a finding") || !strings.Contains(string(adm.Content), "20 model calls without a finding") {
				t.Fatalf("refusal = %q %s", adm.Step, adm.Content)
			}
			break
		}
		if nudge, _ := s.EndTurn(); nudge == messageFindingNext {
			nudgedAfter = turn
		}
	}
	if refusedAt != 20 || nudgedAfter != 19 {
		t.Fatalf("refused at call %d, nudged after call %d, want 20 and 19", refusedAt, nudgedAfter)
	}
}

func TestTheStoppedReasonNamesTheConfiguredInvestigationCalls(t *testing.T) {
	s := State{Mode: tools.ModeInvestigate, InvestigationCalls: 20, LastGuard: guardFindingNow}
	want := "Stopped: the model kept reading after 20 calls without a finding"
	if got := s.stalledReason(); got != want {
		t.Fatalf("stalledReason = %q, want %q", got, want)
	}
}

func TestNudgeFor(t *testing.T) {
	for _, c := range []struct {
		name   string
		mode   tools.Mode
		window Window
		want   string
	}{
		{"fourth call", tools.ModeCreate, Window{Calls: 4}, ""},
		{"fifth call", tools.ModeCreate, Window{Calls: 5}, messageSaveNext},
		{"fifth call of a revision", tools.ModeRevise, Window{Calls: 5}, messageSaveNext},
		{"sixth call", tools.ModeCreate, Window{Calls: 6}, ""},
		{"suggest fifth call", tools.ModeSuggest, Window{Calls: 5}, ""},
		{"suggest eleventh call", tools.ModeSuggest, Window{Calls: 11}, messageProposeNext},
		{"suggest twelfth call", tools.ModeSuggest, Window{Calls: 12}, ""},
		{"investigation eleventh call", tools.ModeInvestigate, Window{Calls: 11}, messageFindingNext},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := State{Mode: c.mode, Window: c.window}
			if got := s.nudge(); got != c.want {
				t.Fatalf("nudge(%v, %+v) = %q, want %q", c.mode, c.window, got, c.want)
			}
		})
	}
}

func TestTheSaveNowMessageNamesTheSixthCall(t *testing.T) {
	want := "6 model calls without a save; only a save can run now. Save the panel you have checked, or call save_finding if the request only asks a question, and say in it what you did not read; reads in the same call as a save still run."
	if got := rules[guardSaveNow].message; got != want {
		t.Fatalf("message = %q", got)
	}
}

func jsonCall(name, input string) Call {
	return Call{Name: name, Input: json.RawMessage(input)}
}

func TestAdmitOrdersAThreeReadTurnWithADuplicateAndTwoSaves(t *testing.T) {
	s := New(tools.ModeCreate)
	s.BeginTurn(true)
	calls := []Call{
		jsonCall("get_panels", `{"a":1}`),
		jsonCall("list_metrics", `{}`),
		jsonCall("get_panels", `{ "a": 1 }`),
		jsonCall("update_panels", `{"n":1}`),
		jsonCall("update_panels", `{"n":2}`),
	}
	order := []int{0, 1, 2, 3, 4}
	var got []Verdict
	for _, i := range order {
		got = append(got, s.Admit(i, calls[i]).Verdict)
	}
	if want := []Verdict{Dispatch, Dispatch, Reuse, Dispatch, Refuse}; !reflect.DeepEqual(got, want) {
		t.Fatalf("verdicts = %v, want %v", got, want)
	}
	if s.Turn.Answered != 4 || s.Counts.ToolCalls != 5 {
		t.Fatalf("answered %d, tool calls %d", s.Turn.Answered, s.Counts.ToolCalls)
	}
	s.BeginTurn(true)
	extra := s.Admit(1, jsonCall("update_panels", `{"n":2}`))
	if extra.Verdict != Dispatch {
		t.Fatalf("a save in a fresh turn = %v", extra.Verdict)
	}
	second := s.Admit(2, jsonCall("update_dashboard", `{}`))
	if second.Verdict != Refuse || !strings.Contains(string(second.Content), "only one save per turn") || second.Step != "Rejected update_dashboard input: only one save per turn" {
		t.Fatalf("second save of a turn = %+v", second)
	}
	if string(second.Content) != `{"code":"invalid_request","message":"only one save per turn"}` {
		t.Fatalf("content = %s", second.Content)
	}
}

func TestAdmitRefusesAReadOfARefusedKeyAgainInTheSameTurn(t *testing.T) {
	s := New(tools.ModeCreate)
	for range maxIdenticalReads - 1 {
		readTurn(&s, "get_panels", `{}`, true)
	}
	s.BeginTurn(false)
	first := s.Admit(0, jsonCall("get_panels", `{}`))
	again := s.Admit(1, jsonCall("get_panels", `{}`))
	if first.Verdict != Refuse || again.Verdict != Reuse || again.ReuseOf != 0 {
		t.Fatalf("first %+v, again %+v", first, again)
	}
	if first.Step != "Refused get_panels: the same read was asked 3 times" || s.LastGuard != guardRepeatedRead {
		t.Fatalf("step %q, last guard %q", first.Step, s.LastGuard)
	}
	if s.Turn.Answered != 0 {
		t.Fatalf("answered = %d, want 0", s.Turn.Answered)
	}
}

func TestAdmitRefusesSaveFindingOnceBuilding(t *testing.T) {
	s := New(tools.ModeCreate)
	s.BeginSession(true, 0)
	s.BeginTurn(true)
	adm := s.Admit(0, jsonCall("save_finding", `{}`))
	if adm.Verdict != Refuse || adm.Step != "Rejected save_finding input: this run already saved panels; finish the dashboard instead of saving a finding" {
		t.Fatalf("admission = %+v", adm)
	}
}

func TestAdmitRefusesARepeatedSaveAndCountsARepairPhase(t *testing.T) {
	s := New(tools.ModeRevise)
	s.Rejections = 1
	s.BeginTurn(true)
	first := s.Admit(0, jsonCall("update_panels", `{"n":1}`))
	if first.Verdict != Dispatch || first.Phase != PhaseRepair || s.Counts.Repairs != 1 {
		t.Fatalf("first save = %+v, repairs %d", first, s.Counts.Repairs)
	}
	s.BeginTurn(true)
	again := s.Admit(0, jsonCall("update_panels", `{"n": 1}`))
	if again.Verdict != Refuse || again.Step != "Refused update_panels: the same save was sent again" || s.Counts.Repairs != 1 {
		t.Fatalf("repeated save = %+v, repairs %d", again, s.Counts.Repairs)
	}
}

func TestAdmitCountsReadsRefusedByTheWindow(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Window.Calls = maxCallsBeforeSave - 1
	s.BeginTurn(false)
	adm := s.Admit(0, jsonCall("get_panels", `{}`))
	if adm.Verdict != Refuse || s.Counts.SaveNowRefusals != 1 || s.LastGuard != guardSaveNow {
		t.Fatalf("admission = %+v, refusals %d, last guard %q", adm, s.Counts.SaveNowRefusals, s.LastGuard)
	}
	if !strings.HasPrefix(adm.Step, "Refused get_panels: 6 model calls passed without a save") {
		t.Fatalf("step = %q", adm.Step)
	}
}

func TestAFindingSavedAfterRefusedReadsCarriesTheirCount(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Window.Calls = maxCallsBeforeSave - 1
	s.BeginTurn(false)
	s.Admit(0, jsonCall("query_range", `{"q": 1}`))
	s.Admit(1, jsonCall("log_patterns", `{"q": 2}`))
	if _, stop := s.EndTurn(); stop != nil {
		t.Fatalf("stop = %+v", stop)
	}
	s.BeginTurn(false)
	adm := s.Admit(0, jsonCall("save_finding", `{"markdown": "x"}`))
	if adm.Verdict != Dispatch || adm.ReadsRefused != 2 {
		t.Fatalf("admission = %+v", adm)
	}
}

func TestAnotherSaveCarriesNoRefusedReadCount(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Counts.SaveNowRefusals = 3
	s.BeginTurn(true)
	if adm := s.Admit(0, jsonCall("update_panels", `{"n": 1}`)); adm.ReadsRefused != 0 {
		t.Fatalf("admission = %+v", adm)
	}
}

func TestSettleClassifiesASave(t *testing.T) {
	ok := func(body string) (tools.Result, []byte) { return tools.Result{}, []byte(body) }
	fail := func(body string) (tools.Result, []byte) { return tools.Result{IsError: true}, []byte(body) }
	for _, c := range []struct {
		name   string
		mode   tools.Mode
		tool   string
		parent int
		result func() (tools.Result, []byte)
		check  func(t *testing.T, got Settled, err error, s *State)
	}{
		{"a saved version", tools.ModeRevise, "update_panels", 0, func() (tools.Result, []byte) {
			return ok(`{"dashboard_id":"d","version":5,"replayed":true}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || got.Outcome.Saved == nil || *got.Outcome.Saved != (Saved{DashboardID: "d", Version: 5, Replayed: true}) || s.Counts.BuildSaves != 1 {
				t.Fatalf("got %+v, %v, build saves %d", got, err, s.Counts.BuildSaves)
			}
		}},
		{"an import is no build save", tools.ModeCreate, "import_grafana_dashboard", 0, func() (tools.Result, []byte) {
			return ok(`{"dashboard_id":"d","version":1}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || got.Outcome.Saved == nil || s.Counts.BuildSaves != 0 {
				t.Fatalf("got %+v, %v, build saves %d", got, err, s.Counts.BuildSaves)
			}
		}},
		{"a finding", tools.ModeInvestigate, "save_finding", 0, func() (tools.Result, []byte) {
			return ok(`{"finding_id":"f"}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || got.Outcome.Finding != "f" {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a finding with no id", tools.ModeInvestigate, "save_finding", 0, func() (tools.Result, []byte) {
			return ok(`{}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("err = %v", err)
			}
		}},
		{"a suggestion", tools.ModeSuggest, "create_dashboard", 0, func() (tools.Result, []byte) {
			return ok(`{"suggested":true,"panel":{"id":"p"}}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || string(got.Outcome.Suggestion) != `{"id":"p"}` {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a build save", tools.ModeCreate, "update_panels", 0, func() (tools.Result, []byte) {
			return tools.Result{Edits: tools.Edits{Panels: []string{"a"}}}, []byte(`{"build":{},"version":0}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !got.Outcome.Built || !s.Building || s.Counts.BuildSaves != 1 {
				t.Fatalf("got %+v, %v, state %+v", got, err, s)
			}
		}},
		{"a proposed save", tools.ModeRevise, "update_dashboard", 3, func() (tools.Result, []byte) {
			return ok(`{"code":"proposed","proposal_id":"p1","revision":1,"opened":true}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || got.Outcome.Proposal != "p1" || got.Outcome.Saved != nil || s.Counts.BuildSaves != 1 || !s.Turn.Progress {
				t.Fatalf("got %+v, %v, state %+v", got, err, s)
			}
		}},
		{"a proposed save without a proposal id", tools.ModeRevise, "update_dashboard", 3, func() (tools.Result, []byte) {
			return ok(`{"code":"proposed"}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("err = %v", err)
			}
		}},
		{"a refusal to remove panels in propose mode is a rejection", tools.ModeRevise, "update_dashboard", 3, func() (tools.Result, []byte) {
			return fail(`{"code":"proposal_required","message":"m","removed":[{"kind":"panel","id":"a"}]}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !got.Outcome.Rejected || s.Rejections != 1 || !reflect.DeepEqual(got.Outcome.IssueCodes, []string{"proposal_required"}) {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a rejection names its failing panels", tools.ModeCreate, "update_panels", 0, func() (tools.Result, []byte) {
			return fail(`{"code":"invalid_spec","failing_panels":["a","a","b-1_x","b"],"issues":[{"code":"query_empty"}]}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !reflect.DeepEqual(got.Outcome.FailingPanels, []string{"a", "b-1_x", "b"}) || got.Outcome.FailingOther != 0 {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"an id that is no panel id is counted and never kept", tools.ModeCreate, "update_panels", 0, func() (tools.Result, []byte) {
			return fail(`{"code":"invalid_spec","failing_panels":["p1","Open http://example.com for the fix","1st","` + strings.Repeat("a", 65) + `","1st"],"issues":[{"code":"query_empty"}]}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !reflect.DeepEqual(got.Outcome.FailingPanels, []string{"p1"}) || got.Outcome.FailingOther != 3 {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a conflict on a newer version", tools.ModeRevise, "update_panels", 3, func() (tools.Result, []byte) {
			return fail(`{"code":"version_conflict","latest_version":4}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !got.Outcome.Conflict || s.Rejections != 0 {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a conflict on the parent counts as a rejection", tools.ModeRevise, "update_panels", 3, func() (tools.Result, []byte) {
			return fail(`{"code":"version_conflict","latest_version":3}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !got.Outcome.Rejected || s.Rejections != 1 {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a missing dashboard", tools.ModeRevise, "update_panels", 3, func() (tools.Result, []byte) {
			return fail(`{"code":"not_found"}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !got.Outcome.NotFound {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a rejection lists plain issue codes once, at most three", tools.ModeCreate, "update_panels", 0, func() (tools.Result, []byte) {
			return fail(`{"code":"invalid_spec","issues":[{"code":"a"},{"code":"a"},{"code":"b c"},{"code":"d"},{"code":"e"},{"code":"f"}]}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !reflect.DeepEqual(got.Outcome.IssueCodes, []string{"a", "d", "e"}) {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a rejection without issues names its code", tools.ModeCreate, "update_panels", 0, func() (tools.Result, []byte) {
			return fail(`{"code":"invalid_spec"}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if err != nil || !reflect.DeepEqual(got.Outcome.IssueCodes, []string{"invalid_spec"}) {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"a result that is not JSON", tools.ModeCreate, "update_panels", 0, func() (tools.Result, []byte) {
			return ok(`nope`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("err = %v", err)
			}
		}},
		{"an error with no code", tools.ModeCreate, "update_panels", 0, func() (tools.Result, []byte) {
			return fail(`{}`)
		}, func(t *testing.T, got Settled, err error, s *State) {
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("err = %v", err)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := New(c.mode)
			s.ParentVersion = c.parent
			res, content := c.result()
			got, err := s.Settle(jsonCall(c.tool, `{}`), res, content)
			c.check(t, got, err, &s)
		})
	}
}

func TestSettleStopsAfterTheFourthSaveThatChangedNothingNew(t *testing.T) {
	s := New(tools.ModeCreate)
	build := func() Settled {
		got, err := s.Settle(jsonCall("update_panels", `{}`), tools.Result{}, []byte(`{"build":{},"version":0}`))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for range maxChurnedSavesInRow {
		if got := build(); got.Stop != nil || got.Note == "" {
			t.Fatalf("got %+v", got)
		}
	}
	got := build()
	want := &Stop{Failure: revisions.FailureNoProgress, Step: "Stopped: 4 saves in a row changed nothing new"}
	if !reflect.DeepEqual(got.Stop, want) {
		t.Fatalf("stop = %+v, want %+v", got.Stop, want)
	}
}

func TestEndTurnStopsAfterTwoTurnsInARowWithEveryCallRefused(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Window.Calls = maxCallsBeforeSave - 1
	turn := func() (string, *Stop) {
		s.BeginTurn(false)
		s.Admit(0, jsonCall("get_panels", `{}`))
		return s.EndTurn()
	}
	if _, stop := turn(); stop != nil {
		t.Fatalf("first stalled turn stopped: %+v", stop)
	}
	_, stop := turn()
	if stop == nil || stop.Failure != revisions.FailureNoProgress || stop.Step != "Stopped: the model kept reading after 6 calls without a save" {
		t.Fatalf("stop = %+v", stop)
	}
	if s.Counts.StalledTurns != 2 {
		t.Fatalf("stalled turns = %d", s.Counts.StalledTurns)
	}
}

func TestEndTurnStopsAfterRejectedSavesInARow(t *testing.T) {
	s := New(tools.ModeSuggest)
	for i := range MaxRejectionsInRow {
		s.BeginTurn(true)
		c := jsonCall("create_dashboard", `{"n":`+string(rune('1'+i))+`}`)
		s.Admit(0, c)
		if _, err := s.Settle(c, tools.Result{IsError: true}, []byte(`{"code":"invalid_spec"}`)); err != nil {
			t.Fatal(err)
		}
		_, stop := s.EndTurn()
		if i < MaxRejectionsInRow-1 && stop != nil {
			t.Fatalf("stopped after %d rejections", i+1)
		}
		if i == MaxRejectionsInRow-1 && (stop == nil || stop.Failure != revisions.FailureValidationExhausted || stop.Step != "Stopped: 3 proposals in a row were rejected") {
			t.Fatalf("stop = %+v", stop)
		}
	}
}

func TestRestrictedAnswerRefusalsAreNotValidationRejections(t *testing.T) {
	s := New(tools.ModeCreate)
	refuse := func(i int, content string) *Stop {
		s.BeginTurn(true)
		c := jsonCall("create_dashboard", `{"n":`+strconv.Itoa(i+1)+`}`)
		s.Admit(0, c)
		if _, err := s.Settle(c, tools.Result{IsError: true}, []byte(content)); err != nil {
			t.Fatal(err)
		}
		_, stop := s.EndTurn()
		return stop
	}
	for i := range MaxRejectionsInRow {
		stop := refuse(i, `{"code":"restricted_answer","message":"m"}`)
		if s.Rejections != 0 {
			t.Fatalf("rejections = %d after %d restricted refusals", s.Rejections, i+1)
		}
		if i < MaxRejectionsInRow-1 && stop != nil {
			t.Fatalf("stopped after %d restricted refusals: %+v", i+1, stop)
		}
		if i == MaxRejectionsInRow-1 && (stop == nil || stop.Failure != revisions.FailureRestrictedAnswer || stop.Step != "Stopped: 3 saves in a row were refused because the run read data only Admins can read") {
			t.Fatalf("stop = %+v", stop)
		}
	}
}

func TestRestrictedAnswerRefusalsBreakTheRunOfValidationRejections(t *testing.T) {
	s := New(tools.ModeCreate)
	codes := []string{"invalid_spec", "invalid_spec", "restricted_answer", "invalid_spec", "invalid_spec"}
	for i, code := range codes {
		s.BeginTurn(true)
		c := jsonCall("create_dashboard", `{"n":`+string(rune('1'+i))+`}`)
		s.Admit(0, c)
		if _, err := s.Settle(c, tools.Result{IsError: true}, []byte(`{"code":"`+code+`"}`)); err != nil {
			t.Fatal(err)
		}
		if _, stop := s.EndTurn(); stop != nil {
			t.Fatalf("stopped after %v: %+v", codes[:i+1], stop)
		}
	}
	if s.Rejections != 2 || s.RestrictedRefusals != 0 {
		t.Fatalf("rejections = %d, restricted refusals = %d", s.Rejections, s.RestrictedRefusals)
	}
}

func TestEndTurnNudgesOneCallBeforeTheWindowClosesAndCountsIt(t *testing.T) {
	s := New(tools.ModeCreate)
	s.Window.Calls = maxCallsBeforeSave - 2
	s.BeginTurn(false)
	s.Admit(0, jsonCall("get_panels", `{}`))
	nudge, stop := s.EndTurn()
	if nudge != messageSaveNext || stop != nil || s.Counts.Nudges != 1 {
		t.Fatalf("nudge %q, stop %+v, nudges %d", nudge, stop, s.Counts.Nudges)
	}
}

func populated() State {
	s := New(tools.ModeRevise)
	s.BeginSession(true, 7)
	s.Rejections = 2
	s.Window = Window{Calls: 4, Stalled: 1, Diagnose: true}
	s.LastGuard = guardRepeatedRead
	s.Churn = map[string]int{"a": 2, "": 1}
	s.ChurnedInRow = 1
	s.Reads = map[string]Read{"get_panels:x": {Times: 2, Replayable: true}, "list_metrics:y": {Times: 1}}
	s.Saves = map[string]bool{"update_panels:z": true}
	s.Turn = Turn{ReadGuard: guardSaveNow, SaveSeen: true, Answered: 2, Progress: true, Rejected: true, Keys: map[string]TurnKey{"k": {Position: 3, Refused: true}}}
	s.Counts = Counts{ToolCalls: 9, Repairs: 1, BuildSaves: 2, Nudges: 3, SaveNowRefusals: 4, StalledTurns: 5, OnePanelRefusals: 6}
	return s
}

func TestStateRoundTrip(t *testing.T) {
	want := populated()
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got State
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the state:\n got %+v\nwant %+v", got, want)
	}
}

func TestStateHoldsNoToolInputOrResult(t *testing.T) {
	s := New(tools.ModeCreate)
	s.BeginTurn(false)
	c := jsonCall("get_panels", `{"secret":"input-text"}`)
	s.Admit(0, c)
	if _, err := s.Settle(c, tools.Result{Content: "result-text"}, []byte(`{"secret":"result-text"}`)); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(s)
	if strings.Contains(string(raw), "input-text") || strings.Contains(string(raw), "result-text") {
		t.Fatalf("state holds tool text: %s", raw)
	}
}

type scriptedCall struct {
	call    Call
	result  tools.Result
	content string
}

type scriptedTurn struct {
	hasSave bool
	calls   []scriptedCall
}

type decision struct {
	Verdicts []Admission
	Settled  []Settled
	Errs     []error
	Nudge    string
	Stop     *Stop
}

func playScript(s State, script []scriptedTurn, reload bool) (State, []decision) {
	step := func(s State) State {
		if !reload {
			return s
		}
		raw, err := json.Marshal(s)
		if err != nil {
			panic(err)
		}
		var out State
		if err := json.Unmarshal(raw, &out); err != nil {
			panic(err)
		}
		return out
	}
	var all []decision
	for _, turn := range script {
		var d decision
		s = step(s)
		s.BeginTurn(turn.hasSave)
		for i, c := range turn.calls {
			s = step(s)
			adm := s.Admit(i, c.call)
			d.Verdicts = append(d.Verdicts, adm)
		}
		for i, c := range turn.calls {
			if d.Verdicts[i].Verdict == Refuse || d.Verdicts[i].Verdict == Reuse || d.Verdicts[i].Verdict == Replay {
				continue
			}
			s = step(s)
			settled, err := s.Settle(c.call, c.result, []byte(c.content))
			d.Settled = append(d.Settled, settled)
			d.Errs = append(d.Errs, err)
		}
		s = step(s)
		d.Nudge, d.Stop = s.EndTurn()
		all = append(all, d)
	}
	return s, all
}

func TestAStateReloadedBetweenEveryCallDecidesLikeOneKeptInMemory(t *testing.T) {
	read := func(name, input string) scriptedCall {
		return scriptedCall{call: jsonCall(name, input), content: `{"ok":true}`}
	}
	save := func(input, content string, res tools.Result) scriptedCall {
		return scriptedCall{call: jsonCall("update_panels", input), result: res, content: content}
	}
	rejected := tools.Result{IsError: true}
	build := tools.Result{Edits: tools.Edits{Panels: []string{"a"}}}
	script := []scriptedTurn{
		{calls: []scriptedCall{read("get_panels", `{}`), read("list_metrics", `{}`)}},
		{calls: []scriptedCall{read("get_panels", `{}`), read("get_panels", `{ }`)}},
		{calls: []scriptedCall{read("get_panels", `{}`)}},
		{hasSave: true, calls: []scriptedCall{read("get_panels", `{}`), save(`{"n":1}`, `{"code":"invalid_spec","issues":[{"code":"x"}]}`, rejected)}},
		{hasSave: true, calls: []scriptedCall{save(`{"n":1}`, `{}`, tools.Result{}), save(`{"n":2}`, `{}`, tools.Result{})}},
		{hasSave: true, calls: []scriptedCall{save(`{"n":3}`, `{"build":{},"version":0}`, build)}},
		{calls: []scriptedCall{read("list_metrics", `{}`)}},
		{calls: []scriptedCall{read("list_metrics", `{}`)}},
		{calls: []scriptedCall{read("list_metrics", `{}`)}},
		{calls: []scriptedCall{read("list_metrics", `{}`)}},
		{calls: []scriptedCall{read("list_metrics", `{}`)}},
		{calls: []scriptedCall{read("list_metrics", `{}`)}},
		{calls: []scriptedCall{read("list_metrics", `{}`)}},
	}
	memState, memDecisions := playScript(New(tools.ModeCreate), script, false)
	reloadedState, reloadedDecisions := playScript(New(tools.ModeCreate), script, true)
	if !reflect.DeepEqual(memDecisions, reloadedDecisions) {
		t.Fatalf("decisions differ:\n memory   %+v\n reloaded %+v", memDecisions, reloadedDecisions)
	}
	if !reflect.DeepEqual(memState, reloadedState) {
		t.Fatalf("states differ:\n memory   %+v\n reloaded %+v", memState, reloadedState)
	}
	sawReplay, sawRefusal, sawStop := false, false, false
	for _, d := range memDecisions {
		sawStop = sawStop || d.Stop != nil
		for _, a := range d.Verdicts {
			sawReplay = sawReplay || a.Verdict == Replay
			sawRefusal = sawRefusal || a.Verdict == Refuse
		}
	}
	if !sawReplay || !sawRefusal || !sawStop {
		t.Fatalf("the script did not exercise a replay (%v), a refusal (%v) and a stop (%v)", sawReplay, sawRefusal, sawStop)
	}
}

func TestThePanelIDPatternIsTheSchemasOwn(t *testing.T) {
	schema, err := os.ReadFile("../../../spec/schema/observespec.v0.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), `"pattern": "`+panelID.String()+`"`) {
		t.Errorf("the schema has no id pattern %s", panelID)
	}
}

func TestAFindingAfterANewSessionCountsOnlyTheReadsRefusedSince(t *testing.T) {
	s := New(tools.ModeInvestigate)
	s.Window.Calls = maxCallsBeforeProposal - 1
	s.BeginTurn(false)
	s.Admit(0, jsonCall("query_range", `{"q": 1}`))
	s.Admit(1, jsonCall("log_patterns", `{"q": 2}`))
	s.EndTurn()
	s.BeginSession(false, 0)
	s.BeginTurn(false)
	if adm := s.Admit(0, jsonCall("save_finding", `{"markdown": "x"}`)); adm.ReadsRefused != 0 {
		t.Fatalf("a finding after a fresh window reports %d refused reads", adm.ReadsRefused)
	}
	if s.Counts.SaveNowRefusals != 2 {
		t.Fatalf("the run's own refusal count changed to %d", s.Counts.SaveNowRefusals)
	}
}

func settleRead(s *State, name, input string, res tools.Result) {
	s.Window = Window{}
	s.BeginTurn(false)
	c := Call{Name: name, Input: json.RawMessage(input)}
	if adm := s.Admit(0, c); adm.Verdict == Dispatch {
		_, _ = s.Settle(c, res, []byte(`{"ok":true}`))
	}
}

func TestAFindingCarriesTheReadsThatCouldNotBeChecked(t *testing.T) {
	s := New(tools.ModeCreate)
	timedOut := tools.Result{IsError: true, Unchecked: "query_timeout"}
	settleRead(&s, "query_range", `{"datasource":"loki","query":"{service=\"checkout\"}"}`, timedOut)
	settleRead(&s, "log_patterns", `{"datasource":"loki","query":"{service=\"checkout\"}"}`, timedOut)
	settleRead(&s, "list_metrics", `{"datasource":"prom","search":"checkout"}`, tools.Result{IsError: true, Unchecked: "datasource_unavailable"})
	settleRead(&s, "list_metrics", `{"datasource":"prom","scope":"{service=\"checkout\"}"}`, tools.Result{})
	settleRead(&s, "lint_query", `{"datasource":"ch","query":"SELECT 1"}`, tools.Result{IsError: true, Unchecked: "query_timeout"})
	settleRead(&s, "query_instant", `{"datasource":"ch","query":"SELECT 1"}`, tools.Result{IsError: true})

	s.BeginTurn(false)
	adm := s.Admit(0, jsonCall("save_finding", `{"markdown": "x"}`))
	want := []tools.Unchecked{{Datasource: "loki", Code: "query_timeout"}}
	if adm.Verdict != Dispatch || !reflect.DeepEqual(adm.Unchecked, want) {
		t.Fatalf("admission = %+v, want unchecked %+v", adm, want)
	}
}

func TestAReadThatLaterSucceedsIsNoLongerUnchecked(t *testing.T) {
	s := New(tools.ModeInvestigate)
	input := `{"datasource":"loki","query":"{service=\"checkout\"}"}`
	settleRead(&s, "query_range", input, tools.Result{IsError: true, Unchecked: "query_timeout"})
	settleRead(&s, "query_range", `{"datasource":"loki","query":"{service=\"checkout\"} |= \"error\""}`, tools.Result{})
	s.BeginTurn(false)
	if adm := s.Admit(0, jsonCall("save_finding", `{"markdown": "x"}`)); len(adm.Unchecked) != 0 {
		t.Fatalf("unchecked = %+v after the read succeeded", adm.Unchecked)
	}
}

func TestAFindingThatCallsASignalMissingNeedsTheReadThatWouldShowIt(t *testing.T) {
	tests := []struct {
		name     string
		mode     tools.Mode
		reads    [][2]string
		markdown string
		want     string
	}{
		{"no metrics after a name search", tools.ModeInvestigate, [][2]string{{"list_metrics", `{"datasource":"prom","search":"checkout"}`}}, "Prometheus has no checkout metrics.", "set scope"},
		{"no metrics after a scoped list", tools.ModeInvestigate, [][2]string{{"list_metrics", `{"datasource":"prom","scope":"{service=\"checkout\"}"}`}}, "Prometheus has no checkout metrics.", ""},
		{"no deploy without annotations in an investigation", tools.ModeInvestigate, nil, "There was no deploy of checkout today.", "read it with tag deploy"},
		{"no deploy after reading annotations", tools.ModeInvestigate, [][2]string{{"list_annotations", `{"tags":["deploy"]}`}}, "There was no deploy of checkout today.", ""},
		{"no deploy on Home without annotations", tools.ModeCreate, nil, "No checkout deploy exists.", "read it with tag deploy"},
		{"no deploy on Home after reading annotations", tools.ModeCreate, [][2]string{{"list_annotations", `{"tags":["deploy"]}`}}, "No checkout deploy exists.", ""},
		{"a finding that claims no absence", tools.ModeInvestigate, nil, "Checkout errors rose at 14:22 UTC on checkout-0.", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.mode)
			for _, r := range tt.reads {
				settleRead(&s, r[0], r[1], tools.Result{})
			}
			s.BeginTurn(false)
			body, err := json.Marshal(map[string]string{"markdown": tt.markdown})
			if err != nil {
				t.Fatal(err)
			}
			adm := s.Admit(0, jsonCall("save_finding", string(body)))
			if tt.want == "" {
				if adm.Verdict != Dispatch {
					t.Fatalf("admission = %+v, want the finding saved", adm)
				}
				return
			}
			if adm.Verdict != Refuse || !strings.Contains(string(adm.Content), tt.want) {
				t.Fatalf("admission = %+v, want refused with %q", adm, tt.want)
			}
			s.BeginTurn(false)
			if again := s.Admit(0, jsonCall("save_finding", string(body))); again.Verdict != Dispatch {
				t.Fatalf("second save = %+v, want it let through after one refusal", again)
			}
		})
	}
}

func TestARejectedFindingNamesItsEvidenceAndWhetherADatasourceWasUnreachable(t *testing.T) {
	cases := []struct {
		name      string
		mode      tools.Mode
		tool      string
		body      string
		finding   bool
		transient bool
		failing   []string
	}{
		{"timed out evidence", tools.ModeInvestigate, "save_finding", `{"code":"invalid_spec","failing_panels":["evidence-2"],"issues":[{"code":"query_timeout"},{"code":"datasource_unavailable"}]}`, true, true, []string{"evidence-2"}},
		{"evidence that is empty", tools.ModeInvestigate, "save_finding", `{"code":"invalid_spec","failing_panels":["evidence-1"],"issues":[{"code":"query_timeout"},{"code":"query_empty"}]}`, true, false, []string{"evidence-1"}},
		{"a finding in a question run", tools.ModeCreate, "save_finding", `{"code":"finding_too_long"}`, true, false, nil},
		{"a dashboard save", tools.ModeCreate, "update_panels", `{"code":"invalid_spec","failing_panels":["p1"],"issues":[{"code":"query_timeout"}]}`, false, true, []string{"p1"}},
	}
	for _, tc := range cases {
		s := New(tc.mode)
		s.BeginTurn(true)
		c := jsonCall(tc.tool, `{"n":1}`)
		s.Admit(0, c)
		settled, err := s.Settle(c, tools.Result{IsError: true}, []byte(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		out := settled.Outcome
		if !out.Rejected || out.RejectedFinding != tc.finding || out.Transient != tc.transient || !slices.Equal(out.FailingPanels, tc.failing) {
			t.Errorf("%s: outcome = %+v", tc.name, out)
		}
	}
}
