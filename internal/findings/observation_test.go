package findings_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
)

var errAfterConclusion = errors.New("after the conclusion")

type failingAfterConclusion struct{ findings.Observations }

func (f failingAfterConclusion) WriteConclusion(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, c findings.Conclusion) (pgtype.UUID, bool, error) {
	if _, _, err := f.Observations.WriteConclusion(ctx, q, by, c); err != nil {
		return pgtype.UUID{}, false, err
	}
	return pgtype.UUID{}, false, errAfterConclusion
}

func (r rig) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r rig) openRunObservation(t *testing.T, by auth.Principal, request pgtype.UUID) pgtype.UUID {
	t.Helper()
	run, err := r.db.Queries().GetAgentRequestByID(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	var id pgtype.UUID
	err = r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		var err error
		id, err = r.observations.OpenForRun(t.Context(), q, observations.RunOpen{Run: run, By: by, Strict: true})
		if err != nil {
			return err
		}
		_, err = q.SetRequestObservation(t.Context(), dbsqlc.SetRequestObservationParams{ID: request, ObservationID: id})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestServiceNeedsAnObservationWriter(t *testing.T) {
	d := newRig(t).db
	access := folders.NewAccess(d.DB)
	if _, err := findings.New(d.DB, access, annotations.New(d.DB, access), nil, nil, slog.New(slog.DiscardHandler)); !errors.Is(err, findings.ErrNoObservations) {
		t.Fatalf("error %v, want %v", err, findings.ErrNoObservations)
	}
}

func TestAFindingTakesItsObservationsIdAndLeavesItsOwnFolderUnset(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, findings.SaveInput{})
	if saved.ID != saved.ObservationID {
		t.Fatalf("finding %s in observation %s", saved.ID, saved.ObservationID)
	}
	if n := r.count(t, `SELECT count(*) FROM findings WHERE id = $1::uuid AND observation_id = id AND folder_id IS NULL`, saved.ID); n != 1 {
		t.Fatalf("a finding row with its observation id and no folder: %d", n)
	}
	if n := r.count(t, `SELECT count(*) FROM observation_blocks WHERE observation_id = $1::uuid AND kind = 'text'`, saved.ObservationID); n != 1 {
		t.Fatalf("text blocks: %d", n)
	}
}

func TestAFindingFromARunWritesIntoTheRunsObservationOnceAndGetsItsOwnId(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, r.open)
	observation := r.openRunObservation(t, editor, request)

	first := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open})
	if first.ObservationID != observation.String() || first.ID == first.ObservationID {
		t.Fatalf("finding %s in observation %s, want a new id in %s", first.ID, first.ObservationID, observation)
	}
	again, replayed, err := r.service.Save(t.Context(), editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Title: "why", Markdown: "second", From: windowTo.Add(-1), To: windowTo})
	if err != nil || !replayed || again.ID != first.ID {
		t.Fatalf("second save: %+v replayed %v, %v", again, replayed, err)
	}
	if n := r.count(t, `SELECT count(*) FROM observation_blocks WHERE observation_id = $1 AND kind = 'text'`, observation); n != 1 {
		t.Fatalf("text blocks after a replay: %d", n)
	}
}

func TestAnAnswerRunOnADashboardFilesItsObservationInTheDashboardsFolder(t *testing.T) {
	r := newRig(t)
	folder := r.folder(t, "Payments")
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, r.open, folder)
	request := r.request(t, editor, r.open)

	saved := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open})
	run, err := r.db.Queries().GetAgentRequestByID(t.Context(), request)
	if err != nil || !run.ObservationID.Valid || run.ObservationID.String() != saved.ObservationID {
		t.Fatalf("run observation %v, finding observation %s, %v", run.ObservationID, saved.ObservationID, err)
	}
	if n := r.count(t, `SELECT count(*) FROM observations WHERE id = $1::uuid AND folder_id = $2`, saved.ObservationID, folder); n != 1 {
		t.Fatalf("observation is not in the dashboard's folder %s", folder)
	}
}

func TestAFailureAfterTheConclusionLeavesNeitherObservationNorFinding(t *testing.T) {
	r := newRig(t)
	access := folders.NewAccess(r.db.DB)
	service, err := findings.New(r.db.DB, access, annotations.New(r.db.DB, access), nil, failingAfterConclusion{r.observations}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Save(t.Context(), editor, findings.SaveInput{Title: "t", Markdown: "m", From: windowTo.Add(-1), To: windowTo})
	if !errors.Is(err, errAfterConclusion) {
		t.Fatalf("save: %v", err)
	}
	if n := r.count(t, `SELECT count(*) FROM findings`) + r.count(t, `SELECT count(*) FROM observations`); n != 0 {
		t.Fatalf("%d rows left behind", n)
	}
}

func TestMovingTheObservationMovesWhereTheFindingLists(t *testing.T) {
	r := newRig(t)
	shared := r.folder(t, "Incidents")
	saved := r.save(t, editor, findings.SaveInput{})
	if _, err := r.observations.Move(t.Context(), editor, mustUUID(t, saved.ObservationID), shared); err != nil {
		t.Fatal(err)
	}
	rows, _, err := r.service.List(t.Context(), viewer, findings.ListFilter{FolderID: shared}, 10, "")
	if err != nil || len(rows) != 1 || rows[0].ID != saved.ID || rows[0].FolderID != shared.String() {
		t.Fatalf("folder list: %+v, %v", rows, err)
	}
	got, err := r.service.Get(t.Context(), viewer, mustUUID(t, saved.ID))
	if err != nil || got.Folder == nil || got.Folder.ID != shared {
		t.Fatalf("read: %+v, %v", got.Folder, err)
	}
}
