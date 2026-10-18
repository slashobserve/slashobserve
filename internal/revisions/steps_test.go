package revisions_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestStepCapAppliesPerAttempt(t *testing.T) {
	svc, d := newService(t, succeedWith(1))
	ctx := context.Background()
	row, err := svc.SubmitObserve(ctx, user, "steps")
	if err != nil {
		t.Fatal(err)
	}
	steps := revisions.NewSteps(d.DB, quietLogger())
	record := func(attempt, n int) {
		d.Exec(t, "UPDATE agent_requests SET status = 'running', attempts = $2 WHERE id = $1", row.ID, attempt)
		for range n {
			steps.RecordStep(ctx, revisions.Request{ID: row.ID.String(), Attempt: attempt}, revisions.Step{
				At: time.Now(), Phase: "query", Tool: "query_range", Status: "ok", Message: "Queried",
			})
		}
	}
	record(1, revisions.MaxSteps+5)
	record(2, 3)

	got, err := svc.GetByID(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	var stored []revisions.Step
	if err := json.Unmarshal(got.Steps, &stored); err != nil {
		t.Fatal(err)
	}
	perAttempt := map[int]int{}
	for _, s := range stored {
		perAttempt[s.Attempt]++
	}
	if perAttempt[1] != revisions.MaxSteps || perAttempt[2] != 3 {
		t.Fatalf("steps per attempt = %v, want %d for attempt 1 and 3 for attempt 2", perAttempt, revisions.MaxSteps)
	}
}

func TestReleasedRunLeavesNoStepsAndTheReplacementGetsAFreshCap(t *testing.T) {
	var steps *revisions.Steps
	record := func(ctx context.Context, req revisions.Request, message string, n int) {
		for range n {
			steps.RecordStep(ctx, req, revisions.Step{At: time.Now(), Phase: "query", Tool: "query_range", Status: "ok", Message: message})
		}
	}
	started := make(chan struct{})
	svc, d := newService(t, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
		record(ctx, req, "released", 3)
		close(started)
		<-ctx.Done()
		return revisions.Result{}, ctx.Err()
	}))
	steps = revisions.NewSteps(d.DB, quietLogger())
	row, err := svc.SubmitObserve(context.Background(), user, "steps")
	if err != nil {
		t.Fatal(err)
	}
	stop := startWorker(t, svc, d, revisions.WorkerOptions{})
	<-started
	stop()

	next := revisions.NewService(d.DB, reviserFunc(func(ctx context.Context, req revisions.Request) (revisions.Result, error) {
		record(ctx, req, "replacement", revisions.MaxSteps+5)
		return revisions.Result{}, errors.New("stop")
	}), quietLogger())
	startWorker(t, next, d, revisions.WorkerOptions{})
	waitFor(t, "the replacement run to finish", func() bool {
		got, err := next.GetByID(context.Background(), row.ID)
		return err == nil && got.Status == string(revisions.StatusFailed)
	})

	got, err := next.GetByID(context.Background(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	var stored []revisions.Step
	if err := json.Unmarshal(got.Steps, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != revisions.MaxSteps {
		t.Fatalf("stored %d steps, want the replacement run's %d", len(stored), revisions.MaxSteps)
	}
	for _, s := range stored {
		if s.Message != "replacement" {
			t.Fatalf("stored a %q step, want only the replacement run's steps", s.Message)
		}
	}
}
