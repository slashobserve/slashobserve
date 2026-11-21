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
