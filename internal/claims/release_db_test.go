package claims

import (
	"context"
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestReleasePutsAClaimedRunBackInTheQueueForTheNextClaim(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)

	answer, err := f.svc.Release(context.Background(), grantClaimer, queued.ID.String(), 1)
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != string(revisions.StateQueued) {
		t.Fatalf("answer = %+v, want queued", answer)
	}
	released := f.stored(t, queued.ID)
	if released.State != string(revisions.StateQueued) || released.Attempts != 0 || released.LeaseUntil.Valid || released.CheckFailures != 0 {
		t.Fatalf("released run = state %q attempts %d lease %v check failures %d, want queued at attempt 0 with no lease and no check failure",
			released.State, released.Attempts, released.LeaseUntil.Valid, released.CheckFailures)
	}
	if n := f.tokenCount(t, queued.ID); n != 0 {
		t.Fatalf("%d run tokens after the release, want none", n)
	}
	if again := f.claim(t, grantClaimer); again.Run.ID != queued.ID.String() || again.Run.Attempt != 1 {
		t.Fatalf("next claim = run %s attempt %d, want the released run at attempt 1", again.Run.ID, again.Run.Attempt)
	}
}

func TestReleaseIsOnlyForTheRunsClaimerAtItsAttempt(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)

	for name, tc := range map[string]struct {
		claimer Claimer
		runID   string
		attempt int
	}{
		"another claimer": {Claimer{Kind: revisions.AssignGrant, ID: otherGrantID, Person: userID}, queued.ID.String(), 1},
		"another attempt": {grantClaimer, queued.ID.String(), 2},
		"no such run":     {grantClaimer, otherGrantID, 1},
		"not a run id":    {grantClaimer, "nope", 1},
	} {
		if _, err := f.svc.Release(context.Background(), tc.claimer, tc.runID, tc.attempt); !errors.Is(err, ErrNotYourRun) {
			t.Errorf("%s: err = %v, want ErrNotYourRun", name, err)
		}
	}
	if held := f.stored(t, queued.ID); held.State != string(revisions.StateRunning) || held.Attempts != 1 {
		t.Fatalf("run = state %q attempts %d, want it left running at attempt 1", held.State, held.Attempts)
	}
	if n := f.tokenCount(t, queued.ID); n != 1 {
		t.Fatalf("%d run tokens after refused releases, want the holder's one", n)
	}
}

func TestReleaseOfARunItsRequesterCanceledSettlesItCanceled(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	if _, err := f.runs.Cancel(context.Background(), queued.ID, ada); err != nil {
		t.Fatal(err)
	}

	answer, err := f.svc.Release(context.Background(), grantClaimer, queued.ID.String(), 1)
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != string(revisions.StateCanceled) {
		t.Fatalf("answer = %+v, want canceled", answer)
	}
}

func TestReleaseOfARunThatEndedAnswersItsStateAndChangesNothing(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	f.d.Exec(t, "UPDATE agent_requests SET state = 'failed', status = 'failed', finished_at = now(), lease_until = NULL WHERE id = $1", queued.ID)

	answer, err := f.svc.Release(context.Background(), grantClaimer, queued.ID.String(), 1)
	if err != nil {
		t.Fatal(err)
	}

	if answer.State != string(revisions.StateFailed) {
		t.Fatalf("answer = %+v, want failed", answer)
	}
	if ended := f.stored(t, queued.ID); ended.State != string(revisions.StateFailed) || ended.Attempts != 1 {
		t.Fatalf("run = state %q attempts %d, want it left failed at attempt 1", ended.State, ended.Attempts)
	}
}

func TestReleaseThatCannotDropTheRunsTokensIsAnErrorAndLeavesTheHold(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	f.d.Exec(t, "ALTER TABLE run_tokens RENAME TO run_tokens_away")

	_, err := f.svc.Release(context.Background(), grantClaimer, queued.ID.String(), 1)

	if err == nil || errors.Is(err, ErrNotYourRun) {
		t.Fatalf("err = %v, want the failed token delete", err)
	}
	f.d.Exec(t, "ALTER TABLE run_tokens_away RENAME TO run_tokens")
	if held := f.stored(t, queued.ID); held.State != string(revisions.StateRunning) || held.Attempts != 1 || !held.LeaseUntil.Valid {
		t.Fatalf("run = state %q attempts %d lease %v, want it still running at attempt 1 under its lease", held.State, held.Attempts, held.LeaseUntil.Valid)
	}
	if n := f.tokenCount(t, queued.ID); n != 1 {
		t.Fatalf("%d run tokens after the failed release, want the holder's one", n)
	}
}

func TestReleaseThatCannotRequeueTheRunIsAnErrorAndTheNextCallReleasesIt(t *testing.T) {
	f := newFixture(t)
	queued, _ := f.claimed(t)
	f.d.Exec(t, `CREATE FUNCTION refuse_requeue() RETURNS trigger LANGUAGE plpgsql AS
		'BEGIN IF NEW.state = ''queued'' THEN RAISE EXCEPTION ''requeue refused''; END IF; RETURN NEW; END'`)
	f.d.Exec(t, "CREATE TRIGGER refuse_requeue BEFORE UPDATE ON agent_requests FOR EACH ROW EXECUTE FUNCTION refuse_requeue()")

	answer, err := f.svc.Release(context.Background(), grantClaimer, queued.ID.String(), 1)

	if err == nil || errors.Is(err, ErrNotYourRun) {
		t.Fatalf("answer = %+v, err = %v, want the failed release as an error", answer, err)
	}
	if held := f.stored(t, queued.ID); held.State != string(revisions.StateRunning) || held.Attempts != 1 {
		t.Fatalf("run = state %q attempts %d, want it still running at attempt 1", held.State, held.Attempts)
	}
	f.d.Exec(t, "DROP TRIGGER refuse_requeue ON agent_requests")
	again, err := f.svc.Release(context.Background(), grantClaimer, queued.ID.String(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if again.State != string(revisions.StateQueued) {
		t.Fatalf("second answer = %+v, want queued", again)
	}
}
