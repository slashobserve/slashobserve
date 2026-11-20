package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

func countRequests(t *testing.T, d *dbtest.Database, prompt string) int {
	t.Helper()
	var n int
	if err := d.SQLDB(t).QueryRow(`SELECT count(*) FROM agent_requests WHERE prompt = $1`, prompt).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAFailedRequestCanBeRetriedOnceAndSaysWhichRequestRetriedIt(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	first, err := svc.SubmitObserve(ctx, user, "retry me once")
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `UPDATE agent_requests SET state = 'canceled', status = 'failed', failure_code = 'canceled', error = 'x', finished_at = clock_timestamp() WHERE id = $1`, first.ID)

	retried, err := svc.Retry(ctx, first.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Retry(ctx, first.ID, user)
	var refusal *lifecycle.Refusal
	if !errors.As(err, &refusal) || !errors.Is(err, lifecycle.ErrInvalidTransition) || refusal.Action != "retry" {
		t.Fatalf("second retry = %v, want a refusal of retry", err)
	}
	if got := countRequests(t, d, "retry me once"); got != 2 {
		t.Fatalf("requests with the prompt = %d, want the original and one retry", got)
	}

	rec := call(observeRouterFor(svc, user), http.MethodGet, "/api/v1/observe/"+first.ID.String(), "")
	var wrapped struct {
		Request struct {
			RetriedBy      *string  `json:"retried_by"`
			AllowedActions []string `json:"allowed_actions"`
		} `json:"request"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wrapped); err != nil || wrapped.Request.RetriedBy == nil || *wrapped.Request.RetriedBy != retried.ID.String() {
		t.Fatalf("retried_by: %d %s", rec.Code, rec.Body)
	}
	for _, action := range wrapped.Request.AllowedActions {
		if action == "retry" {
			t.Fatalf("allowed_actions = %v, want no retry once it was retried", wrapped.Request.AllowedActions)
		}
	}
}

func TestASucceededRequestCanStillBeRetriedAgain(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	first, err := svc.SubmitObserve(ctx, user, "ask again")
	if err != nil {
		t.Fatal(err)
	}
	answerWith(t, d, first.ID, "answer")
	d.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', finished_at = clock_timestamp() WHERE id = $1`, first.ID)
	for range 2 {
		if _, err := svc.Retry(ctx, first.ID, user); err != nil {
			t.Fatal(err)
		}
	}
}

func TestARunTakesOneSuccessorWhetherItWasRetriedOrContinued(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	ctx := reading(context.Background())
	first, err := svc.SubmitObserve(ctx, user, "one successor")
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(t, `UPDATE agent_requests SET state = 'canceled', status = 'failed', failure_code = 'canceled', error = 'x', finished_at = clock_timestamp() WHERE id = $1`, first.ID)
	if _, err := svc.Retry(ctx, first.ID, user); err != nil {
		t.Fatal(err)
	}
	other, err := svc.SubmitObserve(ctx, user, "a later request")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.SQLDB(t).ExecContext(ctx, `UPDATE agent_requests SET continues_id = $1 WHERE id = $2`, first.ID, other.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "agent_requests_successor_key" {
		t.Fatalf("continuing a retried run = %v, want the successor key to refuse it", err)
	}
}
