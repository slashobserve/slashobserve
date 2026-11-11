package claims

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func finishOf(queued dbsqlc.AgentRequest, outcome, message string) FinishInput {
	return FinishInput{RunID: queued.ID.String(), Attempt: 1, Outcome: outcome, Message: message}
}

func TestFinishSucceededWithNothingSavedFailsTheRunWithNoToolCall(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)

	answer, err := f.svc.Finish(context.Background(), grantClaimer, finishOf(queued, "succeeded", ""))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "failed" || answer.FailureCode == nil || *answer.FailureCode != "no_tool_call" || answer.ResultVersion != nil {
		t.Fatalf("answer = %+v, want failed no_tool_call", answer)
	}
}

func TestFinishFailedRecordsTheCleanedMessage(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)

	answer, err := f.svc.Finish(context.Background(), grantClaimer, finishOf(queued, "failed", "  the datasource has no data  "))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "failed" || answer.FailureCode == nil || *answer.FailureCode != "agent_failed" {
		t.Fatalf("answer = %+v, want failed agent_failed", answer)
	}
	if got := f.stored(t, queued.ID).Error.String; got != "the datasource has no data" {
		t.Fatalf("stored error = %q", got)
	}
}

func TestFinishAfterAVersionWasSavedSucceedsWhateverTheOutcomeSays(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	_, err := f.d.Queries().InsertDashboardVersion(context.Background(), dbsqlc.InsertDashboardVersionParams{
		DashboardID: f.dashboard, Version: 2, ParentVersion: pgtype.Int4{Int32: 1, Valid: true}, Spec: []byte(`{}`), Summary: "s",
		AuthorKind: "user", AuthorID: mustUUID(t, userID), AuthorName: "Ada", AgentRequestID: queued.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.d.Exec(t, "UPDATE dashboards SET latest_version = 2 WHERE id = $1", f.dashboard)

	answer, err := f.svc.Finish(context.Background(), grantClaimer, finishOf(queued, "failed", "gave up"))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "succeeded" || answer.ResultVersion == nil || *answer.ResultVersion != 2 || answer.FailureCode != nil {
		t.Fatalf("answer = %+v, want succeeded with version 2", answer)
	}
}

func TestFinishOfACancelingRunSettlesItCanceled(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	if _, err := f.runs.Cancel(context.Background(), queued.ID, ada); err != nil {
		t.Fatal(err)
	}

	answer, err := f.svc.Finish(context.Background(), grantClaimer, finishOf(queued, "succeeded", ""))
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != "canceled" {
		t.Fatalf("answer = %+v, want canceled", answer)
	}
}

func TestSecondFinishAnswersTheSameState(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	first, err := f.svc.Finish(context.Background(), grantClaimer, finishOf(queued, "failed", "no data"))
	if err != nil {
		t.Fatal(err)
	}

	second, err := f.svc.Finish(context.Background(), grantClaimer, finishOf(queued, "succeeded", ""))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("first answer %+v, second %+v, want the same", first, second)
	}
}

func TestFinishDeletesTheRunTokens(t *testing.T) {
	f := newFixture(t)
	queued, offer := f.claimed(t)
	if f.tokenCount(t, queued.ID) != 1 {
		t.Fatal("the claim did not store a run token")
	}

	if _, err := f.svc.Finish(context.Background(), grantClaimer, finishOf(queued, "failed", "no data")); err != nil {
		t.Fatal(err)
	}

	if n := f.tokenCount(t, queued.ID); n != 0 {
		t.Fatalf("%d run tokens left after finish", n)
	}
	if _, ok := f.authenticate(t, "Bearer "+offer.Session.Token); ok {
		t.Fatal("the token still authenticates")
	}
}

func TestFinishStoresTheReportedCost(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	cost := 0.42
	in := finishOf(queued, "failed", "no data")
	in.Usage = &FinishUsage{Provider: "anthropic", Model: "claude-haiku-5-5", InputTokens: 1200, OutputTokens: 300, CostUSD: &cost}

	if _, err := f.svc.Finish(context.Background(), grantClaimer, in); err != nil {
		t.Fatal(err)
	}

	var usage revisions.Usage
	if err := json.Unmarshal(f.stored(t, queued.ID).RunUsage, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.CostUSD == nil || *usage.CostUSD != 0.42 || usage.Model != "claude-haiku-5-5" || usage.InputTokens != 1200 || usage.OutputTokens != 300 {
		t.Fatalf("stored usage = %+v", usage)
	}
}

func TestFinishOfARunThatIsNotYoursIsRefused(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	otherAgent := Claimer{Kind: revisions.AssignGrant, ID: otherGrantID, Person: userID}

	if _, err := f.svc.Finish(context.Background(), otherAgent, finishOf(queued, "failed", "no")); !errors.Is(err, ErrNotYourRun) {
		t.Fatalf("finish by another claimer = %v, want ErrNotYourRun", err)
	}
	wrongAttempt := finishOf(queued, "failed", "no")
	wrongAttempt.Attempt = 2
	if _, err := f.svc.Finish(context.Background(), grantClaimer, wrongAttempt); !errors.Is(err, ErrNotYourRun) {
		t.Fatalf("finish at the wrong attempt = %v, want ErrNotYourRun", err)
	}
	if got := f.stored(t, queued.ID).State; got != "running" {
		t.Fatalf("run state = %s, want it untouched", got)
	}
}
