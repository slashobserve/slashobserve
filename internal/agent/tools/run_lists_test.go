package tools

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func runRow(id, title string) string {
	return `{"id":"` + id + `","kind":"build","intent":"apply","trigger":"person","title":"` + title + `","pinned_at":null,"prompt":"` + strings.Repeat("long prompt ", 300) + `","status":"succeeded","state":"succeeded","result_kind":"version","dashboard_id":"` + runID + `","dashboard_title":"Checkout","observation_id":null,"observation_title":null,"chat_id":"` + id + `","created_at":"2026-11-01T10:00:00Z","requested_by":{"kind":"user","id":"` + runID + `","name":"Ada"},"allowed_actions":["retry"]}`
}

func listPage(rows []string, next string, total int) json.RawMessage {
	cursor := "null"
	if next != "" {
		cursor = `"` + next + `"`
	}
	return json.RawMessage(`{"requests":[` + strings.Join(rows, ",") + `],"next_cursor":` + cursor + `,"summary":{"total":` + strconv.Itoa(total) + `,"people":null}}`)
}

func TestListRunsPassesTheFilterAndAnswersACompactPage(t *testing.T) {
	runs := &fakeRuns{raw: listPage([]string{runRow(runID, "Checkout health")}, "c2", 7)}
	res := callRunTool(t, runs, nameListRuns, `{"scope":"all","state":["failed","canceled"],"q":"  checkout ","pinned":false,"dashboard_id":"`+runID+`","limit":5,"cursor":"c1"}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	got := runs.gotFilter
	if got.Scope != revisions.ScopeAll || got.Query != "checkout" || got.Pinned == nil || *got.Pinned || got.DashboardID.String() != runID ||
		len(got.States) != 2 || got.States[0] != revisions.StateFailed || got.States[1] != revisions.StateCanceled {
		t.Fatalf("filter = %+v", got)
	}
	if runs.gotLimits[0] != 5 || runs.gotCursor != "c1" {
		t.Fatalf("limit %v, cursor %q", runs.gotLimits, runs.gotCursor)
	}
	body := string(contentJSON(t, res))
	if strings.Contains(body, "long prompt") || !strings.Contains(body, `"next_cursor":"c2"`) || !strings.Contains(body, `"title":"Checkout health"`) || !strings.Contains(body, `"allowed_actions":["retry"]`) || !strings.Contains(body, `"intent":"apply"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestListRunsKeepsAnchoredRunsAboutAnObservationAndShowsTheSpot(t *testing.T) {
	row := strings.Replace(runRow(runID, "Spot"), `"observation_id":null`, `"observation_id":null,"anchor":{"kind":"row","pos":{"x":0.4,"y":0.3},"row":{"service":"payments"},"shown":"440"},"block_id":"`+runID+`"`, 1)
	runs := &fakeRuns{raw: listPage([]string{row, runRow(runID, "Plain")}, "", 2)}
	res := callRunTool(t, runs, nameListRuns, `{"anchored":true,"observation_id":"`+runID+`"}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if !runs.gotFilter.Anchored || runs.gotFilter.ObservationID.String() != runID || runs.gotFilter.Scope != revisions.ScopeMine {
		t.Fatalf("filter = %+v", runs.gotFilter)
	}
	body := string(contentJSON(t, res))
	if !strings.Contains(body, `"anchor":{"kind":"row","pos":{"x":0.4,"y":0.3},"row":{"service":"payments"},"shown":"440"}`) || !strings.Contains(body, `"block_id":"`+runID+`"`) || !strings.Contains(body, `"anchor":null,"block_id":null`) {
		t.Fatalf("body = %s", body)
	}
	if code := errorCode(t, callRunTool(t, &fakeRuns{raw: listPage(nil, "", 0)}, nameListRuns, `{"observation_id":"nope"}`)); code != codeInvalidRequest {
		t.Fatalf("code %q", code)
	}
	refused := &fakeRuns{err: revisions.ErrAnchoredNeedsMine}
	if code := errorCode(t, callRunTool(t, refused, nameListRuns, `{"anchored":true,"scope":"all"}`)); code != codeInvalidRequest {
		t.Fatalf("code %q", code)
	}
}

func TestListRunsDefaultsToYourOwnRunsAndTwentyAPage(t *testing.T) {
	runs := &fakeRuns{raw: listPage(nil, "", 0)}
	res := callRunTool(t, runs, nameListRuns, `{}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if runs.gotFilter.Scope != revisions.ScopeMine || runs.gotLimits[0] != 20 {
		t.Fatalf("filter %+v, limits %v", runs.gotFilter, runs.gotLimits)
	}
	if !strings.Contains(string(contentJSON(t, res)), `"runs":[]`) {
		t.Fatalf("body = %s", contentJSON(t, res))
	}
}

func TestListRunsRejectsABadInput(t *testing.T) {
	for name, input := range map[string]string{
		"scope":        `{"scope":"everyone"}`,
		"source":       `{"source":"prompt"}`,
		"state":        `{"state":["done"]}`,
		"empty q":      `{"q":"   "}`,
		"long q":       `{"q":"` + strings.Repeat("a", 201) + `"}`,
		"dashboard id": `{"dashboard_id":"nope"}`,
		"limit":        `{"limit":51}`,
		"unknown":      `{"extra":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			runs := &fakeRuns{raw: listPage(nil, "", 0)}
			res := callRunTool(t, runs, nameListRuns, input)
			if code := errorCode(t, res); code != codeInvalidRequest {
				t.Fatalf("code %q", code)
			}
			if runs.invoked {
				t.Fatal("the list was read with a bad input")
			}
		})
	}
}

func TestListRunsAnswersAnInvalidCursorAndAnAdminOnlyScope(t *testing.T) {
	cursor := &fakeRuns{err: revisions.ErrInvalidCursor}
	if code := errorCode(t, callRunTool(t, cursor, nameListRuns, `{"cursor":"x"}`)); code != codeInvalidRequest {
		t.Fatalf("code %q", code)
	}
	denied := &fakeRuns{err: revisions.ErrForbidden}
	if code := errorCode(t, callRunTool(t, denied, nameListRuns, `{"scope":"all"}`)); code != codeForbiddenRole {
		t.Fatalf("code %q", code)
	}
}

func TestListRunsAsksAgainForFewerRunsWhenThePageIsTooBig(t *testing.T) {
	rows := make([]string, 50)
	for i := range rows {
		rows[i] = runRow(runID, strings.Repeat("t", 100))
	}
	runs := &fakeRuns{pages: []json.RawMessage{listPage(rows, "more", 0), listPage(rows[:20], "more", 0)}}
	res := callRunTool(t, runs, nameListRuns, `{"limit":50}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if len(runs.gotLimits) < 2 || runs.gotLimits[0] != 50 || runs.gotLimits[1] >= 50 {
		t.Fatalf("limits asked %v", runs.gotLimits)
	}
	if size := jsonSize(res.Content); size > listBudget {
		t.Fatalf("answer is %d bytes, over the %d budget", size, listBudget)
	}
}

func TestRenameRunSendsTheTitleOrClearsIt(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","title":"Checkout status"}`)}
	res := callRunTool(t, runs, nameRenameRun, `{"id":"`+runID+`","title":"  Checkout status "}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if runs.gotID.String() != runID || runs.gotTitle == nil || *runs.gotTitle != "Checkout status" || runs.gotClear || runs.gotPinned != nil || runs.gotBy.Role != auth.RoleViewer {
		t.Fatalf("update = id %s title %v clear %v pinned %v by %+v", runs.gotID, runs.gotTitle, runs.gotClear, runs.gotPinned, runs.gotBy)
	}
	cleared := &fakeRuns{raw: json.RawMessage(`{}`)}
	if res := callRunTool(t, cleared, nameRenameRun, `{"run_id":"`+runID+`","title":null}`); res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if cleared.gotTitle != nil || !cleared.gotClear {
		t.Fatalf("title %v, clear %v", cleared.gotTitle, cleared.gotClear)
	}
}

func TestRenameRunRejectsABadTitle(t *testing.T) {
	for name, input := range map[string]string{
		"missing title": `{"id":"` + runID + `"}`,
		"empty":         `{"id":"` + runID + `","title":"  "}`,
		"too long":      `{"id":"` + runID + `","title":"` + strings.Repeat("a", 121) + `"}`,
		"two lines":     `{"id":"` + runID + `","title":"a\nb"}`,
		"a number":      `{"id":"` + runID + `","title":7}`,
		"bad id":        `{"id":"nope","title":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			runs := &fakeRuns{raw: json.RawMessage(`{}`)}
			res := callRunTool(t, runs, nameRenameRun, input)
			if code := errorCode(t, res); code != codeInvalidRequest {
				t.Fatalf("code %q", code)
			}
			if runs.invoked {
				t.Fatal("the run was changed with a bad input")
			}
		})
	}
}

func TestPinAndUnpinRunSetThePin(t *testing.T) {
	for tool, want := range map[string]bool{namePinRun: true, nameUnpinRun: false} {
		t.Run(tool, func(t *testing.T) {
			runs := &fakeRuns{raw: json.RawMessage(`{}`)}
			res := callRunTool(t, runs, tool, `{"id":"`+runID+`"}`)
			if res.IsError {
				t.Fatalf("error result %+v", res.Content)
			}
			if runs.gotPinned == nil || *runs.gotPinned != want || runs.gotTitle != nil || runs.gotClear {
				t.Fatalf("pinned %v title %v clear %v", runs.gotPinned, runs.gotTitle, runs.gotClear)
			}
		})
	}
}

func TestArchiveRunArchivesTheRun(t *testing.T) {
	runs := &fakeRuns{}
	res := callRunTool(t, runs, nameArchiveRun, `{"id":"`+runID+`"}`)
	if res.IsError || runs.archivedID.String() != runID {
		t.Fatalf("result %+v, archived %s", res.Content, runs.archivedID)
	}
	if body := string(contentJSON(t, res)); body != `{"id":"`+runID+`","archived":true}` {
		t.Fatalf("body = %s", body)
	}
}

func TestChangingARunIsRefusedForSomeoneElsesAndAnUnknownRun(t *testing.T) {
	for tool, input := range map[string]string{
		nameRenameRun:  `{"id":"` + runID + `","title":"x"}`,
		namePinRun:     `{"id":"` + runID + `"}`,
		nameUnpinRun:   `{"id":"` + runID + `"}`,
		nameArchiveRun: `{"id":"` + runID + `"}`,
	} {
		t.Run(tool, func(t *testing.T) {
			other := &fakeRuns{err: revisions.ErrNotRequester}
			if code := errorCode(t, callRunTool(t, other, tool, input)); code != "forbidden" {
				t.Fatalf("code %q", code)
			}
			missing := &fakeRuns{err: revisions.ErrRequestNotFound}
			if code := errorCode(t, callRunTool(t, missing, tool, input)); code != codeNotFound {
				t.Fatalf("code %q", code)
			}
			broken := &fakeRuns{err: errors.New("boom")}
			if code := errorCode(t, callRunTool(t, broken, tool, input)); code != FailureToolFailed {
				t.Fatalf("code %q", code)
			}
		})
	}
}

func TestRunListToolsNeedRuns(t *testing.T) {
	r := New(Deps{})
	for tool, input := range map[string]string{
		nameListRuns:   `{}`,
		nameRenameRun:  `{"id":"` + runID + `","title":"x"}`,
		namePinRun:     `{"id":"` + runID + `"}`,
		nameUnpinRun:   `{"id":"` + runID + `"}`,
		nameArchiveRun: `{"id":"` + runID + `"}`,
	} {
		res, err := r.Call(auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer}), tool, []byte(input))
		if err != nil || errorCode(t, res) != FailureToolFailed {
			t.Errorf("%s: res = %+v, err = %v", tool, res, err)
		}
	}
}

func TestRunListToolsAreRunToolsAndNeedAViewer(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	readOnly := map[string]bool{nameListRuns: true, nameRenameRun: false, namePinRun: false, nameUnpinRun: false, nameArchiveRun: false}
	for name, wantReadOnly := range readOnly {
		tool := byName[name]
		if tool.MCPOnly || !tool.OfferedIn(ModeCreate) || tool.MinRole != auth.RoleViewer || tool.ReadOnly != wantReadOnly || tool.Destructive != (name == nameRenameRun) || tool.PersonOnly {
			t.Errorf("%s = %+v", name, tool)
		}
	}
}

func TestRunChangeDescriptionsNameBothRefusals(t *testing.T) {
	for name, desc := range map[string]string{
		nameRenameRun:  descRenameRun,
		namePinRun:     descPinRun,
		nameUnpinRun:   descUnpinRun,
		nameArchiveRun: descArchiveRun,
		nameCancelRun:  descCancelRun,
	} {
		if !strings.Contains(desc, "forbidden on a run they can see on a dashboard and not_found on any other") {
			t.Errorf("%s does not say when it answers forbidden and when not_found: %s", name, desc)
		}
	}
}
