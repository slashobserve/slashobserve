package grafanaconn_test

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/grafanaconn"
)

func importedFromGrafana(t *testing.T, r *rig) string {
	t.Helper()
	threeFolders(r.grafana)
	r.grafana.add(&fakeDashboard{uid: "checkout", title: "Checkout", folder: "f-apps", model: simpleModel("graf-prom", "prometheus"), version: 1})
	done := r.finish(t, r.connect(t), grafanaconn.SyncRequest{All: true})
	if len(done.Results) != 1 || done.Results[0].DashboardID == "" {
		t.Fatalf("sync %+v", done)
	}
	return done.Results[0].DashboardID
}

func TestResolveFindsTheImportedDashboard(t *testing.T) {
	r := newRig(t)
	dashboardID := importedFromGrafana(t, r)
	for _, who := range []string{"admin", "viewer"} {
		by := r.admin
		if who == "viewer" {
			by = r.viewer
		}
		got, err := r.svc.Resolve(t.Context(), by, "checkout")
		if err != nil || got.String() != dashboardID {
			t.Fatalf("%s resolves %v, err %v; want %s", who, got, err, dashboardID)
		}
	}
	if _, err := r.svc.Resolve(t.Context(), r.viewer, "never-imported"); !errors.Is(err, grafanaconn.ErrUnknownUID) {
		t.Fatalf("unknown uid: %v", err)
	}
}

func TestResolveHidesADashboardInAFolderTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	dashboardID := importedFromGrafana(t, r)
	r.db.Exec(t, "INSERT INTO folder_permissions (folder_id, group_name, level) SELECT folder_id, 'secret-team', 'view' FROM dashboards WHERE id = $1", dashboardID)

	if _, err := r.svc.Resolve(t.Context(), r.viewer, "checkout"); !errors.Is(err, grafanaconn.ErrUnknownUID) {
		t.Fatalf("viewer outside the folder: %v, want ErrUnknownUID", err)
	}
	if got, err := r.svc.Resolve(t.Context(), r.admin, "checkout"); err != nil || got.String() != dashboardID {
		t.Fatalf("the folder's creator resolves %v, err %v", got, err)
	}
}
