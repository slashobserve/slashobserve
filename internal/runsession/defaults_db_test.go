package runsession

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestEachTaskTypeStartsWithItsOwnCallLimitUnlessTheSettingIsSet(t *testing.T) {
	for _, c := range []struct {
		prompt string
		set    int
		want   int
	}{
		{"what is the p99 latency of checkout", 0, 16},
		{"why is checkout slow", 0, 30},
		{"which services exist", 0, 12},
		{"tell me something", 0, 30},
		{"what is the p99 latency of checkout", 25, 25},
	} {
		req := revisions.Request{ID: "run", Attempt: 1, Intent: revisions.IntentAsk, Prompt: c.prompt, Investigation: true}
		state, err := (&Service{investigationCalls: c.set}).initial(context.Background(), req, pgtype.UUID{}, false)
		if err != nil {
			t.Fatal(err)
		}
		if state.Guard.InvestigationCalls != c.want {
			t.Errorf("%q with %d set: %d calls, want %d", c.prompt, c.set, state.Guard.InvestigationCalls, c.want)
		}
	}
}

func (r *rig) storedState(req revisions.Request) State {
	r.t.Helper()
	var raw []byte
	if err := r.d.SQLDB(r.t).QueryRow("SELECT state FROM run_sessions WHERE agent_request_id = $1", mustUUID(r.t, req.ID)).Scan(&raw); err != nil {
		r.t.Fatal(err)
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		r.t.Fatal(err)
	}
	return state
}

func TestNewDefaultsReachNewRunsOnlyAndAStoredSessionKeepsWhatItStartedWith(t *testing.T) {
	r := newRig(t, func(c *Config) {
		c.InvestigationLoop, c.FindingGrounding, c.InvestigationCallsMax = true, grounding.Enforce, 40
	})
	req := r.investigate()
	req.Assignee = revisions.AssignBuiltin
	r.read(req, 1)
	before := r.storedState(req)
	if !before.Loop || before.Grounding == nil || before.Grounding.Mode != grounding.Enforce || before.Guard.InvestigationCalls != 30 || before.Guard.Loop == nil || before.Guard.Loop.Max != 40 {
		t.Fatalf("a run under the defaults started with loop %v, grounding %+v, calls %d, loop %+v", before.Loop, before.Grounding, before.Guard.InvestigationCalls, before.Guard.Loop)
	}

	changed := New(Config{DB: r.d.DB, Runs: r.runs, Claims: r.claims, Builds: r.store, Dashboards: r.store, Statuses: healthy{}, Steps: r.steps,
		InvestigationCalls: 8, FindingGrounding: grounding.Off})
	if _, err := changed.Call(context.Background(), req, Call{Name: "list_metrics", Input: json.RawMessage(`{"q":"2"}`)}, readOK); err != nil {
		t.Fatal(err)
	}
	after := r.storedState(req)
	if !after.Loop || after.Grounding == nil || after.Guard.InvestigationCalls != 30 || after.Guard.Loop == nil || after.Guard.Loop.Max != 40 {
		t.Fatalf("the stored session changed with the settings: loop %v, grounding %+v, calls %d", after.Loop, after.Grounding, after.Guard.InvestigationCalls)
	}

	fresh := r.investigate()
	if _, err := changed.Call(context.Background(), fresh, Call{Name: "list_metrics", Input: json.RawMessage(`{"q":"1"}`)}, readOK); err != nil {
		t.Fatal(err)
	}
	if got := r.storedState(fresh); got.Loop || got.Grounding != nil || got.Guard.InvestigationCalls != 8 {
		t.Fatalf("a new run did not take the new settings: loop %v, grounding %+v, calls %d", got.Loop, got.Grounding, got.Guard.InvestigationCalls)
	}
}
