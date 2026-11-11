package runsession

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func contentCode(t *testing.T, a Answer) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(a.Content, &body); err != nil {
		t.Fatal(err)
	}
	return body.Code
}

func hasNote(a Answer, fragment string) bool {
	return slices.ContainsFunc(a.Notes, func(n string) bool { return strings.Contains(n, fragment) })
}

func settledOf(a Answer) string {
	if a.Meta.Settled == nil {
		return ""
	}
	return *a.Meta.Settled
}

func TestReadsRunUntilTheSixthTurnWithoutASaveAndThenAreRefused(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	dispatched := &dispatchCount{}
	for n := range 5 {
		answer := r.call(req, "list_metrics", `{"q":"`+strconv.Itoa(n)+`"}`, nil, dispatched.wrap(readOK))
		if answer.IsError {
			t.Fatalf("read %d was refused: %s", n+1, answer.Content)
		}
		if nudged := hasNote(answer, "Save the next panel"); nudged != (n == 4) {
			t.Fatalf("read %d: nudge = %v, notes %q", n+1, nudged, answer.Notes)
		}
	}
	refused := r.call(req, "list_metrics", `{"q":"z"}`, nil, dispatched.wrap(readOK))
	if !refused.IsError || contentCode(t, refused) != "invalid_request" {
		t.Fatalf("sixth read = %s, want refused", refused.Content)
	}
	if dispatched.n != 5 {
		t.Fatalf("dispatched %d reads, want 5: a refused call never runs", dispatched.n)
	}
	if got := r.stored(req).State; got != string(revisions.StateRunning) {
		t.Fatalf("run state = %s", got)
	}
	last := r.steps.all()[len(r.steps.all())-1]
	if last.Status != "error" || !strings.HasPrefix(last.Message, "Refused list_metrics") {
		t.Fatalf("refusal step = %+v", last)
	}
}

func TestAReadInATurnThatHoldsASaveRunsAfterTheSixthTurn(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	for n := range 5 {
		r.read(req, n)
	}
	turn := &Turn{ID: "t6", Calls: 2, Saves: 1}
	read := r.call(req, "list_metrics", `{"q":"z"}`, turn, readOK)
	if read.IsError {
		t.Fatalf("a read in a turn with a save was refused: %s", read.Content)
	}
	save := r.call(req, "update_panels", `{"n":1}`, turn, buildSaved(r, "a", 2))
	if save.IsError {
		t.Fatalf("save refused: %s", save.Content)
	}
	if save.Meta.Build == nil || save.Meta.Build.Saves != 1 {
		t.Fatalf("meta = %+v", save.Meta)
	}
}

func TestAThirdIdenticalReadIsRefusedAndTheSecondCarriesTheReplayNote(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	first := r.call(req, "list_metrics", `{"q":"a"}`, nil, readOK)
	second := r.call(req, "list_metrics", `{"q":"a"}`, nil, readOK)
	third := r.call(req, "list_metrics", `{"q":"a"}`, nil, readOK)
	if first.IsError || len(first.Notes) != 0 {
		t.Fatalf("first = %+v", first)
	}
	if second.IsError || !hasNote(second, "result of the same earlier call") {
		t.Fatalf("second = %+v", second)
	}
	if !third.IsError || !strings.Contains(string(third.Content), "already ran 2 times") {
		t.Fatalf("third = %s", third.Content)
	}
}

func TestASecondSaveInOneTurnIsRefusedWithoutRunning(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	turn := &Turn{ID: "t1", Calls: 2, Saves: 2}
	dispatched := &dispatchCount{}
	first := r.call(req, "update_panels", `{"n":1}`, turn, dispatched.wrap(buildSaved(r, "a", 3)))
	second := r.call(req, "update_panels", `{"n":2}`, turn, dispatched.wrap(buildSaved(r, "b", 3)))
	if first.IsError || !second.IsError || !strings.Contains(string(second.Content), "only one save per turn") {
		t.Fatalf("first %s, second %s", first.Content, second.Content)
	}
	if dispatched.n != 1 {
		t.Fatalf("dispatched %d saves, want 1", dispatched.n)
	}
}

func TestARepeatedSaveIsRefused(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.call(req, "update_panels", `{"n":1}`, nil, buildSaved(r, "a", 3))
	again := r.call(req, "update_panels", `{"n":1}`, nil, buildSaved(r, "a", 3))
	if !again.IsError || !strings.Contains(string(again.Content), "identical to an earlier one") {
		t.Fatalf("repeat = %s", again.Content)
	}
}

func TestSaveFindingIsRefusedWhileABuildExists(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.call(req, "update_panels", `{"n":1}`, nil, buildSaved(r, "a", 3))
	answer := r.call(req, "save_finding", `{"x":1}`, nil, findingSaved)
	if !answer.IsError || !strings.Contains(string(answer.Content), "already saved panels") {
		t.Fatalf("finding = %s", answer.Content)
	}
}

