package runsession

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/brief"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/llm/pricing"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func floatPtr(v float64) *float64 { return &v }

func (r *rig) startBuild(req revisions.Request, panel string) {
	r.t.Helper()
	r.call(req, "update_panels", `{"panel":"`+panel+`"}`, nil, buildSaved(r, panel, 3))
}

func TestAConflictRebasesAndCommitsTheBuildWhenItsPanelsAreUntouched(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	r.store.bump()
	r.store.rebaseOK = true

	answer := r.call(req, "update_panels", `{"panel":"b"}`, nil, versionConflict(2))

	if settledOf(answer) != "saved" || r.store.commits != 1 || r.store.rebases != 1 {
		t.Fatalf("settled %q, commits %d, rebases %d", settledOf(answer), r.store.commits, r.store.rebases)
	}
	if r.stored(req).State != string(revisions.StateRunning) {
		t.Fatalf("the run ended: %s", r.stored(req).State)
	}
	if !slicesContain(r.steps.messages(), "Rebased onto version 2") {
		t.Fatalf("steps = %q", r.steps.messages())
	}
}

func slicesContain(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestAConflictThatCannotRebaseRestartsOnceAndTheSecondEndsTheRun(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	r.store.bump()

	first := r.call(req, "update_panels", `{"panel":"b"}`, nil, versionConflict(2))
	if settledOf(first) != "restarted" || !hasNote(first, "your build was discarded") || r.store.discards != 1 {
		t.Fatalf("settled %q, notes %q, discards %d", settledOf(first), first.Notes, r.store.discards)
	}
	if r.stored(req).State != string(revisions.StateRunning) {
		t.Fatalf("the run ended after the first conflict: %s", r.stored(req).State)
	}
	scope, err := r.svc.Scope(context.Background(), req)
	if err != nil || scope.ParentVersion != 2 {
		t.Fatalf("scope after the restart = %+v, %v", scope, err)
	}

	r.startBuild(req, "c")
	r.store.bump()
	second := r.call(req, "update_panels", `{"panel":"d"}`, nil, versionConflict(3))

	row := r.stored(req)
	if row.State != string(revisions.StateFailed) || row.FailureCode.String != string(revisions.FailureConflict) || settledOf(second) != "ended" {
		t.Fatalf("run %s %q, answer %+v", row.State, row.FailureCode.String, second)
	}
	if row.Error.String != "the dashboard changed twice while the agent was revising it" {
		t.Fatalf("stored message = %q", row.Error.String)
	}
}

func TestANotFoundSaveEndsTheRunNotFound(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.call(req, "update_panels", `{"n":1}`, nil, func(context.Context) (tools.Result, error) {
		return tools.Result{Content: map[string]any{"code": "not_found"}, IsError: true}, nil
	})
	row := r.stored(req)
	if row.FailureCode.String != string(revisions.FailureNotFound) || row.Error.String != "the dashboard no longer exists" {
		t.Fatalf("run %q %q", row.FailureCode.String, row.Error.String)
	}
}

func TestBriefAnswersTheSystemTextTheMessageAndTheReason(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.System == "" || !strings.Contains(got.Message, "add a latency panel") || got.Reason != "" || got.Build != nil {
		t.Fatalf("brief = %+v", got)
	}
	if _, err := r.svc.Brief(context.Background(), req, "nonsense"); !errors.Is(err, tools.ErrInvalidReason) {
		t.Fatalf("err = %v, want tools.ErrInvalidReason", err)
	}
}

func TestBriefOfARunWithABuildResumesIt(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	got, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Reason != string(brief.Restart) || got.Build == nil || got.Build.Saves != 1 || !strings.Contains(got.Message, "Panels built: 1 of 3") {
		t.Fatalf("brief = %+v", got)
	}
}

func TestBriefRebasesAStaleBuildAndRestartsWhenItCannot(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	r.store.bump()
	r.store.rebaseOK = true
	rebased, err := r.svc.Brief(context.Background(), req, "")
	if err != nil || rebased.Build == nil || r.store.build.BaseVersion != 2 {
		t.Fatalf("rebased brief = %+v, %v", rebased, err)
	}

	r.store.bump()
	r.store.rebaseOK = false
	restarted, err := r.svc.Brief(context.Background(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Reason != string(brief.Conflict) || restarted.Build != nil || r.store.discards != 1 {
		t.Fatalf("restarted brief = %+v, discards %d", restarted, r.store.discards)
	}
}

func TestBriefEndsTheRunWhenTheBuildIsStaleAgainAfterItsRestart(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	r.store.bump()
	if _, err := r.svc.Brief(context.Background(), req, ""); err != nil {
		t.Fatal(err)
	}
	r.startBuild(req, "b")
	r.store.bump()
	if _, err := r.svc.Brief(context.Background(), req, ""); !errors.Is(err, tools.ErrRunEnded) {
		t.Fatalf("err = %v, want tools.ErrRunEnded", err)
	}
	if row := r.stored(req); row.FailureCode.String != string(revisions.FailureConflict) {
		t.Fatalf("failure = %q", row.FailureCode.String)
	}
}

func TestBriefOfAnEndedRunIsRunEnded(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	if _, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "failed", Message: "no"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Brief(context.Background(), req, ""); !errors.Is(err, tools.ErrRunEnded) {
		t.Fatalf("err = %v, want tools.ErrRunEnded", err)
	}
}

func TestNoteUsageMergesCumulativeCountsAndStopsAtTheCostLimit(t *testing.T) {
	r := newRig(t, withMaxCost(1, pricing.Table{}))
	req := r.revise()
	stop, err := r.svc.NoteUsage(context.Background(), req, revisions.Usage{Model: "m", InputTokens: 100, CostUSD: floatPtr(0.5)})
	if err != nil || stop {
		t.Fatalf("stop %v, err %v", stop, err)
	}
	stop, err = r.svc.NoteUsage(context.Background(), req, revisions.Usage{InputTokens: 40})
	if err != nil || stop {
		t.Fatalf("a lower report stopped the run: stop %v, err %v", stop, err)
	}
	stop, err = r.svc.NoteUsage(context.Background(), req, revisions.Usage{InputTokens: 300, CostUSD: floatPtr(1.5)})
	if err != nil || !stop {
		t.Fatalf("stop %v, err %v, want a stop at the cost limit", stop, err)
	}
	row := r.stored(req)
	if row.State != string(revisions.StateFailed) || row.FailureCode.String != string(revisions.FailureBudgetExhausted) || row.Error.String != "the run reached the server's cost limit" {
		t.Fatalf("run %s %q %q", row.State, row.FailureCode.String, row.Error.String)
	}
	if !slicesContain(r.steps.messages(), "Stopped: the run reached the server's cost limit") {
		t.Fatalf("steps = %q", r.steps.messages())
	}
	answer := r.read(req, 0)
	if contentCode(t, answer) != "run_ended" {
		t.Fatalf("the next call = %s", answer.Content)
	}
	if again, err := r.svc.NoteUsage(context.Background(), req, revisions.Usage{}); err != nil || !again {
		t.Fatalf("a report after the end: stop %v, err %v", again, err)
	}
}

func TestNoteUsageWithoutALimitNeverStops(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	stop, err := r.svc.NoteUsage(context.Background(), req, revisions.Usage{CostUSD: floatPtr(9000)})
	if err != nil || stop {
		t.Fatalf("stop %v, err %v", stop, err)
	}
}

func TestNoteUsageOfARunEndedElsewhereIsAStop(t *testing.T) {
	r := newRig(t, withMaxCost(1, pricing.Table{}))
	req := r.revise()
	r.read(req, 0)
	if _, err := r.runs.End(context.Background(), r.stored(req), revisions.Ending{Failure: revisions.FailureInternal, Message: "x"}); err != nil {
		t.Fatal(err)
	}
	stop, err := r.svc.NoteUsage(context.Background(), req, revisions.Usage{CostUSD: floatPtr(5)})
	if err != nil || !stop {
		t.Fatalf("stop %v, err %v", stop, err)
	}
	if row := r.stored(req); row.FailureCode.String != string(revisions.FailureInternal) {
		t.Fatalf("the elsewhere ending was overwritten: %q", row.FailureCode.String)
	}
	if n := r.sessionRows(req); n != 0 {
		t.Fatalf("%d session rows remain", n)
	}
}

func TestFinishRefusesInputTheLoopNeverSends(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	cases := []struct {
		name string
		in   tools.RunFinish
		want error
	}{
		{"unknown outcome", tools.RunFinish{Outcome: "done"}, claims.ErrInvalidOutcome},
		{"code with a success", tools.RunFinish{Outcome: "succeeded", FailureCode: "refused"}, tools.ErrInvalidFailureCode},
		{"code the loop never reports", tools.RunFinish{Outcome: "failed", FailureCode: "conflict"}, tools.ErrInvalidFailureCode},
		{"commit with a failure", tools.RunFinish{Outcome: "failed", Commit: true}, tools.ErrCommitNeedsSuccess},
	}
	for _, tc := range cases {
		if _, err := r.svc.Finish(context.Background(), req, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if r.stored(req).State != string(revisions.StateRunning) {
		t.Fatal("a refused finish ended the run")
	}
}

func TestFinishFailedWithALoopCodeStoresTheSharedMessage(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	out, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "failed", FailureCode: "rate_limited", Message: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	row := r.stored(req)
	if out.State != "failed" || out.FailureCode == nil || *out.FailureCode != "rate_limited" || row.Error.String != "the model provider is rate limiting requests" {
		t.Fatalf("out %+v, error %q", out, row.Error.String)
	}
	if n := r.sessionRows(req); n != 0 {
		t.Fatalf("%d session rows remain", n)
	}
	if n := r.tokenCount(req); n != 0 {
		t.Fatalf("%d run tokens remain", n)
	}
}

func TestFinishFailedWithoutACodeStoresTheCleanedMessageOrASilentOne(t *testing.T) {
	r := newRig(t)
	said := r.revise()
	out, err := r.svc.Finish(context.Background(), said, tools.RunFinish{Outcome: "failed", Message: "  no data \x00 here  "})
	if err != nil || out.FailureCode == nil || *out.FailureCode != "agent_failed" {
		t.Fatalf("out %+v, err %v", out, err)
	}
	if got := r.stored(said).Error.String; got != "no data  here" {
		t.Fatalf("stored message = %q", got)
	}
	silent := r.revise()
	if _, err := r.svc.Finish(context.Background(), silent, tools.RunFinish{Outcome: "failed", Message: " "}); err != nil {
		t.Fatal(err)
	}
	if got := r.stored(silent).Error.String; got != "the agent did not say why" {
		t.Fatalf("stored message = %q", got)
	}
}

func TestFinishSucceededWithNothingSavedFailsNoToolCall(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	out, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "succeeded"})
	if err != nil || out.State != "failed" || out.FailureCode == nil || *out.FailureCode != "no_tool_call" {
		t.Fatalf("out %+v, err %v", out, err)
	}
}

func TestFinishWithCommitSavesTheBuildAndSucceedsWithItsVersion(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	out, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "succeeded", Commit: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "succeeded" || out.ResultVersion == nil || *out.ResultVersion != 2 || out.Restarted || r.store.commits != 1 {
		t.Fatalf("out %+v, commits %d", out, r.store.commits)
	}
}

func TestFinishWithCommitOnAStaleBuildRebasesOrRestartsTheRun(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.startBuild(req, "a")
	r.store.bump()
	r.store.rebaseOK = true
	out, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "succeeded", Commit: true})
	if err != nil || out.State != "succeeded" || out.ResultVersion == nil || *out.ResultVersion != 3 {
		t.Fatalf("rebased finish = %+v, %v", out, err)
	}

	other := newRig(t)
	stale := other.revise()
	other.startBuild(stale, "a")
	other.store.bump()
	out, err = other.svc.Finish(context.Background(), stale, tools.RunFinish{Outcome: "succeeded", Commit: true})
	if err != nil || !out.Restarted || out.State != "running" {
		t.Fatalf("restarting finish = %+v, %v", out, err)
	}
	if other.stored(stale).State != string(revisions.StateRunning) || other.store.discards != 1 {
		t.Fatalf("state %s, discards %d", other.stored(stale).State, other.store.discards)
	}
}

func TestFinishAfterTheRunEndedAnswersTheSettledState(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	first, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "failed", Message: "no"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "succeeded"})
	if err != nil || second.State != first.State || *second.FailureCode != *first.FailureCode {
		t.Fatalf("second finish = %+v, %v; first %+v", second, err, first)
	}
}

