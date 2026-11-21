package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const runID = "11111111-2222-3333-4444-555555555555"

type fakeRuns struct {
	raw     json.RawMessage
	err     error
	gotID   pgtype.UUID
	gotBy   auth.Principal
	gotMax  int
	invoked bool
	started *revisions.StartRequest
	retried bool

	pages       []json.RawMessage
	gotFilter   revisions.ListFilter
	gotLimits   []int
	gotCursor   string
	gotTitle    *string
	gotClear    bool
	gotPinned   *bool
	archivedID  pgtype.UUID
	updateCalls int
	chatFilter  revisions.ChatFilter
}

func (f *fakeRuns) Chats(_ context.Context, by auth.Principal, filter revisions.ChatFilter, limit int, cursor string) (json.RawMessage, error) {
	f.invoked, f.gotBy, f.chatFilter, f.gotCursor = true, by, filter, cursor
	f.gotLimits = append(f.gotLimits, limit)
	if len(f.pages) > 0 {
		page := f.pages[0]
		if len(f.pages) > 1 {
			f.pages = f.pages[1:]
		}
		return page, f.err
	}
	return f.raw, f.err
}

func (f *fakeRuns) Chat(_ context.Context, by auth.Principal, id pgtype.UUID) (json.RawMessage, error) {
	f.invoked, f.gotBy, f.gotID = true, by, id
	return f.raw, f.err
}

func (f *fakeRuns) List(_ context.Context, by auth.Principal, filter revisions.ListFilter, limit int, cursor string) (json.RawMessage, error) {
	f.invoked, f.gotBy, f.gotFilter, f.gotCursor = true, by, filter, cursor
	f.gotLimits = append(f.gotLimits, limit)
	if len(f.pages) > 0 {
		page := f.pages[0]
		if len(f.pages) > 1 {
			f.pages = f.pages[1:]
		}
		return page, f.err
	}
	return f.raw, f.err
}

func (f *fakeRuns) Update(_ context.Context, id pgtype.UUID, by auth.Principal, title *string, clearTitle bool, pinned *bool) (json.RawMessage, error) {
	f.invoked, f.gotID, f.gotBy, f.gotTitle, f.gotClear, f.gotPinned = true, id, by, title, clearTitle, pinned
	f.updateCalls++
	return f.raw, f.err
}

func (f *fakeRuns) Archive(_ context.Context, id pgtype.UUID, by auth.Principal) error {
	f.invoked, f.gotBy, f.archivedID = true, by, id
	return f.err
}

func (f *fakeRuns) Start(_ context.Context, by auth.Principal, req revisions.StartRequest) (json.RawMessage, error) {
	f.invoked, f.gotBy, f.started = true, by, &req
	return f.raw, f.err
}

func (f *fakeRuns) Build(_ context.Context, id pgtype.UUID, by auth.Principal) (json.RawMessage, error) {
	f.invoked, f.gotID, f.gotBy = true, id, by
	return f.raw, f.err
}

func (f *fakeRuns) Cancel(_ context.Context, id pgtype.UUID, by auth.Principal) (json.RawMessage, error) {
	f.invoked, f.gotID, f.gotBy = true, id, by
	return f.raw, f.err
}

func (f *fakeRuns) Retry(_ context.Context, id pgtype.UUID, by auth.Principal) (json.RawMessage, error) {
	f.invoked, f.gotID, f.gotBy, f.retried = true, id, by, true
	return f.raw, f.err
}

func (f *fakeRuns) Run(_ context.Context, id pgtype.UUID, by auth.Principal, maxSteps int) (json.RawMessage, error) {
	f.invoked, f.gotID, f.gotBy, f.gotMax = true, id, by, maxSteps
	return f.raw, f.err
}

func callGetRun(t *testing.T, runs RunDesk, role auth.Role, input string) (Result, error) {
	t.Helper()
	r := New(Deps{Runs: runs})
	by := auth.Principal{Kind: auth.KindUser, Name: "n", Role: role}
	return r.Call(auth.WithPrincipal(t.Context(), by), nameGetRun, []byte(input))
}

func TestGetRunRejectsABadInput(t *testing.T) {
	for name, input := range map[string]string{
		"missing id":  `{}`,
		"not a uuid":  `{"id":"nope"}`,
		"empty id":    `{"id":""}`,
		"unknown key": `{"id":"` + runID + `","extra":1}`,
		"wrong type":  `{"id":7}`,
	} {
		t.Run(name, func(t *testing.T) {
			runs := &fakeRuns{raw: json.RawMessage(`{}`)}
			res, err := callGetRun(t, runs, auth.RoleViewer, input)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != codeInvalidRequest {
				t.Fatalf("code %q", code)
			}
			if runs.invoked {
				t.Fatal("the reader was called with a bad input")
			}
		})
	}
}

