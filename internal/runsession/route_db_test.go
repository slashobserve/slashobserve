package runsession

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) storedTask(req revisions.Request) route.Task {
	r.t.Helper()
	var task string
	if err := r.d.SQLDB(r.t).QueryRow("SELECT coalesce(state->>'task', '') FROM run_sessions WHERE agent_request_id = $1", mustUUID(r.t, req.ID)).Scan(&task); err != nil {
		r.t.Fatal(err)
	}
	return route.Task(task)
}

func TestAnInvestigateRunIsRoutedOnceWithoutItsQuestionInTheStep(t *testing.T) {
	r := newRig(t)
	req := r.investigate()
	r.read(req, 1)
	r.read(req, 2)
	if got := r.storedTask(req); got != route.Incident {
		t.Fatalf("stored task = %q, want incident", got)
	}
	var routed []string
	for _, m := range r.steps.messages() {
		if strings.HasPrefix(m, "Routed the question") {
			routed = append(routed, m)
		}
	}
	if len(routed) != 1 || routed[0] != "Routed the question to the incident task type" {
		t.Fatalf("route steps = %v", routed)
	}
	if strings.Contains(routed[0], req.Prompt) {
		t.Fatal("the route step carries the question")
	}
}

func TestOnlyInvestigateRunsAreRouted(t *testing.T) {
	r := newRig(t)
	req := r.revise()
	r.read(req, 1)
	if got := r.storedTask(req); got != route.Full {
		t.Fatalf("a revision was routed to %q", got)
	}
	for _, m := range r.steps.messages() {
		if strings.HasPrefix(m, "Routed the question") {
			t.Fatalf("a revision recorded %q", m)
		}
	}
}