func TestFinishOfACancelingRunSettlesItCanceled(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	id := mustUUID(t, req.ID)
	if _, err := r.runs.Cancel(context.Background(), id, ada); err != nil {
		t.Fatal(err)
	}
	if row := r.stored(req); row.State != string(revisions.StateCanceling) {
		t.Fatalf("state before finish = %q, want canceling", row.State)
	}
	out, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "failed", FailureCode: "provider_error"})
	if err != nil || out.State != string(revisions.StateCanceled) || out.FailureCode == nil || *out.FailureCode != "canceled" {
		t.Fatalf("out %+v, err %v", out, err)
	}
	if n := r.tokenCount(req); n != 0 {
		t.Fatalf("%d run tokens remain", n)
	}
}

func TestFinishOfARunEndedElsewhereAnswersItsSettledState(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.read(req, 0)
	if _, err := r.runs.End(context.Background(), r.stored(req), revisions.Ending{Failure: revisions.FailureInternal, Message: "x"}); err != nil {
		t.Fatal(err)
	}
	out, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "succeeded"})
	if err != nil || out.State != "failed" || *out.FailureCode != "internal" {
		t.Fatalf("out %+v, err %v", out, err)
	}
	if n := r.sessionRows(req); n != 0 {
		t.Fatalf("%d session rows remain", n)
	}
}

