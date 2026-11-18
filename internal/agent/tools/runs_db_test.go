package tools

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type runsRig struct {
	registry *Registry
	database *dbtest.Database
}

func newRunsRig(t *testing.T) *runsRig {
	t.Helper()
	database := dbtest.New(t)
	queue := revisions.NewService(database.DB, slog.New(slog.DiscardHandler), revisions.WithBuiltInAgent(), revisions.WithAccess(folders.NewAccess(database.DB)))
	return &runsRig{registry: New(Deps{Runs: revisions.NewRunDesk(queue, revisions.ObserveOptions{})}), database: database}
}

func (r *runsRig) call(t *testing.T, tool, input string) Result {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(t.Context(), placeholderUser), tool, []byte(input))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

var finishedRunStates = map[string]string{
	"succeeded": "state = 'succeeded', status = 'succeeded', failure_code = NULL",
	"failed":    "state = 'failed', status = 'failed', failure_code = 'no_progress'",
	"canceled":  "state = 'canceled', status = 'failed', failure_code = 'canceled'",
}

func (r *runsRig) finish(t *testing.T, id, state string) {
	t.Helper()
	r.database.Exec(t, "UPDATE agent_requests SET "+finishedRunStates[state]+", finished_at = now(), lease_until = NULL WHERE id = $1", id)
}

func (r *runsRig) start(t *testing.T) string {
	t.Helper()
	res := r.call(t, nameStartRun, `{"prompt":"a latency dashboard"}`)
	if res.IsError {
		t.Fatalf("start_run: %s", contentJSON(t, res))
	}
	var run struct{ ID string }
	if err := json.Unmarshal(contentJSON(t, res), &run); err != nil || run.ID == "" {
		t.Fatalf("start_run answered %s", contentJSON(t, res))
	}
	return run.ID
}

func stateOf(t *testing.T, res Result) string {
	t.Helper()
	var run struct{ State string }
	if err := json.Unmarshal(contentJSON(t, res), &run); err != nil {
		t.Fatal(err)
	}
	return run.State
}

func TestCancelRunAnswersTheCurrentStateOfAFinishedRun(t *testing.T) {
	r := newRunsRig(t)
	id := r.start(t)
	target := `{"id":"` + id + `"}`

	if res := r.call(t, nameCancelRun, target); res.IsError || stateOf(t, res) != "canceled" {
		t.Fatalf("first cancel: %s", contentJSON(t, res))
	}
	if res := r.call(t, nameCancelRun, target); res.IsError || stateOf(t, res) != "canceled" {
		t.Fatalf("repeated cancel: %s", contentJSON(t, res))
	}
	for _, state := range []string{"succeeded", "failed"} {
		r.finish(t, id, state)
		if res := r.call(t, nameCancelRun, target); res.IsError || stateOf(t, res) != state {
			t.Fatalf("cancel of a %s run: %s", state, contentJSON(t, res))
		}
	}
}

func TestRetryRunMatchesWhatTheRunMachineAllows(t *testing.T) {
	r := newRunsRig(t)
	id := r.start(t)
	target := `{"id":"` + id + `"}`

	queued := r.call(t, nameRetryRun, target)
	if code := errorCode(t, queued); code != codeInvalidTransition {
		t.Fatalf("retry of a queued run: code %q", code)
	}
	r.database.Exec(t, "UPDATE agent_requests SET state = 'running', status = 'running', attempts = 1 WHERE id = $1", id)
	if code := errorCode(t, r.call(t, nameRetryRun, target)); code != codeInvalidTransition {
		t.Fatalf("retry of a running run: code %q", code)
	}
	for _, state := range []string{"succeeded", "failed", "canceled"} {
		r.finish(t, id, state)
		res := r.call(t, nameRetryRun, target)
		if res.IsError || stateOf(t, res) != "queued" {
			t.Fatalf("retry of a %s run: %s", state, contentJSON(t, res))
		}
	}
}
