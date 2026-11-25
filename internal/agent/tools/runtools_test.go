package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	subjectID = "11111111-1111-1111-1111-111111111111"
	otherID   = "22222222-2222-2222-2222-222222222222"
	madeID    = "33333333-3333-3333-3333-333333333333"
	builtID   = "44444444-4444-4444-4444-444444444444"
)

func homeChat() Run {
	return Run{Mode: ModeCreate, Build: true, RequestID: otherID, ChatID: "chat"}
}

func threadAsk() Run {
	return Run{Mode: ModeInvestigate, RequestID: otherID, Observations: []string{subjectID}, ThreadAsk: true}
}

func privateChat() Run {
	return Run{Mode: ModeInvestigate, RequestID: otherID, ChatID: "chat", DashboardID: subjectID}
}

func violation(tool string, run Run, scope WriteScope, input string) string {
	return writeScopeViolation(tool, run, scope, writeTargets[tool](json.RawMessage(input)), func(string) bool { return false })
}

func TestScopeAllowsAWriteOnTheRunsSubject(t *testing.T) {
	for _, input := range []string{`{"observation_id":"` + subjectID + `","kind":"markdown"}`, `{"observation_id":"` + strings.ToUpper(subjectID) + `"}`} {
		if got := violation(nameAddBlock, threadAsk(), WriteScope{}, input); got != "" {
			t.Errorf("%s: %s", input, got)
		}
	}
}

func TestScopeRefusesAWriteOnAnObjectOutsideTheRun(t *testing.T) {
	got := violation(nameAddBlock, threadAsk(), WriteScope{}, `{"observation_id":"`+otherID+`"}`)
	if !strings.Contains(got, "add_block cannot run here") || !strings.Contains(got, "Add a block on the observation page") {
		t.Fatalf("message = %q", got)
	}
}

func TestScopeAllowsAnObjectTheRunCreatedAndTheBuildItCommitted(t *testing.T) {
	scope := WriteScope{Created: []string{madeID}, Committed: builtID}
	if got := violation(nameResolveObservation, homeChat(), scope, `{"observation_id":"`+madeID+`","summary":"s"}`); got != "" {
		t.Errorf("created: %s", got)
	}
	if got := violation(nameMoveDashboard, homeChat(), scope, `{"dashboard_id":"`+builtID+`"}`); got != "" {
		t.Errorf("committed: %s", got)
	}
	if got := violation(nameMoveDashboard, homeChat(), scope, `{"dashboard_id":"`+otherID+`"}`); got == "" {
		t.Error("an unrelated dashboard was allowed")
	}
}

func TestScopeAllowsABlockOrFolderOnlyWhenTheRunCreatedIt(t *testing.T) {
	if got := violation(nameDiscardBlock, threadAsk(), WriteScope{Created: []string{madeID}}, `{"block_id":"`+madeID+`"}`); got != "" {
		t.Errorf("own block: %s", got)
	}
	if got := violation(nameDiscardBlock, threadAsk(), WriteScope{}, `{"block_id":"`+madeID+`"}`); got == "" {
		t.Error("a block the run did not create was allowed")
	}
	if got := violation(nameRenameFolder, homeChat(), WriteScope{}, `{"folder_id":"`+madeID+`","title":"t"}`); got == "" {
		t.Error("a folder the run did not create was allowed")
	}
}

func TestScopeAllowsCreatingNewObjectsOutsideAPrivateAsk(t *testing.T) {
	for _, tool := range []string{nameCreateObservation, nameCreateFolder, nameSaveSearch} {
		if got := violation(tool, homeChat(), WriteScope{}, `{}`); got != "" {
			t.Errorf("%s: %s", tool, got)
		}
	}
}

func TestScopeRefusesEverySharedWriteInAPrivateAsk(t *testing.T) {
	for _, tool := range []string{nameCreateObservation, nameAddBlock, nameSaveRunAnswer, nameMoveDashboard} {
		got := violation(tool, privateChat(), WriteScope{Created: []string{subjectID}}, `{"observation_id":"`+subjectID+`","run_id":"`+otherID+`","dashboard_id":"`+subjectID+`"}`)
		if !strings.Contains(got, "private ask writes nothing shared") {
			t.Errorf("%s: %q", tool, got)
		}
	}
}