func TestThreeRejectedSavesInARowEndTheRunValidationExhausted(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	var last Answer
	for n := range guard.MaxRejectionsInRow {
		last = r.call(req, "update_panels", `{"n":`+strconv.Itoa(n+1)+`}`, nil, rejectedSave)
	}
	row := r.stored(req)
	if row.State != string(revisions.StateFailed) || row.FailureCode.String != string(revisions.FailureValidationExhausted) {
		t.Fatalf("run = %s %q", row.State, row.FailureCode.String)
	}
	want := "the dashboard still failed validation after 3 rejected saves in a row (last issues: unknown_field)"
	if row.Error.String != want {
		t.Fatalf("stored message = %q, want %q", row.Error.String, want)
	}
	if settledOf(last) != "ended" || !last.IsError {
		t.Fatalf("last answer = %+v", last)
	}
	if !hasNote(last, "Stopped: 3 ") || !hasNote(last, " in a row were rejected") {
		t.Fatalf("last notes = %q", last.Notes)
	}
	if n := r.sessionRows(req); n != 0 {
		t.Fatalf("%d session rows remain after the run ended", n)
	}
	if n := r.tokenCount(req); n != 0 {
		t.Fatalf("%d run tokens remain after the run ended", n)
	}
}

func TestTwoStalledTurnsEndTheRunNoProgress(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	for n := range 5 {
		r.read(req, n)
	}
	r.call(req, "list_metrics", `{"q":"y"}`, nil, readOK)
	last := r.call(req, "list_metrics", `{"q":"z"}`, nil, readOK)
	row := r.stored(req)
	if row.State != string(revisions.StateFailed) || row.FailureCode.String != string(revisions.FailureNoProgress) {
		t.Fatalf("run = %s %q", row.State, row.FailureCode.String)
	}
	if row.Error.String != "the agent stopped making progress" || settledOf(last) != "ended" {
		t.Fatalf("error %q, answer %+v", row.Error.String, last)
	}
}

func TestSavesThatChangeNothingNewEndTheRunNoProgress(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	var last Answer
	for n := range 7 {
		last = r.call(req, "update_panels", `{"n":`+strconv.Itoa(n+1)+`}`, nil, buildSaved(r, "a", 9))
		if state := r.stored(req).State; state != string(revisions.StateRunning) {
			t.Fatalf("the run ended at save %d: %s", n+1, state)
		}
	}
	if !hasNote(last, "changed no panel that still needs work") {
		t.Fatalf("the churn note is missing: %q", last.Notes)
	}
	last = r.call(req, "update_panels", `{"n":8}`, nil, buildSaved(r, "a", 9))
	row := r.stored(req)
	if row.State != string(revisions.StateFailed) || row.FailureCode.String != string(revisions.FailureNoProgress) {
		t.Fatalf("run = %s %q", row.State, row.FailureCode.String)
	}
	if !hasNote(last, "Stopped: 4 saves in a row changed nothing new") {
		t.Fatalf("stop note missing: %q", last.Notes)
	}
}

func TestACallAfterTheRunEndedAnswersRunEndedWithoutRunningTheTool(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	for n := range guard.MaxRejectionsInRow {
		r.call(req, "update_panels", `{"n":`+strconv.Itoa(n+1)+`}`, nil, rejectedSave)
	}
	dispatched := &dispatchCount{}
	answer := r.call(req, "list_metrics", `{"q":"a"}`, nil, dispatched.wrap(readOK))
	if !answer.IsError || contentCode(t, answer) != "run_ended" || settledOf(answer) != "ended" || dispatched.n != 0 {
		t.Fatalf("answer %+v, dispatched %d", answer, dispatched.n)
	}
}

func TestAKeyedTurnSettlesWhenItsLastCallReturnsAndTheNudgeLandsOnThatCall(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	for n := range 4 {
		r.read(req, n)
	}
	turn := &Turn{ID: "t5", Calls: 2}
	first := r.call(req, "list_metrics", `{"q":"x"}`, turn, readOK)
	if hasNote(first, "Save the next panel") {
		t.Fatalf("the nudge landed on the first call of the turn: %q", first.Notes)
	}
	second := r.call(req, "list_metrics", `{"q":"y"}`, turn, readOK)
	if !hasNote(second, "Save the next panel") {
		t.Fatalf("the nudge is missing on the call that completes the turn: %q", second.Notes)
	}
}

