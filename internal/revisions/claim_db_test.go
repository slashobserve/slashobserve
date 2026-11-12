package revisions_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	grantA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	grantB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

var builtinClaimant = revisions.Claimant{Kind: revisions.AssignBuiltin, ID: "server-1"}

func grantClaimant(id, person string) revisions.Claimant {
	return revisions.Claimant{Kind: revisions.AssignGrant, ID: id, Person: person}
}

func assignTo(t *testing.T, d *dbtest.Database, id, kind, assignee string) {
	t.Helper()
	d.Exec(t, "UPDATE agent_requests SET assigned_kind = $2, assigned_id = $3 WHERE id = $1", mustUUID(t, id), kind, mustUUID(t, assignee))
}

func storedRun(t *testing.T, svc *revisions.Service, id string) dbsqlc.AgentRequest {
	t.Helper()
	row, err := svc.GetByID(context.Background(), mustUUID(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func claimAs(t *testing.T, svc *revisions.Service, c revisions.Claimant) (revisions.Claimed, bool) {
	t.Helper()
	claimed, found, err := svc.Claim(context.Background(), c)
	if err != nil {
		t.Fatalf("claim as %s: %v", c.Key(), err)
	}
	return claimed, found
}

func TestBuiltinClaimantNeverTakesARunAssignedToAnOutsideAgent(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	outside := submit(t, svc, dash, "for a connected agent")
	assignTo(t, d, outside.ID, "grant", grantA)

	if claimed, found := claimAs(t, svc, builtinClaimant); found {
		t.Fatalf("the built-in claimant took %s, assigned to a grant", claimed.Row.ID)
	}
	own := submit(t, svc, dash, "for the built-in agent")
	claimed, found := claimAs(t, svc, builtinClaimant)
	if !found || claimed.Row.ID != mustUUID(t, own.ID) || claimed.Row.ClaimedBy.String != "builtin:server-1" {
		t.Fatalf("claim = %+v found %v, want the built-in run claimed by builtin:server-1", claimed.Row, found)
	}
	if claimed.Request.Assignee != revisions.AssignBuiltin {
		t.Fatalf("request assignee = %q, want builtin", claimed.Request.Assignee)
	}
}

func TestGrantClaimantTakesOnlyRunsAssignedToItsGrant(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	first := submit(t, svc, seedDashboard(t, d, 1), "for grant b")
	second := submit(t, svc, seedDashboard(t, d, 1), "for grant a")
	assignTo(t, d, first.ID, "grant", grantB)
	assignTo(t, d, second.ID, "grant", grantA)

	claimed, found := claimAs(t, svc, grantClaimant(grantA, userID))
	if !found || claimed.Row.ID != mustUUID(t, second.ID) {
		t.Fatalf("claim = %+v found %v, want the run assigned to grant a", claimed.Row, found)
	}
	if claimed.Row.ClaimedBy.String != "grant:"+grantA || claimed.Request.RequestedBy.ID != userID || claimed.Request.Attempt != 1 {
		t.Fatalf("claimed_by %q, requester %q, attempt %d", claimed.Row.ClaimedBy.String, claimed.Request.RequestedBy.ID, claimed.Request.Attempt)
	}
	if claimed.Request.Assignee != revisions.AssignGrant {
		t.Fatalf("request assignee = %q, want grant", claimed.Request.Assignee)
	}
	if _, found := claimAs(t, svc, grantClaimant(grantA, userID)); found {
		t.Fatal("grant a took a second run, assigned to grant b")
	}
	if _, found := claimAs(t, svc, revisions.Claimant{Kind: revisions.AssignServiceAccount, ID: grantB}); found {
		t.Fatal("a service account took a run assigned to a grant with its id")
	}
}

func TestTwoClaimsOfOneQueuedRunReturnOneClaim(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	submit(t, svc, seedDashboard(t, d, 1), "only one")

	var wg sync.WaitGroup
	results := make([]bool, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, found, err := svc.Claim(context.Background(), builtinClaimant)
			if err != nil {
				t.Error(err)
			}
			results[i] = found
		}()
	}
	wg.Wait()
	if results[0] == results[1] {
		t.Fatalf("claims found %v, want exactly one", results)
	}
}

func TestGrantClaimantCannotTakeARunOfAnotherPerson(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	rev := submit(t, svc, seedDashboard(t, d, 1), "someone else's run")
	assignTo(t, d, rev.ID, "grant", grantA)

	if _, found := claimAs(t, svc, grantClaimant(grantA, "22222222-2222-4222-8222-222222222222")); found {
		t.Fatal("a grant claimed a run of another person")
	}
	row := storedRun(t, svc, rev.ID)
	if row.State != string(revisions.StateFailed) || row.FailureCode.String != string(revisions.FailureRequesterForbidden) || row.Error.String != "this agent acts for another person" {
		t.Fatalf("run = state %s code %q error %q, want failed requester_forbidden", row.State, row.FailureCode.String, row.Error.String)
	}
}

func TestEndWithNoSavedResultFailsTheRunWithNoToolCall(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	rev := submit(t, svc, seedDashboard(t, d, 1), "stops early")
	assignTo(t, d, rev.ID, "grant", grantA)
	claimed, _ := claimAs(t, svc, grantClaimant(grantA, userID))

	ended, err := svc.End(context.Background(), claimed.Row, revisions.Ending{})
	if err != nil {
		t.Fatal(err)
	}
	if ended.State != string(revisions.StateFailed) || ended.FailureCode.String != string(revisions.FailureNoToolCall) {
		t.Fatalf("ended run = state %s code %q, want failed no_tool_call", ended.State, ended.FailureCode.String)
	}
}

func TestEndWithAReportedFailureKeepsItsCodeAndMessage(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	rev := submit(t, svc, seedDashboard(t, d, 1), "gives up")
	assignTo(t, d, rev.ID, "grant", grantA)
	claimed, _ := claimAs(t, svc, grantClaimant(grantA, userID))

	ended, err := svc.End(context.Background(), claimed.Row, revisions.Ending{Failure: revisions.FailureAgentFailed, Message: "no data"})
	if err != nil {
		t.Fatal(err)
	}
	if ended.FailureCode.String != string(revisions.FailureAgentFailed) || ended.Error.String != "no data" {
		t.Fatalf("ended run = code %q error %q", ended.FailureCode.String, ended.Error.String)
	}
}

func TestEndAfterAVersionWasSavedSucceeds(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	rev := submit(t, svc, dash, "saved, then reports failure")
	assignTo(t, d, rev.ID, "grant", grantA)
	claimed, _ := claimAs(t, svc, grantClaimant(grantA, userID))
	saveVersionUnder(t, d, dash, claimed.Row.ID, 2)

	ended, err := svc.End(context.Background(), claimed.Row, revisions.Ending{Failure: revisions.FailureAgentFailed, Message: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if ended.State != string(revisions.StateSucceeded) || ended.ResultVersion.Int32 != 2 || ended.FailureCode.Valid {
		t.Fatalf("ended run = state %s version %d code %+v, want succeeded with version 2", ended.State, ended.ResultVersion.Int32, ended.FailureCode)
	}
}

func TestEndOfALostAttemptWritesNothing(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	rev := submit(t, svc, seedDashboard(t, d, 1), "lease lost")
	assignTo(t, d, rev.ID, "grant", grantA)
	claimed, _ := claimAs(t, svc, grantClaimant(grantA, userID))
	d.Exec(t, "UPDATE agent_requests SET attempts = attempts + 1 WHERE id = $1", claimed.Row.ID)

	if _, err := svc.End(context.Background(), claimed.Row, revisions.Ending{}); !errors.Is(err, revisions.ErrLeaseLost) {
		t.Fatalf("End error = %v, want ErrLeaseLost", err)
	}
	if row := storedRun(t, svc, rev.ID); row.State != string(revisions.StateRunning) {
		t.Fatalf("run state = %s, want running", row.State)
	}
}

func TestSweepExpiresOnlyOldRunsOfOutsideAgents(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	old := submit(t, svc, dash, "outside and old")
	recent := submit(t, svc, dash, "outside and recent")
	builtin := submit(t, svc, dash, "built-in and old")
	assignTo(t, d, old.ID, "grant", grantA)
	assignTo(t, d, recent.ID, "service_account", grantB)
	d.Exec(t, "UPDATE agent_requests SET created_at = now() - interval '11 minutes' WHERE id = ANY($1)", []pgtype.UUID{mustUUID(t, old.ID), mustUUID(t, builtin.ID)})
	d.Exec(t, "UPDATE agent_requests SET created_at = now() - interval '9 minutes' WHERE id = $1", mustUUID(t, recent.ID))

	if err := svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	expired := storedRun(t, svc, old.ID)
	if expired.State != string(revisions.StateFailed) || expired.FailureCode.String != string(revisions.FailureAgentUnavailable) || expired.Error.String != revisions.UnclaimedMessage {
		t.Fatalf("old outside run = state %s code %q error %q, want failed agent_unavailable", expired.State, expired.FailureCode.String, expired.Error.String)
	}
	for name, id := range map[string]string{"recent outside run": recent.ID, "old built-in run": builtin.ID} {
		if row := storedRun(t, svc, id); row.State != string(revisions.StateQueued) {
			t.Errorf("%s state = %s, want queued", name, row.State)
		}
	}
}

func TestSweepExpiresAnOutsideRunWhoseLeaseLapsedLongAgo(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	rev := submit(t, svc, seedDashboard(t, d, 1), "the agent never came back")
	assignTo(t, d, rev.ID, "grant", grantA)
	claimAs(t, svc, grantClaimant(grantA, userID))
	d.Exec(t, "UPDATE agent_requests SET created_at = now() - interval '1 hour', lease_until = now() - interval '11 minutes' WHERE id = $1", mustUUID(t, rev.ID))

	if err := svc.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if row := storedRun(t, svc, rev.ID); row.State != string(revisions.StateFailed) || row.FailureCode.String != string(revisions.FailureAgentUnavailable) {
		t.Fatalf("run = state %s code %q, want failed agent_unavailable", row.State, row.FailureCode.String)
	}
}