func TestScopeAllowsPersonStateOnlyInAChatRun(t *testing.T) {
	input := `{"dashboard_id":"` + subjectID + `"}`
	if got := violation(namePinDashboard, homeChat(), WriteScope{}, input); got != "" {
		t.Errorf("home chat: %s", got)
	}
	if got := violation(namePinDashboard, privateChat(), WriteScope{}, input); got != "" {
		t.Errorf("private ask: %s", got)
	}
	investigation := homeChat()
	investigation.Investigation = true
	for name, run := range map[string]Run{"thread ask": threadAsk(), "investigation": investigation, "revise": {Mode: ModeRevise, Build: true, DashboardID: subjectID}} {
		if got := violation(namePinDashboard, run, WriteScope{}, input); !strings.Contains(got, "only a chat run may do") {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestScopeRunTargetsAreTheRunsOwnChat(t *testing.T) {
	run := homeChat()
	input := `{"run_id":"` + otherID + `"}`
	if got := violation(nameSaveRunAnswer, run, WriteScope{}, input); got != "" {
		t.Errorf("own run: %s", got)
	}
	other := `{"run_id":"` + madeID + `"}`
	inChat := func(id string) bool { return id == madeID }
	if got := writeScopeViolation(nameSaveRunAnswer, run, WriteScope{}, writeTargets[nameSaveRunAnswer](json.RawMessage(other)), inChat); got != "" {
		t.Errorf("run of the chat: %s", got)
	}
	if got := violation(nameSaveRunAnswer, run, WriteScope{}, other); got == "" {
		t.Error("a run of another chat was allowed")
	}
	if got := violation(namePinRun, run, WriteScope{}, `{"id":"`+madeID+`"}`); got == "" {
		t.Error("pin_run on a run of another chat was allowed")
	}
}

func TestScopeRefusesAnAnnotationOnEveryDashboard(t *testing.T) {
	if got := violation(nameCreateAnnotation, homeChat(), WriteScope{}, `{"text":"deploy"}`); !strings.Contains(got, "every dashboard") {
		t.Errorf("message = %q", got)
	}
	if got := violation(nameCreateAnnotation, threadAsk(), WriteScope{}, `{"text":"deploy","dashboard_id":"`+otherID+`"}`); !strings.Contains(got, "only what it is about") {
		t.Errorf("annotation on another dashboard: %q", got)
	}
}

func TestScopeRefusesEveryWriteInASuggestRun(t *testing.T) {
	run := Run{Mode: ModeSuggest, RequestID: otherID, DashboardID: subjectID}
	if got := violation(nameMoveDashboard, run, WriteScope{}, `{"dashboard_id":"`+subjectID+`"}`); !strings.Contains(got, "suggest run") {
		t.Errorf("message = %q", got)
	}
}

func TestScopeChecksBothTargetsOfPinObservation(t *testing.T) {
	run := threadAsk()
	input := `{"observation_id":"` + subjectID + `","dashboard_id":"` + otherID + `"}`
	if got := violation(namePinObservation, run, WriteScope{}, input); got == "" {
		t.Error("pinning onto a dashboard outside the run was allowed")
	}
	if got := violation(namePinObservation, run, WriteScope{}, `{"observation_id":"`+subjectID+`"}`); got != "" {
		t.Errorf("subject only: %s", got)
	}
}

func TestScopePassesAMissingIDToTheHandlerToReject(t *testing.T) {
	if got := violation(nameAddBlock, threadAsk(), WriteScope{}, `{"kind":"markdown"}`); got != "" {
		t.Errorf("missing id: %s", got)
	}
	if got := violation(nameAddBlock, threadAsk(), WriteScope{}, `not json`); got != "" {
		t.Errorf("malformed input: %s", got)
	}
}

func TestWriteScopeTravelsInTheContext(t *testing.T) {
	scope := WriteScope{Created: []string{madeID}, Committed: builtID}
	got := WriteScopeFrom(WithWriteScope(context.Background(), scope))
	if !slices.Equal(got.Created, scope.Created) || got.Committed != builtID {
		t.Fatalf("scope = %+v", got)
	}
	if WriteScopeFrom(context.Background()).Committed != "" {
		t.Fatal("an empty context carried a scope")
	}
}

func TestEveryWriteToolOfferedToARunDeclaresItsScope(t *testing.T) {
	r := New(Deps{})
	for _, tool := range r.List() {
		if Phase(tool.Name) != PhaseWrite || tool.MCPOnly {
			continue
		}
		if writeTargets[tool.Name] == nil {
			t.Errorf("%s is offered in a run and has no write target", tool.Name)
		}
	}
}

func TestWriteTargetsNameRegisteredWriteToolsWithAWebAction(t *testing.T) {
	r := New(Deps{})
	registered := map[string]Tool{}
	for _, tool := range r.List() {
		registered[tool.Name] = tool
	}
	for name := range writeTargets {
		tool, ok := registered[name]
		if !ok || Phase(name) != PhaseWrite {
			t.Errorf("%s is not a registered write tool", name)
		}
		if webActions[name] == "" {
			t.Errorf("%s has no web action", name)
		}
		if tool.PersonOnly && !personStateWrites[name] {
			t.Errorf("%s is person only and not person state", name)
		}
	}
	for name := range personStateWrites {
		if writeTargets[name] == nil {
			t.Errorf("%s is person state without a write target", name)
		}
	}
	if len(writeTargets) != 32 {
		t.Errorf("write targets = %d, want 32", len(writeTargets))
	}
}

func TestACallInsideARunStopsAtItsWriteScopeBeforeTheHandler(t *testing.T) {
	r := New(Deps{})
	ctx := WithRun(context.Background(), threadAsk())
	res, ok := r.withinWriteScope(ctx, writeTool(r, nameAddBlock), threadAsk(), true, json.RawMessage(`{"observation_id":"`+otherID+`"}`))
	eb, isBody := res.Content.(errorBody)
	if ok || !res.IsError || !isBody || eb.Code != codeOutOfScope || res.Step != "Refused add_block: out_of_scope" {
		t.Fatalf("res = %+v ok = %v", res, ok)
	}
	if _, ok := r.withinWriteScope(ctx, writeTool(r, nameAddBlock), threadAsk(), true, json.RawMessage(`{"observation_id":"`+subjectID+`"}`)); !ok {
		t.Fatal("a write on the subject was refused")
	}
	if _, ok := r.withinWriteScope(ctx, writeTool(r, nameGetPanels), threadAsk(), true, json.RawMessage(`{}`)); !ok {
		t.Fatal("a read was checked against the write scope")
	}
}

func writeTool(r *Registry, name string) Tool {
	tool := r.entries[name].tool
	tool.MCPOnly = false
	return tool
}

func TestOnlyAChatAskInInvestigateModeIsPrivate(t *testing.T) {
	apply := Run{Mode: ModeRevise, Build: true, RequestID: otherID, ChatID: "chat", DashboardID: subjectID}
	threadWithChat := threadAsk()
	threadWithChat.ChatID = "chat"
	investigation := privateChat()
	investigation.Investigation = true
	cases := map[string]struct {
		run  Run
		want bool
	}{
		"chat ask on a dashboard":  {privateChat(), true},
		"home chat":                {homeChat(), false},
		"apply from a chat":        {apply, false},
		"thread ask with a chat":   {threadWithChat, false},
		"investigation from chat":  {investigation, false},
		"ask on an observation":    {Run{Mode: ModeInvestigate, ChatID: "chat", Observations: []string{subjectID}}, true},
		"investigate without chat": {Run{Mode: ModeInvestigate, DashboardID: subjectID}, false},
	}
	for name, tc := range cases {
		if got := tc.run.privateAsk(); got != tc.want {
			t.Errorf("%s: privateAsk = %v, want %v", name, got, tc.want)
		}
	}
	if got := violation(nameMoveDashboard, apply, WriteScope{}, `{"dashboard_id":"`+subjectID+`"}`); got != "" {
		t.Errorf("a write on the subject of an apply run from a chat: %s", got)
	}
	if got := violation(nameAddBlock, threadWithChat, WriteScope{}, `{"observation_id":"`+subjectID+`"}`); got != "" {
		t.Errorf("a write on the subject of a thread ask with a chat id: %s", got)
	}
}

func TestScopeRefusesACreatingWriteOnceTheRunTracksTheMostObjects(t *testing.T) {
	created := make([]string, MaxCreatedIDs)
	for i := range created {
		created[i] = "id-" + strconv.Itoa(i)
	}
	scope := WriteScope{Created: created}
	if got := violation(nameCreateFolder, homeChat(), scope, `{"title":"t"}`); !strings.Contains(got, "already created 256 objects") {
		t.Errorf("creating write: %q", got)
	}
	if got := violation(nameAddBlock, threadAsk(), scope, `{"observation_id":"`+subjectID+`"}`); !strings.Contains(got, "already created 256 objects") {
		t.Errorf("add_block: %q", got)
	}
	if got := violation(nameResolveObservation, threadAsk(), scope, `{"observation_id":"`+subjectID+`"}`); got != "" {
		t.Errorf("a write that creates nothing: %s", got)
	}
}

func TestEveryWriteToolHasARunScopeOrIsLeftOutOfRuns(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if Phase(tool.Name) != PhaseWrite {
			continue
		}
		scoped, leftOut := writeTargets[tool.Name] != nil, leftOutOfRuns[tool.Name] != ""
		switch {
		case scoped && leftOut:
			t.Errorf("%s has a run scope and is listed as left out of runs", tool.Name)
		case !scoped && !leftOut:
			t.Errorf("%s writes and has neither a run scope nor a place in the left out list", tool.Name)
		case leftOut && !tool.MCPOnly:
			t.Errorf("%s is left out of runs and is not MCP only", tool.Name)
		}
	}
}