func TestFinishOfAnAttemptThatIsNotTheRunsIsNotYours(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	req.Attempt = 2
	if _, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "failed"}); !errors.Is(err, claims.ErrNotYourRun) {
		t.Fatalf("err = %v, want ErrNotYourRun", err)
	}
	if r.stored(req).State != string(revisions.StateRunning) {
		t.Fatal("a stale attempt ended the run")
	}
}

func TestAStaleAttemptWithItsOwnSessionCannotEndTheNewAttempt(t *testing.T) {
	r := newRig(t, withMaxCost(1, pricing.Table{}))
	req := r.revise()
	r.read(req, 0)
	r.d.Exec(t, "UPDATE agent_requests SET attempts = attempts + 1 WHERE id = $1", mustUUID(t, req.ID))
	stop, err := r.svc.NoteUsage(context.Background(), req, revisions.Usage{CostUSD: floatPtr(5)})
	if err != nil || !stop {
		t.Fatalf("stop %v, err %v", stop, err)
	}
	if got := r.stored(req).State; got != string(revisions.StateRunning) {
		t.Fatalf("the stale attempt ended the new attempt: %s", got)
	}
}

func TestFinishOfASuggestRunStoresTheSuggestionTheCallCaptured(t *testing.T) {
	r := newRig(t)
	req := r.suggest()
	r.call(req, "update_panels", `{"x":1}`, nil, suggestionSaved)
	out, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "succeeded"})
	if err != nil || out.State != "succeeded" || out.ResultKind != "suggestion" {
		t.Fatalf("out %+v, err %v", out, err)
	}
	if got := string(r.stored(req).Suggestion); !strings.Contains(got, `"type": "stat"`) && !strings.Contains(got, `"type":"stat"`) {
		t.Fatalf("stored suggestion = %s", got)
	}
}

