package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func callClaimTool(t *testing.T, claimer *claims.Claimer, name, input string) (Result, error) {
	t.Helper()
	return callClaimToolWith(t, Deps{}, claimer, name, input)
}

func callClaimToolWith(t *testing.T, deps Deps, claimer *claims.Claimer, name, input string) (Result, error) {
	t.Helper()
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{Kind: auth.KindUser, ID: "u1", Name: "n", Role: auth.RoleViewer})
	if claimer != nil {
		ctx = claims.WithClaimer(ctx, *claimer)
	}
	return New(deps).Call(ctx, name, []byte(input))
}

type fakeClaims struct {
	req       revisions.Request
	runErr    error
	reports   []claims.ReportInput
	answers   []claims.ReportAnswer
	offer     claims.Offer
	claimedBy claims.Claimer
}

func (f *fakeClaims) Claim(_ context.Context, c claims.Claimer, _ time.Duration) (claims.Offer, error) {
	f.claimedBy = c
	return f.offer, nil
}

func (f *fakeClaims) ClaimedRun(context.Context, claims.Claimer, string, int) (revisions.Request, error) {
	return f.req, f.runErr
}

func (f *fakeClaims) Report(_ context.Context, _ claims.Claimer, in claims.ReportInput) (claims.ReportAnswer, error) {
	f.reports = append(f.reports, in)
	answer := f.answers[min(len(f.reports), len(f.answers))-1]
	return answer, nil
}

type fakeRunSessions struct {
	brief        BriefAnswer
	briefErr     error
	briefReasons []string
	stop         bool
	noted        []revisions.Usage
	finishErr    error
	finished     []RunFinish
	finishedAs   RunFinished
	progress     *BuildProgress
}

func (f *fakeRunSessions) Brief(_ context.Context, _ revisions.Request, reason string) (BriefAnswer, error) {
	f.briefReasons = append(f.briefReasons, reason)
	return f.brief, f.briefErr
}

func (f *fakeRunSessions) NoteUsage(_ context.Context, _ revisions.Request, u revisions.Usage) (bool, error) {
	f.noted = append(f.noted, u)
	return f.stop, nil
}

func (f *fakeRunSessions) Finish(_ context.Context, _ revisions.Request, in RunFinish) (RunFinished, error) {
	f.finished = append(f.finished, in)
	return f.finishedAs, f.finishErr
}

func (f *fakeRunSessions) Progress(context.Context, revisions.Request) (*BuildProgress, error) {
	return f.progress, nil
}

type fakeLiveText struct {
	runs     []string
	attempts []int
	texts    []string
	err      error
}

func (f *fakeLiveText) Append(_ context.Context, runID string, attempt int, text string) error {
	f.runs, f.attempts, f.texts = append(f.runs, runID), append(f.attempts, attempt), append(f.texts, text)
	return f.err
}

var outsideClaimer = &claims.Claimer{Kind: revisions.AssignGrant, ID: "g1", Person: "u1"}

func claimedRequest() revisions.Request {
	return revisions.Request{ID: runID, Attempt: 2}
}

