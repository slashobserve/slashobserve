package tools

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func chatRow(id string) string {
	return `{"id":"` + id + `","title":"Checkout","dashboard_id":null,"observation_id":null,"pinned_at":null,"created_at":"2026-11-01T10:00:00Z","last_turn_at":"2026-11-01T10:00:00Z","turns":2,"last_turn":{"id":"` + id + `","state":"succeeded","prompt":"` + strings.Repeat("p", 3000) + `"}}`
}

func turnRow(i int) string {
	return `{"id":"` + runID + `","n":` + strconv.Itoa(i) + `,"prompt":"` + strings.Repeat("t", 5000) + `"}`
}

func TestListChatsPassesTheFilterAndAnswersAPageThatFits(t *testing.T) {
	rows := make([]string, 8)
	for i := range rows {
		rows[i] = chatRow(runID)
	}
	runs := &fakeRuns{pages: []json.RawMessage{
		json.RawMessage(`{"chats":[` + strings.Join(rows, ",") + `],"next_cursor":"c2"}`),
		json.RawMessage(`{"chats":[` + strings.Join(rows[:4], ",") + `],"next_cursor":"c3"}`),
	}}
	res := callRunTool(t, runs, nameListChats, `{"q":" checkout ","pinned":true,"dashboard_id":"`+runID+`","observation_id":"`+runID+`","limit":8,"cursor":"c1"}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	got := runs.chatFilter
	if got.Query != "checkout" || got.Pinned == nil || !*got.Pinned || got.DashboardID.String() != runID || got.ObservationID.String() != runID || runs.gotCursor != "c1" {
		t.Fatalf("filter = %+v cursor %q", got, runs.gotCursor)
	}
	if len(runs.gotLimits) != 2 || runs.gotLimits[0] != 8 || runs.gotLimits[1] >= 8 {
		t.Fatalf("limits = %v, want a second smaller page", runs.gotLimits)
	}
	var out struct {
		Chats      []json.RawMessage `json:"chats"`
		NextCursor string            `json:"next_cursor"`
	}
	if err := json.Unmarshal(contentJSON(t, res), &out); err != nil || len(out.Chats) != 4 || out.NextCursor != "c3" {
		t.Fatalf("out = %d chats, cursor %q, err %v", len(out.Chats), out.NextCursor, err)
	}
}

func TestListChatsDefaultsToTwentyAndRejectsABadInput(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"chats":null,"next_cursor":null}`)}
	res := callRunTool(t, runs, nameListChats, `{}`)
	if res.IsError || runs.gotLimits[0] != 20 || runs.chatFilter != (revisions.ChatFilter{}) || !strings.Contains(string(contentJSON(t, res)), `"chats":[]`) {
		t.Fatalf("res = %+v limits %v filter %+v", res, runs.gotLimits, runs.chatFilter)
	}
	for _, input := range []string{`{"limit":51}`, `{"observation_id":"x"}`, `{"dashboard_id":"x"}`, `{"q":""}`, `{"scope":"everyone"}`} {
		bad := &fakeRuns{}
		if code := errorCode(t, callRunTool(t, bad, nameListChats, input)); code != codeInvalidRequest || bad.invoked {
			t.Errorf("%s: code %q, invoked %v", input, code, bad.invoked)
		}
	}
}