func TestFinishStoresTheMergedUsageAndTheGuardCounts(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.read(req, 0)
	r.read(req, 1)
	if _, err := r.svc.NoteUsage(context.Background(), req, revisions.Usage{Provider: "p", Model: "m", InputTokens: 10, ModelCalls: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: "failed", Usage: &revisions.Usage{OutputTokens: 7}}); err != nil {
		t.Fatal(err)
	}
	var usage revisions.Usage
	if err := json.Unmarshal(r.stored(req).RunUsage, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.ToolCalls != 2 || usage.InputTokens != 10 || usage.OutputTokens != 7 || usage.ModelCalls != 2 || usage.Model != "m" {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestAGrantOrServiceAccountClaimerNeverReceivesASuggestRun(t *testing.T) {
	r := newRig(t)
	row := r.submit(revisions.SourceSuggest, "a")
	if _, found, err := r.runs.Claim(context.Background(), grantClaimant); err != nil || found {
		t.Fatalf("a grant claimed a suggest run: found %v, err %v", found, err)
	}
	account := revisions.Claimant{Kind: revisions.AssignServiceAccount, ID: grantID}
	if _, found, err := r.runs.Claim(context.Background(), account); err != nil || found {
		t.Fatalf("a service account claimed a suggest run: found %v, err %v", found, err)
	}
	for _, kind := range []string{"grant", "service_account"} {
		err := execErr(r, "UPDATE agent_requests SET assigned_kind = $2, assigned_id = $3 WHERE id = $1", row.ID, kind, mustUUID(t, grantID))
		if err == nil || !strings.Contains(err.Error(), "agent_requests_assigned_source_check") {
			t.Fatalf("assigning a suggest run to a %s: err = %v, want the source check", kind, err)
		}
	}
	if claimed := r.claim(builtin); claimed.Row.ID != row.ID {
		t.Fatalf("the built-in claimant took %v", claimed.Row.ID)
	}
}

func execErr(r *rig, query string, args ...any) error {
	_, err := r.d.SQLDB(r.t).Exec(query, args...)
	return err
}
