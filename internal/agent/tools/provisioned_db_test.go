package tools

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/folders"
)

func (r *accessRig) starter(t *testing.T) pgtype.UUID {
	t.Helper()
	id := r.seed(t, "Starter board")
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `INSERT INTO starter_content (key, ref) VALUES ('audit/dashboards/test', $1)`, id.String()); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAStarterDashboardRefusesEveryWriteOverMCP(t *testing.T) {
	r := newAccessRig(t)
	id := r.starter(t).String()
	general := r.newFolder(t, "Ops").String()
	writes := map[string]struct{ tool, input string }{
		"update the dashboard":   {nameUpdateDashboard, `{"dashboard_id":"` + id + `","parent_version":1,"summary":"s","spec":` + boardSpec("Changed") + `}`},
		"patch panels":           {nameUpdatePanels, `{"dashboard_id":"` + id + `","parent_version":1,"summary":"s","upsert":[` + statPanel("p_extra", 8) + `]}`},
		"restore a version":      {nameRestoreDashboardVersion, `{"dashboard_id":"` + id + `","version":1,"parent_version":1}`},
		"move to another folder": {nameMoveDashboard, `{"dashboard_id":"` + id + `","folder_id":"` + general + `"}`},
	}
	for name, w := range writes {
		t.Run(name, func(t *testing.T) {
			res := r.call(t, adminCaller, w.tool, w.input)
			if got := errorCode(t, res); got != folders.CodeProvisioned {
				t.Fatalf("code = %q, want provisioned", got)
			}
			if text := textOf(t, res); !strings.Contains(text, nameDuplicateDashboard) {
				t.Fatalf("the refusal %s does not point to %s", text, nameDuplicateDashboard)
			}
		})
	}
}

func TestAStarterDashboardStillTakesCommentsAnnotationsAndDuplicatesOverMCP(t *testing.T) {
	r := newAccessRig(t)
	id := r.starter(t).String()

	if res := r.call(t, adminCaller, nameCreateComment, `{"target":"dashboard:`+id+`","body":"what does this panel count?"}`); res.IsError {
		t.Errorf("create_comment: %s", textOf(t, res))
	}
	if res := r.call(t, adminCaller, nameCreateAnnotation, `{"text":"audit review","dashboard_id":"`+id+`"}`); res.IsError {
		t.Errorf("create_annotation: %s", textOf(t, res))
	}
	res := r.call(t, adminCaller, nameDuplicateDashboard, `{"dashboard_id":"`+id+`","title":"My starter"}`)
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok {
		t.Fatalf("duplicate_dashboard: %s", textOf(t, res))
	}
	update := `{"dashboard_id":"` + saved.DashboardID + `","parent_version":1,"summary":"s","spec":` + boardSpec("My starter renamed") + `}`
	if res := r.call(t, adminCaller, nameUpdateDashboard, update); res.IsError {
		t.Errorf("updating the copy: %s", textOf(t, res))
	}
}

func TestSaveFailureNamesAProvisionedRefusal(t *testing.T) {
	res, err := saveFailure(folders.ErrProvisioned)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != folders.CodeProvisioned {
		t.Fatalf("code = %q", got)
	}
	if _, err := saveFailure(errors.New("other")); err == nil {
		t.Fatal("an unknown error must not be reported as a tool result")
	}
}
