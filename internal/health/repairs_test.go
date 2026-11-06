package health_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestRepairQueuesOneRevisionNamingTheFailingPanels(t *testing.T) {
	r := newRig(t)
	empty := r.create(t, boardSpec("Empty", "up", "empty_series"))
	healthy := r.create(t, boardSpec("Healthy", "up"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	missing := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	out, err := r.repairs.Repair(t.Context(), r.editor, []pgtype.UUID{empty, healthy, missing, empty})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Queued) != 1 || out.Queued[0].DashboardID != empty.String() {
		t.Fatalf("queued = %+v", out.Queued)
	}
	wantSkipped := []repairs.Skipped{
		{DashboardID: healthy.String(), Reason: repairs.ReasonNothingToRepair},
		{DashboardID: missing.String(), Reason: repairs.ReasonNotFound},
	}
	if !slices.Equal(out.Skipped, wantSkipped) {
		t.Errorf("skipped = %+v, want %+v", out.Skipped, wantSkipped)
	}
	var prompt, source string
	r.scan(t, `SELECT prompt, source FROM agent_requests WHERE id = $1`, out.Queued[0].RequestID, &prompt, &source)
	if source != string(revisions.SourcePrompt) || !strings.HasPrefix(prompt, "Repair the panels and variables the health check found failing") || !strings.Contains(prompt, `- panel p1 ("Panel 1") returns no data`) {
		t.Errorf("source %q prompt %q", source, prompt)
	}

	again, err := r.repairs.Repair(t.Context(), r.editor, []pgtype.UUID{empty})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(again.Skipped, []repairs.Skipped{{DashboardID: empty.String(), Reason: repairs.ReasonActive}}) {
		t.Errorf("second repair = %+v, want active", again)
	}

	asViewer, err := r.repairs.Repair(t.Context(), r.viewer, []pgtype.UUID{healthy})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(asViewer.Skipped, []repairs.Skipped{{DashboardID: healthy.String(), Reason: repairs.ReasonForbidden}}) {
		t.Errorf("viewer repair = %+v, want forbidden_role", asViewer)
	}

	if _, err := r.noAgent.Repair(t.Context(), r.editor, []pgtype.UUID{empty}); err == nil {
		t.Error("a repair without an agent was accepted")
	}
}
