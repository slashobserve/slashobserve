package claims

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (f *fixture) claimed(t *testing.T) (dbsqlc.AgentRequest, Offer) {
	t.Helper()
	queued := f.queue(t, revisions.AssignGrant, grantID)
	return queued, f.claim(t, grantClaimer)
}

func reportOf(queued dbsqlc.AgentRequest, attempt int) ReportInput {
	return ReportInput{RunID: queued.ID.String(), Attempt: attempt}
}

func TestReportRenewsTheLeaseOfALiveRun(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	f.d.Exec(t, "UPDATE agent_requests SET lease_until = now() + interval '5 seconds' WHERE id = $1", queued.ID)
	before := f.stored(t, queued.ID).LeaseUntil.Time

	answer, err := f.svc.Report(context.Background(), grantClaimer, reportOf(queued, 1))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "running" || answer.Stop || answer.LeaseUntil == nil {
		t.Fatalf("answer = %+v, want running without stop and a lease", answer)
	}
	if after := f.stored(t, queued.ID).LeaseUntil.Time; !after.After(before.Add(30 * time.Second)) {
		t.Fatalf("lease_until moved from %v to %v, want it renewed to about a minute", before, after)
	}
	until, err := time.Parse(time.RFC3339, *answer.LeaseUntil)
	if err != nil || time.Until(until) < 50*time.Second || time.Until(until) > 70*time.Second {
		t.Fatalf("lease_until %q (%v), want about a minute from now", *answer.LeaseUntil, err)
	}
}

func TestReportRecordsTheNoteAsAStep(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	in := reportOf(queued, 1)
	in.Note = "  checking the latency query  "

	if _, err := f.svc.Report(context.Background(), grantClaimer, in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Report(context.Background(), grantClaimer, reportOf(queued, 1)); err != nil {
		t.Fatal(err)
	}

	var steps []revisions.Step
	if err := json.Unmarshal(f.stored(t, queued.ID).Steps, &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Fatalf("steps = %+v, want only the one report with a note", steps)
	}
	if got := steps[0]; got.Phase != "discover" || got.Tool != "report_run" || got.Status != "ok" || got.Message != "checking the latency query" || got.Attempt != 1 {
		t.Fatalf("step = %+v", got)
	}
}

func TestReportOfACancelingRunTellsTheAgentToStop(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	if _, err := f.runs.Cancel(context.Background(), queued.ID, ada); err != nil {
		t.Fatal(err)
	}

	answer, err := f.svc.Report(context.Background(), grantClaimer, reportOf(queued, 1))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "canceling" || !answer.Stop || answer.LeaseUntil != nil {
		t.Fatalf("answer = %+v, want canceling with stop and no lease", answer)
	}
}

func TestReportOfAFinishedRunTellsTheAgentToStop(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	if _, err := f.runs.End(context.Background(), f.stored(t, queued.ID), revisions.Ending{Failure: revisions.FailureAgentFailed, Message: "no"}); err != nil {
		t.Fatal(err)
	}

	answer, err := f.svc.Report(context.Background(), grantClaimer, reportOf(queued, 1))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "failed" || !answer.Stop {
		t.Fatalf("answer = %+v, want failed with stop", answer)
	}
}

func TestReportOfARunThatIsNotYoursIsRefused(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	otherAgent := Claimer{Kind: revisions.AssignGrant, ID: otherGrantID, Person: userID}

	for name, tc := range map[string]struct {
		claimer Claimer
		in      ReportInput
	}{
		"wrong attempt":       {grantClaimer, reportOf(queued, 2)},
		"attempt zero":        {grantClaimer, reportOf(queued, 0)},
		"another claimer":     {otherAgent, reportOf(queued, 1)},
		"bad run id":          {grantClaimer, ReportInput{RunID: "nope", Attempt: 1}},
		"unknown run":         {grantClaimer, ReportInput{RunID: otherGrantID, Attempt: 1}},
		"service account key": {Claimer{Kind: revisions.AssignServiceAccount, ID: grantID}, reportOf(queued, 1)},
	} {
		if _, err := f.svc.Report(context.Background(), tc.claimer, tc.in); !errors.Is(err, ErrNotYourRun) {
			t.Errorf("%s: report = %v, want ErrNotYourRun", name, err)
		}
	}
}

func TestReportPastTheDeadlineEndsTheRunWithRunTimeout(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	f.d.Exec(t, "UPDATE run_tokens SET expires_at = now() - interval '1 second' WHERE agent_request_id = $1", queued.ID)

	answer, err := f.svc.Report(context.Background(), grantClaimer, reportOf(queued, 1))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "failed" || !answer.Stop {
		t.Fatalf("answer = %+v, want failed with stop", answer)
	}
	row := f.stored(t, queued.ID)
	if row.State != "failed" || row.FailureCode.String != string(revisions.FailureRunTimeout) || row.Error.String != "the run passed its time limit" {
		t.Fatalf("run = state %s code %q error %q", row.State, row.FailureCode.String, row.Error.String)
	}
	if n := f.tokenCount(t, queued.ID); n != 0 {
		t.Fatalf("%d run tokens left after the timeout", n)
	}
}

func TestReportAfterTheTokensWerePrunedEndsTheRunWithRunTimeout(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	f.d.Exec(t, "DELETE FROM run_tokens WHERE agent_request_id = $1", queued.ID)

	answer, err := f.svc.Report(context.Background(), grantClaimer, reportOf(queued, 1))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "failed" || !answer.Stop {
		t.Fatalf("answer = %+v, want failed with stop", answer)
	}
	if code := f.stored(t, queued.ID).FailureCode.String; code != string(revisions.FailureRunTimeout) {
		t.Fatalf("failure code = %q, want run_timeout", code)
	}
}

func TestReportOfAnEarlierAttemptIsRefusedOnceTheRunWasClaimedAgain(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	f.d.Exec(t, "UPDATE agent_requests SET lease_until = now() - interval '1 second' WHERE id = $1", queued.ID)
	f.claim(t, grantClaimer)

	if _, err := f.svc.Report(context.Background(), grantClaimer, reportOf(queued, 1)); !errors.Is(err, ErrNotYourRun) {
		t.Fatalf("report of attempt 1 after attempt 2 took the run = %v, want ErrNotYourRun", err)
	}
}