func jsonOf(t *testing.T, res Result) map[string]any {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %#v", res.Content)
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestClaimToolsRejectABadInput(t *testing.T) {
	claimer := &claims.Claimer{Kind: revisions.AssignGrant, ID: "g1", Person: "u1"}
	longText := strings.Repeat("x", maxRunTextRunes+1)
	cases := map[string]struct{ tool, input string }{
		"wait above 30":          {nameClaimRun, `{"wait_seconds":31}`},
		"negative wait":          {nameClaimRun, `{"wait_seconds":-1}`},
		"fractional wait":        {nameClaimRun, `{"wait_seconds":1.5}`},
		"unknown claim field":    {nameClaimRun, `{"wait":5}`},
		"report without run":     {nameReportRun, `{"attempt":1}`},
		"report bad uuid":        {nameReportRun, `{"run_id":"nope","attempt":1}`},
		"report attempt zero":    {nameReportRun, `{"run_id":"` + runID + `","attempt":0}`},
		"report missing attempt": {nameReportRun, `{"run_id":"` + runID + `"}`},
		"report long note":       {nameReportRun, `{"run_id":"` + runID + `","attempt":1,"note":"` + longText + `"}`},
		"report long text":       {nameReportRun, `{"run_id":"` + runID + `","attempt":1,"text":"` + strings.Repeat("x", maxLiveTextRunes+1) + `"}`},
		"report unknown usage":   {nameReportRun, `{"run_id":"` + runID + `","attempt":1,"usage":{"tokens":1}}`},
		"brief bad uuid":         {nameGetRunBrief, `{"run_id":"nope","attempt":1}`},
		"brief attempt zero":     {nameGetRunBrief, `{"run_id":"` + runID + `","attempt":0}`},
		"brief unknown field":    {nameGetRunBrief, `{"run_id":"` + runID + `","attempt":1,"why":"x"}`},
		"report unknown field":   {nameReportRun, `{"run_id":"` + runID + `","attempt":1,"extra":1}`},
		"finish bad uuid":        {nameFinishRun, `{"run_id":"nope","attempt":1,"outcome":"succeeded"}`},
		"finish attempt zero":    {nameFinishRun, `{"run_id":"` + runID + `","attempt":0,"outcome":"succeeded"}`},
		"finish unknown outcome": {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"done"}`},
		"finish no outcome":      {nameFinishRun, `{"run_id":"` + runID + `","attempt":1}`},
		"finish long message":    {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","message":"` + longText + `"}`},
		"finish unknown field":   {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","why":"x"}`},
		"finish unknown usage":   {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","usage":{"tokens":1}}`},
		"finish bad usage type":  {nameFinishRun, `{"run_id":"` + runID + `","attempt":1,"outcome":"failed","usage":{"model":3}}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := callClaimTool(t, claimer, tc.tool, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != codeInvalidRequest {
				t.Fatalf("code %q, want %q", code, codeInvalidRequest)
			}
		})
	}
}

func TestClaimToolsRefuseACallerThatIsNotAClaimer(t *testing.T) {
	for tool, input := range map[string]string{
		nameClaimRun:    `{}`,
		nameReportRun:   `{"run_id":"` + runID + `","attempt":1}`,
		nameFinishRun:   `{"run_id":"` + runID + `","attempt":1,"outcome":"failed"}`,
		nameGetRunBrief: `{"run_id":"` + runID + `","attempt":1}`,
	} {
		t.Run(tool, func(t *testing.T) {
			res, err := callClaimTool(t, nil, tool, input)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != codeNotAWorker {
				t.Fatalf("code %q, want %q", code, codeNotAWorker)
			}
		})
	}
}

func TestClaimToolsNeedTheClaimsService(t *testing.T) {
	claimer := &claims.Claimer{Kind: revisions.AssignGrant, ID: "g1", Person: "u1"}
	_, err := callClaimTool(t, claimer, nameClaimRun, `{"wait_seconds":0}`)
	if !errors.Is(err, errClaimsNotConfigured) {
		t.Fatalf("err = %v, want errClaimsNotConfigured", err)
	}
}

func TestClaimErrorsBecomeToolErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"not a worker":    {claims.ErrNotAWorker, codeNotAWorker},
		"not your run":    {claims.ErrNotYourRun, codeNotYourRun},
		"invalid outcome": {claims.ErrInvalidOutcome, codeInvalidRequest},
		"invalid reason":  {ErrInvalidReason, codeInvalidRequest},
		"invalid failure": {ErrInvalidFailureCode, codeInvalidRequest},
		"commit failure":  {ErrCommitNeedsSuccess, codeInvalidRequest},
		"run ended":       {ErrRunEnded, codeRunEnded},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := claimFailure(tc.err)
			if err != nil {
				t.Fatal(err)
			}
			if code := errorCode(t, res); code != tc.code {
				t.Fatalf("code %q, want %q", code, tc.code)
			}
		})
	}
	boom := errors.New("boom")
	if _, err := claimFailure(boom); !errors.Is(err, boom) {
		t.Fatalf("an unknown error must be returned, got %v", err)
	}
}

func TestAFinishRefusedForRemovalsInProposeModeIsAToolErrorNamingThem(t *testing.T) {
	refusal := &dashboards.ProposalRequiredError{Removed: []spec.UnitChange{{Kind: spec.UnitPanel, ID: "a"}}}
	res, err := claimFailure(fmt.Errorf("finish: %w", refusal))
	if err != nil {
		t.Fatal(err)
	}
	body, ok := res.Content.(proposalRequiredOut)
	if !res.IsError || !ok || body.Code != codeProposalRequired || len(body.Removed) != 1 || body.Removed[0].ID != "a" {
		t.Fatalf("result = %+v", res)
	}
}

func TestClaimToolsAreMCPOnlyAndNeverOfferedInARun(t *testing.T) {
	byName := toolsByName(t)
	for _, name := range []string{nameClaimRun, nameReportRun, nameFinishRun, nameGetRunBrief} {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		for _, mode := range []Mode{ModeCreate, ModeRevise, ModeSuggest, ModeInvestigate} {
			if tool.OfferedIn(mode) {
				t.Errorf("%s is offered in %s runs", name, mode)
			}
		}
		if !tool.Claim {
			t.Errorf("%s is not a claim tool", name)
		}
	}
}

func TestOnlyTheFourClaimToolsAreClaimTools(t *testing.T) {
	var got []string
	for _, tool := range New(Deps{}).List() {
		if tool.Claim {
			got = append(got, tool.Name)
		}
	}
	if strings.Join(got, ",") != "claim_run,finish_run,get_run_brief,report_run" {
		t.Fatalf("claim tools = %v", got)
	}
}

func TestCallClaimServesOnlyClaimToolsAndNeedsNoPrincipal(t *testing.T) {
	deps := Deps{Claims: &fakeClaims{offer: claims.Offer{RetryAfterSeconds: 5}}}
	ctx := claims.WithClaimer(t.Context(), claims.Claimer{Kind: revisions.AssignBuiltin, ID: "server-1"})
	r := New(deps)

	res, err := r.CallClaim(ctx, nameClaimRun, []byte(`{"wait_seconds":0}`))
	if err != nil || res.IsError {
		t.Fatalf("claim_run without a principal = %#v, %v", res, err)
	}
	res, err = r.CallClaim(ctx, "list_folders", []byte(`{}`))
	if err != nil || errorCode(t, res) != codeUnknownTool {
		t.Fatalf("list_folders over the claim-only entry = %#v, %v", res, err)
	}
	res, err = r.Call(ctx, nameClaimRun, []byte(`{"wait_seconds":0}`))
	if err != nil || errorCode(t, res) != codeForbiddenRole {
		t.Fatalf("claim_run through Call without a principal = %#v, %v", res, err)
	}
}

func TestReportRunSendsTextUsageAndAnswersTheBuild(t *testing.T) {
	fc := &fakeClaims{req: claimedRequest(), answers: []claims.ReportAnswer{{State: "running"}}}
	rs := &fakeRunSessions{progress: &BuildProgress{Saves: 1, Panels: 2, PlannedPanels: 5}}
	live := &fakeLiveText{}
	deps := Deps{Claims: fc, RunSessions: rs, LiveText: live}
	input := `{"run_id":"` + strings.ToUpper(runID) + `","attempt":2,"note":"n","text":"thinking","usage":{"provider":"p","model":"m","input_tokens":10,"output_tokens":4,"cache_read_tokens":3,"cache_write_tokens":2,"cost_usd":0.5,"model_calls":3,"failed_calls":2,"retries":1,"compactions":2,"renewals":1,"model_ms":900,"max_save_output_tokens":77}}`

	res, err := callClaimToolWith(t, deps, outsideClaimer, nameReportRun, input)
	if err != nil {
		t.Fatal(err)
	}

	out := jsonOf(t, res)
	if out["state"] != "running" || out["stop"] != false || out["build"] == nil {
		t.Fatalf("answer = %v", out)
	}
	if live.runs[0] != runID || live.attempts[0] != 2 || live.texts[0] != "thinking" {
		t.Fatalf("live text = %v %v %v", live.runs, live.attempts, live.texts)
	}
	cost := 0.5
	want := revisions.Usage{Provider: "p", Model: "m", InputTokens: 10, OutputTokens: 4, CacheReadTokens: 3, CacheWriteTokens: 2, CostUSD: &cost, ModelCalls: 3, FailedCalls: 2, Retries: 1, Compactions: 2, Renewals: 1, ModelMS: 900, MaxSaveOutputTokens: 77}
	if len(rs.noted) != 1 || !reflect.DeepEqual(rs.noted[0], want) {
		t.Fatalf("noted usage = %+v", rs.noted)
	}
	if fc.reports[0].Note != "n" {
		t.Fatalf("report = %+v", fc.reports[0])
	}
}

func TestReportRunFromAnOutsideAgentThatSendsNoneOfTheNewFieldsIsAsBefore(t *testing.T) {
	fc := &fakeClaims{req: claimedRequest(), answers: []claims.ReportAnswer{{State: "running"}}}
	rs := &fakeRunSessions{}
	live := &fakeLiveText{}
	deps := Deps{Claims: fc, RunSessions: rs, LiveText: live}

	res, err := callClaimToolWith(t, deps, outsideClaimer, nameReportRun, `{"run_id":"`+runID+`","attempt":2}`)
	if err != nil {
		t.Fatal(err)
	}

	out := jsonOf(t, res)
	if out["state"] != "running" || out["stop"] != false || out["lease_until"] != nil || out["build"] != nil {
		t.Fatalf("answer = %v", out)
	}
	if len(rs.noted) != 0 || len(live.texts) != 0 {
		t.Fatalf("usage %v and text %v were sent without being asked", rs.noted, live.texts)
	}
}

func TestReportRunStopsBeforeUsageAndTextWhenTheRunHasEnded(t *testing.T) {
	fc := &fakeClaims{req: claimedRequest(), answers: []claims.ReportAnswer{{State: "canceling", Stop: true}}}
	rs := &fakeRunSessions{}
	live := &fakeLiveText{}

	res, err := callClaimToolWith(t, Deps{Claims: fc, RunSessions: rs, LiveText: live}, outsideClaimer, nameReportRun,
		`{"run_id":"`+runID+`","attempt":2,"text":"x","usage":{"model":"m"}}`)
	if err != nil {
		t.Fatal(err)
	}

	if out := jsonOf(t, res); out["stop"] != true || out["state"] != "canceling" {
		t.Fatalf("answer = %v", out)
	}
	if len(rs.noted) != 0 || len(live.texts) != 0 {
		t.Fatalf("usage %v and text %v were sent for an ended run", rs.noted, live.texts)
	}
}

func TestReportRunStopsWhenTheCostLimitEndedTheRunAndAnswersItsState(t *testing.T) {
	fc := &fakeClaims{req: claimedRequest(), answers: []claims.ReportAnswer{{State: "running"}, {State: "failed", Stop: true}}}
	rs := &fakeRunSessions{stop: true}
	live := &fakeLiveText{}

	res, err := callClaimToolWith(t, Deps{Claims: fc, RunSessions: rs, LiveText: live}, outsideClaimer, nameReportRun,
		`{"run_id":"`+runID+`","attempt":2,"text":"x","usage":{"cost_usd":99}}`)
	if err != nil {
		t.Fatal(err)
	}

	if out := jsonOf(t, res); out["stop"] != true || out["state"] != "failed" {
		t.Fatalf("answer = %v", out)
	}
	if len(live.texts) != 0 {
		t.Fatalf("live text %v was appended after the run ended", live.texts)
	}
}

func TestReportRunKeepsGoingWhenLiveTextFailsAndNeverLogsTheText(t *testing.T) {
	var logs bytes.Buffer
	fc := &fakeClaims{req: claimedRequest(), answers: []claims.ReportAnswer{{State: "running"}}}
	live := &fakeLiveText{err: errors.New("notify failed")}
	deps := Deps{Claims: fc, RunSessions: &fakeRunSessions{}, LiveText: live, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}

	res, err := callClaimToolWith(t, deps, outsideClaimer, nameReportRun, `{"run_id":"`+runID+`","attempt":2,"text":"secret model words"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v, err = %v", res, err)
	}

	if len(live.texts) != 1 || logs.Len() == 0 || strings.Contains(logs.String(), "secret model words") {
		t.Fatalf("texts %v, log %q", live.texts, logs.String())
	}
}

func TestFinishRunPassesTheLoopFieldsAndAnswersRestarted(t *testing.T) {
	rs := &fakeRunSessions{finishedAs: RunFinished{State: "running", Restarted: true}}
	deps := Deps{Claims: &fakeClaims{req: claimedRequest()}, RunSessions: rs}

	res, err := callClaimToolWith(t, deps, outsideClaimer, nameFinishRun,
		`{"run_id":"`+runID+`","attempt":2,"outcome":"succeeded","commit":true,"usage":{"model_calls":4,"retries":1,"compactions":2,"renewals":1,"model_ms":10,"max_save_output_tokens":5,"cache_read_tokens":6,"cache_write_tokens":7}}`)
	if err != nil {
		t.Fatal(err)
	}

	if out := jsonOf(t, res); out["restarted"] != true || out["state"] != "running" {
		t.Fatalf("answer = %v", out)
	}
	got := rs.finished[0]
	if got.Outcome != "succeeded" || !got.Commit || got.Usage == nil || got.Usage.ModelCalls != 4 || got.Usage.Retries != 1 || got.Usage.Compactions != 2 ||
		got.Usage.Renewals != 1 || got.Usage.ModelMS != 10 || got.Usage.MaxSaveOutputTokens != 5 || got.Usage.CacheReadTokens != 6 || got.Usage.CacheWriteTokens != 7 {
		t.Fatalf("finish = %+v usage %+v", got, got.Usage)
	}
}

func TestFinishRunWithAFailureCodeSendsNoMessageNeeded(t *testing.T) {
	rs := &fakeRunSessions{}
	deps := Deps{Claims: &fakeClaims{req: claimedRequest()}, RunSessions: rs}

	if _, err := callClaimToolWith(t, deps, outsideClaimer, nameFinishRun, `{"run_id":"`+runID+`","attempt":2,"outcome":"failed","failure_code":"rate_limited"}`); err != nil {
		t.Fatal(err)
	}

	if got := rs.finished[0]; got.FailureCode != "rate_limited" || got.Message != "" || got.Usage != nil || got.Commit {
		t.Fatalf("finish = %+v", got)
	}
}

func TestFinishRunTurnsASessionRefusalIntoAnInvalidArgument(t *testing.T) {
	for name, refusal := range map[string]error{"failure code": ErrInvalidFailureCode, "commit": ErrCommitNeedsSuccess} {
		deps := Deps{Claims: &fakeClaims{req: claimedRequest()}, RunSessions: &fakeRunSessions{finishErr: refusal}}

		res, err := callClaimToolWith(t, deps, outsideClaimer, nameFinishRun, `{"run_id":"`+runID+`","attempt":2,"outcome":"failed","failure_code":"conflict"}`)

		if err != nil || errorCode(t, res) != codeInvalidRequest {
			t.Errorf("%s: res = %#v, err = %v", name, res, err)
		}
	}
}

func TestClaimToolsOnARunThatIsNotTheClaimersAnswerNotYourRun(t *testing.T) {
	deps := Deps{Claims: &fakeClaims{runErr: claims.ErrNotYourRun}, RunSessions: &fakeRunSessions{}}
	for tool, input := range map[string]string{
		nameReportRun:   `{"run_id":"` + runID + `","attempt":2}`,
		nameFinishRun:   `{"run_id":"` + runID + `","attempt":2,"outcome":"failed"}`,
		nameGetRunBrief: `{"run_id":"` + runID + `","attempt":2}`,
	} {
		res, err := callClaimToolWith(t, deps, outsideClaimer, tool, input)
		if err != nil || errorCode(t, res) != codeNotYourRun {
			t.Errorf("%s: res = %#v, err = %v", tool, res, err)
		}
	}
}

func TestGetRunBriefAnswersTheBriefAndPassesTheReason(t *testing.T) {
	rs := &fakeRunSessions{brief: BriefAnswer{System: "sys", Message: "msg", Reason: "renewed", Build: &BuildProgress{Saves: 2, Panels: 2, PlannedPanels: 3}}}
	deps := Deps{Claims: &fakeClaims{req: claimedRequest()}, RunSessions: rs}

	res, err := callClaimToolWith(t, deps, outsideClaimer, nameGetRunBrief, `{"run_id":"`+runID+`","attempt":2,"reason":"renewed"}`)
	if err != nil {
		t.Fatal(err)
	}

	out := jsonOf(t, res)
	build, _ := out["build"].(map[string]any)
	if out["system"] != "sys" || out["message"] != "msg" || out["reason"] != "renewed" || build["planned_panels"] != float64(3) {
		t.Fatalf("answer = %v", out)
	}
	if rs.briefReasons[0] != "renewed" {
		t.Fatalf("reason = %q", rs.briefReasons[0])
	}
}

func TestGetRunBriefOfAFreshSessionLeavesTheReasonOutAndKeepsBuildNullWithoutOne(t *testing.T) {
	rs := &fakeRunSessions{brief: BriefAnswer{System: "s", Message: "m"}}
	deps := Deps{Claims: &fakeClaims{req: claimedRequest()}, RunSessions: rs}

	res, err := callClaimToolWith(t, deps, outsideClaimer, nameGetRunBrief, `{"run_id":"`+runID+`","attempt":2}`)
	if err != nil {
		t.Fatal(err)
	}

	if out := jsonOf(t, res); out["reason"] != "" || out["build"] != nil {
		t.Fatalf("answer = %v", out)
	}
	if rs.briefReasons[0] != "" {
		t.Fatalf("reason = %q", rs.briefReasons[0])
	}
}

func TestGetRunBriefAnswersABriefBeyondTheDefaultResultCapAndRefusesOneBeyondItsOwn(t *testing.T) {
	call := func(system, message int) Result {
		rs := &fakeRunSessions{brief: BriefAnswer{System: strings.Repeat("s", system), Message: strings.Repeat("m", message)}}
		deps := Deps{Claims: &fakeClaims{req: claimedRequest()}, RunSessions: rs}
		res, err := callClaimToolWith(t, deps, outsideClaimer, nameGetRunBrief, `{"run_id":"`+runID+`","attempt":2}`)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	if res := call(40<<10, 50<<10); res.IsError {
		t.Fatalf("a 90 KiB brief refused: %#v", res.Content)
	}
	if code := errorCode(t, call(maxBriefBytes, 1)); code != FailureResultTooLarge {
		t.Fatalf("code = %q, want %q", code, FailureResultTooLarge)
	}
}
