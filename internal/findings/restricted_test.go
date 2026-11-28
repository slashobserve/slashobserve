package findings_test

import (
	"errors"
	"testing"

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

func TestAFindingThatReadAdminOnlyDataIsNotMadeADashboardInTheDefaultFolder(t *testing.T) {
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
	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), admin, id); !errors.Is(err, folders.ErrRestrictedAnswer) {
		t.Fatalf("save as a dashboard in the default folder: %v, want the restricted answer refusal", err)
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

func TestAnAnswerAboutADashboardOnlyAdminsReadIsMadeADashboardOnlyAfterItMovesOut(t *testing.T) {
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
	r.db.Exec(t, `UPDATE dashboards SET folder_id = (SELECT id FROM folders WHERE general) WHERE id = $1`, r.open)
	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), admin, id); err != nil {
		t.Fatalf("save as a dashboard after the dashboard moved out: %v", err)
	}
}

func TestAFindingAboutADashboardOnlyAdminsReadSaysItIsRestrictedUntilTheDashboardMovesOut(t *testing.T) {
	r := newRig(t)
	request := r.investigation(t, admin, r.open)
	notes := panelOf(t, `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"Root changed it"}`)
	f := r.save(t, admin, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Evidence: []spec.Panel{notes}})
	id := mustUUID(t, f.ID)
	locked := r.folder(t, "Audit")
	r.db.Exec(t, `UPDATE folders SET admins_only = true WHERE id = $1`, locked)

	for _, c := range []struct {
		name   string
		folder string
		want   bool
	}{
		{"in a folder only Admins read", "locked", true},
		{"after it moved to General", "general", false},
	} {
		if c.folder == "locked" {
			r.db.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, r.open, locked)
		} else {
			r.db.Exec(t, `UPDATE dashboards SET folder_id = general_folder_id() WHERE id = $1`, r.open)
		}
		got, err := r.service.Get(reading(t.Context()), admin, id)
		if err != nil || got.Restricted != c.want {
			t.Fatalf("the dashboard %s: restricted %v, want %v, %v", c.name, got.Restricted, c.want, err)
		}
	}
}