func TestGetRunReturnsTheRawRunAndAsksForTwentySteps(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","state":"running","allowed_actions":["cancel"]}`)}
	res, err := callGetRun(t, runs, auth.RoleViewer, `{"id":"`+runID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("error result %+v", res)
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(runs.raw) {
		t.Fatalf("content %s, want %s", raw, runs.raw)
	}
	if runs.gotID.String() != runID || runs.gotMax != 20 || runs.gotBy.Role != auth.RoleViewer {
		t.Fatalf("reader got %v %v %d", runs.gotID, runs.gotBy, runs.gotMax)
	}
}

func TestGetRunCarriesTheLiveTextOfARunningRun(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","state":"running","live_text":"Checking the p99 of checkout."}`)}
	res, err := callGetRun(t, runs, auth.RoleViewer, `{"id":"`+runID+`"}`)
	if err != nil || res.IsError {
		t.Fatalf("result %+v, error %v", res, err)
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		LiveText string `json:"live_text"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || got.LiveText != "Checking the p99 of checkout." {
		t.Fatalf("live_text %q from %s, error %v", got.LiveText, raw, err)
	}
	if !strings.Contains(descGetRun, "live_text") {
		t.Fatalf("the get_run description does not name live_text: %s", descGetRun)
	}
}

func TestGetRunMapsErrors(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{revisions.ErrRequestNotFound, codeNotFound},
		{revisions.ErrNotFound, codeNotFound},
		{revisions.ErrDashboardNotFound, codeNotFound},
		{folders.ErrNotFound, codeNotFound},
		{fmt.Errorf("wrapped: %w", revisions.ErrRequestNotFound), codeNotFound},
		{folders.ErrForbidden, codeForbiddenRole},
		{folders.ErrNoAccess, codeForbiddenRole},
		{revisions.ErrForbidden, codeForbiddenRole},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			res, err := callGetRun(t, &fakeRuns{err: tc.err}, auth.RoleViewer, `{"id":"`+runID+`"}`)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != tc.want {
				t.Fatalf("code %q, want %q", code, tc.want)
			}
		})
	}
}

func TestGetRunReportsAnUnknownFailureAsAToolFailure(t *testing.T) {
	res, err := callGetRun(t, &fakeRuns{err: errors.New("database is down")}, auth.RoleViewer, `{"id":"`+runID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != FailureToolFailed {
		t.Fatalf("code %q", code)
	}
}

func TestGetRunWithoutAReaderIsAToolFailure(t *testing.T) {
	res, err := callGetRun(t, nil, auth.RoleViewer, `{"id":"`+runID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != FailureToolFailed {
		t.Fatalf("code %q", code)
	}
}

func callRunTool(t *testing.T, runs RunDesk, tool, input string) Result {
	t.Helper()
	r := New(Deps{Runs: runs})
	by := auth.Principal{Kind: auth.KindUser, Name: "n", Role: auth.RoleViewer}
	res, err := r.Call(auth.WithPrincipal(t.Context(), by), tool, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestStartRunPassesThePromptAndTheCallerAndReturnsTheRun(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","state":"queued"}`)}
	res := callRunTool(t, runs, nameStartRun, `{"prompt":"a latency dashboard","dashboard_id":"`+runID+`","base_version":2}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	got := runs.started
	if got == nil || *got.Prompt != "a latency dashboard" || *got.DashboardID != runID || *got.BaseVersion != 2 || got.Kind != nil {
		t.Fatalf("start request = %+v", got)
	}
	if runs.gotBy.Role != auth.RoleViewer {
		t.Fatalf("started as %+v", runs.gotBy)
	}
}

func TestStartRunPassesTheAnchorTheTurnIsAbout(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","state":"queued"}`)}
	res := callRunTool(t, runs, nameStartRun, `{"prompt":"why this spike","dashboard_id":"`+runID+`","anchor":{"kind":"position","pos":{"x":0.4,"y":0.3}}}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if got := runs.started; got == nil || got.Anchor == nil || got.Anchor.Kind != "position" || got.Anchor.Pos.X != 0.4 {
		t.Fatalf("start request = %+v", got)
	}
}

func TestStartRunPassesNewChat(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","state":"queued"}`)}
	res := callRunTool(t, runs, nameStartRun, `{"prompt":"a latency panel","kind":"revise","dashboard_id":"`+runID+`","new_chat":true}`)
	if res.IsError {
		t.Fatalf("error result %+v", res.Content)
	}
	if got := runs.started; got == nil || got.NewChat == nil || !*got.NewChat {
		t.Fatalf("start request = %+v", got)
	}
}

func TestStartRunHasNoWayToAskForAnInvestigation(t *testing.T) {
	runs := &fakeRuns{raw: json.RawMessage(`{}`)}
	res := callRunTool(t, runs, nameStartRun, `{"prompt":"why is it slow","kind":"investigate"}`)
	if code := errorCode(t, res); code != codeInvalidRequest || runs.invoked {
		t.Fatalf("code %q, invoked %v", code, runs.invoked)
	}
}

func TestRunToolsMapTheServicesRefusals(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"bad prompt":         {&revisions.StartError{Message: "prompt is required"}, codeInvalidRequest},
		"no model":           {revisions.ErrUnavailable, "agent_unavailable"},
		"too many revisions": {revisions.ErrTooManyRevisions, "too_many_revisions"},
		"too many requests":  {revisions.ErrTooManyRequests, "too_many_requests"},
		"busy":               {revisions.ErrAgentBusy, "agent_busy"},
		"not found":          {revisions.ErrRequestNotFound, codeNotFound},
		"not the requester":  {revisions.ErrNotRequester, "forbidden"},
		"no role":            {revisions.ErrForbidden, codeForbiddenRole},
		"assignee gone":      {revisions.ErrAssigneeUnavailable, "assignee_unavailable"},
		"wrong state":        {&lifecycle.Refusal{Machine: "run", State: "running", Action: "retry", Allowed: []string{"cancel"}}, codeInvalidTransition},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for tool, input := range map[string]string{
				nameStartRun:    `{"prompt":"x"}`,
				nameCancelRun:   `{"id":"` + runID + `"}`,
				nameGetRunBuild: `{"id":"` + runID + `"}`,
			} {
				res := callRunTool(t, &fakeRuns{err: tc.err}, tool, input)
				if code := errorCode(t, res); code != tc.want {
					t.Fatalf("%s: code %q, want %q", tool, code, tc.want)
				}
			}
		})
	}
}