func TestAKeyedTurnOfTwoCallsRefusesTheEleventhCallWhereKeylessRefusesTheSixth(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	for turn := range 5 {
		key := &Turn{ID: strconv.Itoa(turn), Calls: 2}
		for call := range 2 {
			q := strconv.Itoa(turn) + "-" + strconv.Itoa(call)
			if answer := r.call(req, "list_metrics", `{"q":"`+q+`"}`, key, readOK); answer.IsError {
				t.Fatalf("turn %d call %d was refused: %s", turn+1, call+1, answer.Content)
			}
		}
	}
	if got := r.call(req, "list_metrics", `{"q":"zz"}`, &Turn{ID: "z", Calls: 1}, readOK); !got.IsError || contentCode(t, got) != "invalid_request" {
		t.Fatalf("the sixth keyed turn was not refused: %s", got.Content)
	}
}

func TestTwoServicesOnOneDatabaseDecideAsOne(t *testing.T) {
	r := newRig(t)
	other := New(Config{DB: r.d.DB, Runs: r.runs, Claims: r.claims, Builds: r.store, Dashboards: r.store, Statuses: healthy{}, Steps: r.steps})
	req := r.revise()
	call := func(s *Service, q string) Answer {
		answer, err := s.Call(context.Background(), req, Call{Name: "list_metrics", Input: json.RawMessage(`{"q":"` + q + `"}`)}, readOK)
		if err != nil {
			t.Fatal(err)
		}
		return answer
	}
	services := []*Service{r.svc, other}
	for n := range 5 {
		call(services[n%2], strconv.Itoa(n))
	}
	if got := call(services[1], "z"); !got.IsError || contentCode(t, got) != "invalid_request" {
		t.Fatalf("the sixth call on the other service = %s", got.Content)
	}
}

func TestParentVersionStaysPinnedUntilTheBrief(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	scope, err := r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 1 || scope.Mode != tools.ModeRevise || !scope.Build {
		t.Fatalf("scope = %+v, %v", scope, err)
	}
	r.store.bump()
	scope, err = r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 1 {
		t.Fatalf("scope after a newer version = %+v, %v", scope, err)
	}
	if _, err := r.svc.Brief(context.Background(), req, ""); err != nil {
		t.Fatal(err)
	}
	scope, err = r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 2 {
		t.Fatalf("scope after the brief = %+v, %v", scope, err)
	}
}

func TestResultsCarryTheRunMeta(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	read := r.read(req, 0)
	if read.Meta.Settled != nil || read.Meta.Build != nil {
		t.Fatalf("read meta = %+v", read.Meta)
	}
	built := r.call(req, "update_panels", `{"n":1}`, nil, buildSaved(r, "a", 3))
	if built.Meta.Build == nil || *built.Meta.Build != (tools.BuildProgress{Saves: 1, Panels: 1, PlannedPanels: 3}) {
		t.Fatalf("build meta = %+v", built.Meta)
	}
}

func TestAFindingAndASuggestionSettleTheirRuns(t *testing.T) {
	r := newRig(t)
	investigation := r.investigate()
	finding := r.call(investigation, "save_finding", `{"x":1}`, nil, findingSaved)
	if settledOf(finding) != "finding" {
		t.Fatalf("finding meta = %+v", finding.Meta)
	}
	suggestion := r.suggest()
	suggested := r.call(suggestion, "update_panels", `{"x":1}`, nil, suggestionSaved)
	if settledOf(suggested) != "suggested" {
		t.Fatalf("suggestion meta = %+v", suggested.Meta)
	}
}

func TestAMalformedSaveResultEndsTheRun(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	answer := r.call(req, "update_panels", `{"n":1}`, nil, func(context.Context) (tools.Result, error) {
		return tools.Result{Content: map[string]any{}}, nil
	})
	row := r.stored(req)
	if row.FailureCode.String != string(revisions.FailureMalformedResult) || settledOf(answer) != "ended" {
		t.Fatalf("run %q, answer %+v", row.FailureCode.String, answer)
	}
}

func TestAToolErrorIsReturnedAndCountedAndTheTurnStillSettles(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	_, err := r.svc.Call(context.Background(), req, Call{Name: "list_metrics", Input: json.RawMessage(`{}`)}, func(context.Context) (tools.Result, error) {
		return tools.Result{}, context.DeadlineExceeded
	})
	if err == nil {
		t.Fatal("the tool error was swallowed")
	}
	if r.read(req, 1).IsError {
		t.Fatal("the next call was refused after a tool error")
	}
}

func TestDiscoveriesFeedTheBriefDigest(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.call(req, "list_metrics", `{"q":"a"}`, nil, func(context.Context) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"ok": true}, Discovered: &tools.Discovery{Datasource: "prom", Of: "metrics", Names: []string{"http_requests_total"}}}, nil
	})
	brief, err := r.svc.Brief(context.Background(), req, "renewed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(brief.Message, "http_requests_total") {
		t.Fatalf("the brief lacks the digest:\n%s", brief.Message)
	}
}
