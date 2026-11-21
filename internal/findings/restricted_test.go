package findings_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestTheJSONOfAFindingSaysWhetherItsRunReadAdminOnlyData(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		if got := findings.JSONOf(findings.Finding{ID: "f", Restricted: restricted}, "").Restricted; got != restricted {
			t.Errorf("restricted %v renders as %v", restricted, got)
		}
	}
}

func TestAFindingThatReadAdminOnlyDataIsPinnedOrMadeADashboardOnlyWhereOnlyAdminsRead(t *testing.T) {
	r := newRig(t)
	request := r.investigation(t, admin, r.open)
	notes := panelOf(t, `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"Root changed it"}`)
	f := r.save(t, admin, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Evidence: []spec.Panel{notes}})
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = true WHERE id = $1`, request)
	id := mustUUID(t, f.ID)

	got, err := r.service.Get(reading(t.Context()), admin, id)
	if err != nil || !got.Restricted {
		t.Fatalf("the finding of a run that read Admin-only data: restricted %v, %v", got.Restricted, err)
	}
	if _, _, err := r.service.Pin(reading(t.Context()), admin, id); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("pin onto a shared dashboard: %v, want the restricted answer refusal", err)
	}
	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), admin, id); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("save as a dashboard in the default folder: %v, want the restricted answer refusal", err)
	}

	locked := r.folder(t, "Audit")
	r.db.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, locked)
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, r.open, locked)
	if _, created, err := r.service.Pin(reading(t.Context()), admin, id); err != nil || !created {
		t.Fatalf("pin onto a dashboard only Admins read: created %v, %v", created, err)
	}
}

func TestAPinOfAFindingThatReadAdminOnlyDataChecksTheFolderInTheAnnotationsTransaction(t *testing.T) {
	r := newRig(t)
	request := r.investigation(t, admin, r.open)
	notes := panelOf(t, `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"Root changed it"}`)
	f := r.save(t, admin, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Evidence: []spec.Panel{notes}})
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = true WHERE id = $1`, request)
	locked := r.folder(t, "Audit")
	r.db.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, locked)
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, r.open, locked)

	sqlDB := r.db.SQLDB(t)
	move, err := sqlDB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = move.Rollback() }()
	if _, err := move.ExecContext(t.Context(), `UPDATE dashboards SET folder_id = (SELECT id FROM folders WHERE general) WHERE id = $1`, r.open); err != nil {
		t.Fatal(err)
	}

	id := mustUUID(t, f.ID)
	pinned := make(chan error, 1)
	go func() {
		_, _, err := r.service.Pin(reading(context.Background()), admin, id)
		pinned <- err
	}()
	deadline := time.Now().Add(15 * time.Second)
	for waiting := 0; waiting == 0; {
		select {
		case err := <-pinned:
			t.Fatalf("the pin finished while the dashboard's move to General was uncommitted: %v; it did not wait for the folder in its own transaction", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the pin neither finished nor waited for the dashboard's folder")
		}
		if err := sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := move.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-pinned; !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("pin after the dashboard moved to General: %v, want the restricted answer refusal", err)
	}
	var annotations int
	if err := sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM annotations WHERE dashboard_id = $1`, r.open).Scan(&annotations); err != nil {
		t.Fatal(err)
	}
	if annotations != 0 {
		t.Fatalf("annotations on the dashboard = %d, want none", annotations)
	}
}

func TestADashboardMadeFromAnAnswerRecordsTheRunItsPanelsCameFrom(t *testing.T) {
	r := newRig(t)
	request := r.investigation(t, admin, r.open)
	notes := panelOf(t, `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"Root changed it"}`)
	f := r.save(t, admin, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Evidence: []spec.Panel{notes}})

	dash, _, err := r.service.SaveAsDashboard(reading(t.Context()), admin, mustUUID(t, f.ID))
	if err != nil {
		t.Fatal(err)
	}
	var run, via string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT coalesce(created_run_id::text, ''), created_via FROM dashboards WHERE id = $1`, dash.ID).Scan(&run, &via); err != nil {
		t.Fatal(err)
	}
	if run != request.String() || via != "direct" {
		t.Fatalf("created_run_id %q via %q, want the answer's run %s made directly", run, via, request.String())
	}
}

func TestAnAnswerAboutADashboardOnlyAdminsReadIsPinnedOnItButMadeADashboardOnlyAfterItMovesOut(t *testing.T) {
	r := newRig(t)
	request := r.investigation(t, admin, r.open)
	notes := panelOf(t, `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"Root changed it"}`)
	f := r.save(t, admin, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Evidence: []spec.Panel{notes}})
	locked := r.folder(t, "Audit")
	r.db.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, locked)
	r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, r.open, locked)
	id := mustUUID(t, f.ID)

	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), admin, id); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("save as a dashboard in the default folder: %v, want the restricted answer refusal", err)
	}
	if _, created, err := r.service.Pin(reading(t.Context()), admin, id); err != nil || !created {
		t.Fatalf("pin onto the dashboard it is about: created %v, %v", created, err)
	}
	r.db.Exec(t, `UPDATE dashboards SET folder_id = (SELECT id FROM folders WHERE general) WHERE id = $1`, r.open)
	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), admin, id); err != nil {
		t.Fatalf("save as a dashboard after the dashboard moved out: %v", err)
	}
}
