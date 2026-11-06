package health_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestRunRecordsEachDashboardsProblemsAndListShowsOnlyUnhealthyOnes(t *testing.T) {
	r := newRig(t)
	healthy := r.create(t, boardSpec("Healthy", "up"))
	empty := r.create(t, boardSpec("Empty", "up", "empty_series"))
	failing := r.create(t, boardSpec("Failing", "fail_series"))
	variable := r.create(t, variableSpec("Variable"))
	viewer := r.create(t, boardSpec("Per viewer", `up{user="${__user.login}"}`, "empty_series"))
	r.broken.Store(true)

	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	items := r.list(t, r.editor, "")
	for _, id := range []pgtype.UUID{healthy, viewer} {
		if _, listed := items[id.String()]; listed {
			t.Errorf("dashboard %s without problems is listed", id.String())
		}
	}
	want := map[pgtype.UUID][]health.Kind{
		empty:    {health.KindNoData},
		failing:  {health.KindQueryFailed},
		variable: {health.KindVariableFailed},
	}
	for id, kindsWanted := range want {
		it, ok := items[id.String()]
		if !ok {
			t.Errorf("dashboard %s is not listed", id.String())
			continue
		}
		if !slices.Equal(kinds(it), kindsWanted) {
			t.Errorf("%s problems = %+v, want kinds %v", it.Title, it.Problems, kindsWanted)
		}
		if it.CheckedVersion == nil || *it.CheckedVersion != 1 || it.CheckedAt == nil || !it.CanEdit {
			t.Errorf("%s checked version %v at %v, can edit %v", it.Title, it.CheckedVersion, it.CheckedAt, it.CanEdit)
		}
	}
	if p := items[empty.String()].Problems[0]; p.PanelID != "p1" || p.PanelTitle != "Panel 1" || p.Code != "query_empty" {
		t.Errorf("no data problem = %+v", p)
	}
	if p := items[variable.String()].Problems[0]; p.Variable != "pod" {
		t.Errorf("variable problem = %+v", p)
	}

	var skipped string
	r.scan(t, `SELECT skipped FROM dashboard_health WHERE dashboard_id = $1`, viewer.String(), &skipped)
	if skipped != health.SkippedDependsOnViewer {
		t.Errorf("per viewer dashboard skipped = %q", skipped)
	}
}

func TestRunChecksADashboardAgainOnlyAfterADay(t *testing.T) {
	r := newRig(t)
	id := r.create(t, boardSpec("Board", "up"))
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkedAt := func() time.Time {
		var at time.Time
		r.scan(t, `SELECT checked_at FROM dashboard_health WHERE dashboard_id = $1`, id.String(), &at)
		return at
	}
	first := checkedAt()
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if again := checkedAt(); !again.Equal(first) {
		t.Errorf("a second run rechecked the dashboard: %v then %v", first, again)
	}
	r.db.Exec(t, `UPDATE dashboard_health SET checked_at = now() - interval '24 hours' WHERE dashboard_id = $1`, id.String())
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if again := checkedAt(); !again.After(first) {
		t.Errorf("a day old check was not run again: %v", again)
	}
}

func TestListFindsDuplicateTitlesAndUnopenedDashboardsAndFiltersByKind(t *testing.T) {
	r := newRig(t)
	first := r.create(t, boardSpec("Checkout", "up"))
	second := r.create(t, boardSpec("checkout", "up"))
	old := r.create(t, boardSpec("Old", "up"))
	opened := r.create(t, boardSpec("Opened", "up"))
	r.db.Exec(t, `UPDATE dashboards SET created_at = now() - interval '100 days' WHERE id IN ($1, $2)`, old.String(), opened.String())
	if err := r.svc.RecordView(t.Context(), r.viewer, opened); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.RecordView(t.Context(), r.key, old); err != nil {
		t.Fatal(err)
	}

	items := r.list(t, r.editor, "")
	for _, id := range []pgtype.UUID{first, second} {
		if got := kinds(items[id.String()]); !slices.Equal(got, []health.Kind{health.KindDuplicateTitle}) {
			t.Errorf("%s kinds = %v, want duplicate_title", id.String(), got)
		}
	}
	if got := kinds(items[old.String()]); !slices.Equal(got, []health.Kind{health.KindUnopened}) {
		t.Errorf("old kinds = %v, want unopened (a key's open does not count)", got)
	}
	if _, listed := items[opened.String()]; listed {
		t.Error("a dashboard opened today is listed")
	}
	if it := items[old.String()]; it.LastViewedAt != nil {
		t.Errorf("a key's open was recorded at %v", it.LastViewedAt)
	}

	unopened := r.list(t, r.editor, health.KindUnopened)
	if len(unopened) != 1 || unopened[old.String()].ID == "" {
		t.Errorf("kind unopened lists %d dashboards", len(unopened))
	}
	if noData := r.list(t, r.editor, health.KindNoData); len(noData) != 0 {
		t.Errorf("kind no_data lists %d dashboards", len(noData))
	}
}

func TestListPagesWithACursor(t *testing.T) {
	r := newRig(t)
	for range 3 {
		r.create(t, boardSpec("Same", "up"))
	}
	seen := map[string]bool{}
	cursor := ""
	for range 3 {
		page, err := r.svc.List(t.Context(), r.editor, health.ListParams{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range page.Items {
			seen[it.ID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 3 || cursor != "" {
		t.Errorf("paging saw %d dashboards, last cursor %q", len(seen), cursor)
	}
	if _, err := r.svc.List(t.Context(), r.editor, health.ListParams{Limit: 2, Cursor: "not a cursor"}); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}

func TestRepairQueuesOneRevisionNamingTheFailingPanels(t *testing.T) {
	r := newRig(t)
	empty := r.create(t, boardSpec("Empty", "up", "empty_series"))
	healthy := r.create(t, boardSpec("Healthy", "up"))
	r.broken.Store(true)
	if err := r.svc.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	missing := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	out, err := r.svc.Repair(t.Context(), r.editor, []pgtype.UUID{empty, healthy, missing, empty})
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

	again, err := r.svc.Repair(t.Context(), r.editor, []pgtype.UUID{empty})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(again.Skipped, []repairs.Skipped{{DashboardID: empty.String(), Reason: repairs.ReasonActive}}) {
		t.Errorf("second repair = %+v, want active", again)
	}

	asViewer, err := r.svc.Repair(t.Context(), r.viewer, []pgtype.UUID{healthy})
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
