package observations_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
)

func (r rig) generalFolder(t *testing.T) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT id::text FROM folders WHERE general`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func (r rig) conclude(t *testing.T, by auth.Principal, c findings.Conclusion) (pgtype.UUID, bool, error) {
	t.Helper()
	var id pgtype.UUID
	var created bool
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		var err error
		id, created, err = r.service.WriteConclusion(t.Context(), q, by, c)
		return err
	})
	return id, created, err
}

func TestARunWithoutAnObservationGetsOneInTheDashboardsFolderAndIsLinkedToIt(t *testing.T) {
	r := newRig(t)
	dashboard := r.dashboardIn(t, r.shared)
	run := r.run(t, editor, dashboard, "Why?", nil, nil)
	c := recordInput(t, run, evidencePanel)
	c.DashboardID = dashboard
	id, created, err := r.conclude(t, editor, c)
	if err != nil || !created {
		t.Fatalf("conclusion: created %v, %v", created, err)
	}
	got, err := r.service.Get(t.Context(), editor, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID != r.shared.String() || got.Title != "Checkout 5xx" || got.State != observations.StateOpen || got.RunID != run.ID.String() {
		t.Errorf("observation %+v", got)
	}
	if got.From == nil || !got.From.Equal(windowTo.Add(-time.Hour)) || got.To == nil || !got.To.Equal(windowTo) {
		t.Errorf("range %v to %v", got.From, got.To)
	}
	if len(got.Blocks) != 2 || got.Blocks[0].Kind != observations.KindText || got.Blocks[1].Kind != observations.KindPanel {
		t.Errorf("blocks %+v", got.Blocks)
	}
	var linked string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT observation_id::text FROM agent_requests WHERE id = $1::uuid`, run.ID.String()).Scan(&linked); err != nil || linked != id.String() {
		t.Errorf("run links %q, %v, want %s", linked, err, id)
	}
	again, createdAgain, err := r.conclude(t, editor, c)
	if err != nil || createdAgain || again != id {
		t.Errorf("a second conclusion: %v, created %v, %v", again, createdAgain, err)
	}
	if after, err := r.service.Get(t.Context(), editor, id); err != nil || len(after.Blocks) != 2 {
		t.Errorf("blocks after the second conclusion %d, %v", len(after.Blocks), err)
	}
}

func TestAConclusionWithoutARunLandsWhereTheCallerAsksOrWhereTheyCreate(t *testing.T) {
	r := newRig(t)
	dashboard := r.dashboardIn(t, r.shared)
	requested := r.folder(t, "Requested")
	conclusion := func(folder, dashboard pgtype.UUID) findings.Conclusion {
		return findings.Conclusion{FolderID: folder, DashboardID: dashboard, Title: "Slow pod", Markdown: "checkout-0 is slow", From: windowTo.Add(-time.Hour), To: windowTo}
	}
	cases := []struct {
		name string
		by   auth.Principal
		in   findings.Conclusion
		want pgtype.UUID
	}{
		{"a requested folder", editor, conclusion(requested, pgtype.UUID{}), requested},
		{"no folder, a user", editor, conclusion(pgtype.UUID{}, pgtype.UUID{}), r.personalFolder(t, editor)},
		{"no folder, a key", key, conclusion(pgtype.UUID{}, pgtype.UUID{}), r.generalFolder(t)},
		{"the folder of a dashboard the caller edits", editor, conclusion(pgtype.UUID{}, dashboard), r.shared},
		{"a dashboard the caller cannot edit", viewer, conclusion(pgtype.UUID{}, dashboard), r.personalFolder(t, viewer)},
	}
	for _, c := range cases {
		id, created, err := r.conclude(t, c.by, c.in)
		if err != nil || !created {
			t.Errorf("%s: created %v, %v", c.name, created, err)
			continue
		}
		got, err := r.service.Get(t.Context(), c.by, id)
		if err != nil || got.FolderID != c.want.String() || got.Title != "Slow pod" || len(got.Blocks) != 1 || got.Blocks[0].State != observations.BlockLive {
			t.Errorf("%s: folder %q, blocks %+v, %v, want folder %s", c.name, got.FolderID, got.Blocks, err, c.want)
		}
	}
}

func TestAConclusionIntoAFolderNeedsCreateThereAndSeesTheFolder(t *testing.T) {
	r := newRig(t)
	open := r.folder(t, "Open")
	hidden := r.hiddenFolder(t)
	conclusion := func(folder pgtype.UUID) findings.Conclusion {
		return findings.Conclusion{FolderID: folder, Title: "t", Markdown: "m", From: windowTo.Add(-time.Hour), To: windowTo}
	}
	if _, _, err := r.conclude(t, viewer, conclusion(open)); !errors.Is(err, folders.ErrForbidden) {
		t.Errorf("a viewer's requested folder: %v", err)
	}
	if _, _, err := r.conclude(t, editor, conclusion(hidden)); !errors.Is(err, findings.ErrFolderNotFound) {
		t.Errorf("a folder the caller cannot see: %v", err)
	}
	if _, _, err := r.conclude(t, editor, conclusion(mustUUID(t, "99999999-9999-4999-8999-999999999999"))); !errors.Is(err, findings.ErrFolderNotFound) {
		t.Errorf("a folder that does not exist: %v", err)
	}
}

func TestAConclusionForAnUnknownRunFails(t *testing.T) {
	r := newRig(t)
	unknown := findings.Conclusion{RunID: mustUUID(t, "00000000-0000-4000-8000-000000000000"), Title: "t", Markdown: "m", From: windowTo.Add(-time.Hour), To: windowTo}
	if _, _, err := r.conclude(t, editor, unknown); !db.IsForeignKeyViolation(err) {
		t.Errorf("an unknown run: %v", err)
	}
}

func TestAFailingConclusionLeavesNoObservationBehind(t *testing.T) {
	r := newRig(t)
	boom := errors.New("boom")
	err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		if _, _, err := r.service.WriteConclusion(t.Context(), q, editor, findings.Conclusion{Title: "t", Markdown: "m", From: windowTo.Add(-time.Hour), To: windowTo}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM observations`).Scan(&n); err != nil || n != 0 {
		t.Errorf("observations = %d, %v", n, err)
	}
}

func TestPlacementNamesTheObservationsFolderAndHidesWhatTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	o := r.create(t, editor, r.shared)
	folder, abilities, err := r.service.Placement(t.Context(), viewer, mustUUID(t, o.ID))
	if err != nil || folder.ID != r.shared || !abilities.View || abilities.Edit {
		t.Errorf("a viewer: %+v, %+v, %v", folder, abilities, err)
	}
	hidden := r.create(t, admin, r.hiddenFolder(t))
	if _, _, err := r.service.Placement(t.Context(), viewer, mustUUID(t, hidden.ID)); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("a hidden observation: %v", err)
	}
	if _, _, err := r.service.Placement(t.Context(), viewer, mustUUID(t, "99999999-9999-4999-8999-999999999999")); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("a missing observation: %v", err)
	}
}