func TestListChatsAsksForEveryonesChatsWithScopeAll(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"chats":[],"next_cursor":null}`)}
	if res := callRunTool(t, runs, nameListChats, `{"scope":"all"}`); res.IsError || !runs.chatFilter.Everyone {
		t.Fatalf("res = %+v filter %+v", res, runs.chatFilter)
	}
	refused := &fakeRuns{err: revisions.ErrForbidden}
	if code := errorCode(t, callRunTool(t, refused, nameListChats, `{"scope":"all"}`)); code != "forbidden_role" {
		t.Errorf("a non-Admin asking for everyone's chats: code %q, want forbidden_role", code)
	}
}

func TestGetChatPagesTheTurnsByOffset(t *testing.T) {
	turns := make([]string, 5)
	for i := range turns {
		turns[i] = turnRow(i)
	}
	runs := &fakeRuns{raw: json.RawMessage(`{"chat":` + chatRow(runID) + `,"turns":[` + strings.Join(turns, ",") + `]}`)}
	var first struct {
		Turns      []struct{ N int } `json:"turns"`
		Truncated  bool              `json:"truncated"`
		NextOffset *int              `json:"next_offset"`
	}
	res := callRunTool(t, runs, nameGetChat, `{"id":"`+runID+`"}`)
	if err := json.Unmarshal(contentJSON(t, res), &first); err != nil || runs.gotID.String() != runID {
		t.Fatalf("res = %+v err %v", res, err)
	}
	if !first.Truncated || first.NextOffset == nil || *first.NextOffset != len(first.Turns) || len(first.Turns) == 0 || first.Turns[0].N != 0 {
		t.Fatalf("first page = %+v", first)
	}
	var last struct {
		Turns     []struct{ N int } `json:"turns"`
		Truncated bool              `json:"truncated"`
	}
	res = callRunTool(t, runs, nameGetChat, `{"id":"`+runID+`","offset":4}`)
	if err := json.Unmarshal(contentJSON(t, res), &last); err != nil || last.Truncated || len(last.Turns) != 1 || last.Turns[0].N != 4 {
		t.Fatalf("last page = %+v err %v", last, err)
	}
}

func TestGetChatGivesATurnLargerThanTheBudgetOnItsOwn(t *testing.T) {
	big := `{"id":"` + runID + `","prompt":"` + strings.Repeat("x", listBudget-1000) + `"}`
	runs := &fakeRuns{raw: json.RawMessage(`{"chat":{"id":"` + runID + `","title":"` + strings.Repeat("c", 1500) + `"},"turns":[` + big + `,` + big + `]}`)}
	var out struct {
		Turns      []json.RawMessage `json:"turns"`
		NextOffset *int              `json:"next_offset"`
	}
	if err := json.Unmarshal(contentJSON(t, callRunTool(t, runs, nameGetChat, `{"id":"`+runID+`"}`)), &out); err != nil || len(out.Turns) != 1 || out.NextOffset == nil || *out.NextOffset != 1 {
		t.Fatalf("out = %d turns, next %v, err %v", len(out.Turns), out.NextOffset, err)
	}
}

func TestGetChatAnswersNotFoundForAChatThatIsNotYours(t *testing.T) {
	runs := &fakeRuns{err: revisions.ErrChatNotFound}
	if code := errorCode(t, callRunTool(t, runs, nameGetChat, `{"id":"`+runID+`"}`)); code != codeNotFound {
		t.Fatalf("code = %q", code)
	}
	if code := errorCode(t, callRunTool(t, &fakeRuns{}, nameGetChat, `{"id":"x"}`)); code != codeInvalidRequest {
		t.Fatalf("bad id code = %q", code)
	}
}

func TestStartRunInAChatOnlyAnOwnerMayAskInAnswersReadOnlyNotNotFound(t *testing.T) {
	runs := &fakeRuns{err: revisions.ErrChatReadOnly}
	res := callRunTool(t, runs, nameStartRun, `{"prompt":"and now?","chat_id":"`+runID+`"}`)
	if code := errorCode(t, res); code != revisions.CodeChatReadOnly {
		t.Fatalf("code = %q, want %q", code, revisions.CodeChatReadOnly)
	}
}

func TestChatToolsAreReadOnlyRunToolsAndNeedAViewer(t *testing.T) {
	byName := map[string]Tool{}
	for _, tool := range New(Deps{}).List() {
		byName[tool.Name] = tool
	}
	for _, name := range []string{nameListChats, nameGetChat} {
		tool := byName[name]
		if tool.MCPOnly || !tool.OfferedIn(ModeCreate) || !tool.ReadOnly || tool.Destructive || tool.MinRole != auth.RoleViewer || tool.PersonOnly {
			t.Errorf("%s = %+v", name, tool)
		}
	}
}

func TestStartRunPassesTheChatAndTheClientID(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","state":"queued","replayed":true}`)}
	res := callRunTool(t, runs, nameStartRun, `{"prompt":"and now?","chat_id":"`+runID+`","client_id":"`+runID+`"}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if got := runs.started; got == nil || got.ChatID == nil || *got.ChatID != runID || got.ClientID == nil || *got.ClientID != runID {
		t.Fatalf("start request = %+v", got)
	}
	if !strings.Contains(res.Step, "already started") {
		t.Fatalf("step = %q", res.Step)
	}
	reused := &fakeRuns{err: revisions.ErrClientIDReused}
	if code := errorCode(t, callRunTool(t, reused, nameStartRun, `{"prompt":"x","client_id":"`+runID+`"}`)); code != "client_id_reused" {
		t.Fatalf("reused code = %q", code)
	}
}