func TestRetryRunRefusesAnInvestigationAndRetriesOtherRuns(t *testing.T) {
	investigation := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","kind":"investigate","investigation":true,"observation_id":"` + runID + `"}`)}
	res := callRunTool(t, investigation, nameRetryRun, `{"id":"`+runID+`"}`)
	if code := errorCode(t, res); code != threads.RefusalPersonStarts || investigation.retried {
		t.Fatalf("code %q, retried %v", code, investigation.retried)
	}
	build := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","kind":"build"}`)}
	if res := callRunTool(t, build, nameRetryRun, `{"id":"`+runID+`"}`); res.IsError || !build.retried {
		t.Fatalf("retry of a build run: %+v retried %v", res.Content, build.retried)
	}
	ask := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","kind":"investigate","investigation":false,"observation_id":null}`)}
	if res := callRunTool(t, ask, nameRetryRun, `{"id":"`+runID+`"}`); res.IsError || !ask.retried {
		t.Fatalf("retry of a private ask: %+v retried %v", res.Content, ask.retried)
	}
	saved := &fakeRuns{raw: json.RawMessage(`{"id":"` + runID + `","kind":"investigate","investigation":false,"observation_id":"` + runID + `"}`)}
	if res := callRunTool(t, saved, nameRetryRun, `{"id":"`+runID+`"}`); res.IsError || !saved.retried {
		t.Fatalf("retry of a private ask saved as an observation: %+v retried %v", res.Content, saved.retried)
	}
}

func TestRunToolsWithoutADeskAreToolFailures(t *testing.T) {
	for tool, input := range map[string]string{nameStartRun: `{"prompt":"x"}`, nameCancelRun: `{"id":"` + runID + `"}`, nameGetRunBuild: `{"id":"` + runID + `"}`, nameRetryRun: `{"id":"` + runID + `"}`} {
		r := New(Deps{})
		res, err := r.Call(auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, Role: auth.RoleViewer}), tool, []byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if code := errorCode(t, res); code != FailureToolFailed {
			t.Fatalf("%s: code %q", tool, code)
		}
	}
}
