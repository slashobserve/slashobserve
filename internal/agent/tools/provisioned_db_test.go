package tools

import (
	"errors"
	"fmt"
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

func TestGetDashboardNamesWhatManagesItAndAFileDashboardWarnsOnSaveOverMCP(t *testing.T) {
	r := newAccessRig(t)
	plain, bound := r.seed(t, "Plain board"), r.seed(t, "File board")
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `INSERT INTO provisioned_dashboards (path, dashboard_id, content_sha256, folder_title) VALUES ('a.json', $1, 'sha', 'General')`, bound.String()); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id                 pgtype.UUID
		managedBy, warning string
	}{
		{plain, "workspace", ""},
		{bound, "file", folders.ManagedByFile.EditWarning()},
		{r.starter(t), "starter", ""},
	}
	for _, tc := range cases {
		t.Run(tc.managedBy, func(t *testing.T) {
			res := r.call(t, adminCaller, nameGetDashboard, `{"dashboard_id":"`+tc.id.String()+`"}`)
			read, ok := res.Content.(dashboardOut)
			if res.IsError || !ok {
				t.Fatalf("get_dashboard: %s", textOf(t, res))
			}
			if read.ManagedBy != tc.managedBy {
				t.Fatalf("managed_by = %q, want %q", read.ManagedBy, tc.managedBy)
			}
			validateOutput(t, r.registry, nameGetDashboard, read)
			if tc.managedBy == "starter" {
				return
			}
			update := `{"dashboard_id":"` + tc.id.String() + `","parent_version":1,"summary":"s","spec":` + boardSpec("Renamed "+tc.managedBy) + `}`
			res = r.call(t, adminCaller, nameUpdateDashboard, update)
			saved, ok := res.Content.(savedOut)
			if res.IsError || !ok {
				t.Fatalf("update_dashboard: %s", textOf(t, res))
			}
			if saved.Warning != tc.warning {
				t.Fatalf("update_dashboard warning = %q, want %q", saved.Warning, tc.warning)
			}
		})
	}
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
	res, err := saveFailure(t.Context(), folders.ErrProvisioned)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != folders.CodeProvisioned {
		t.Fatalf("code = %q", got)
	}
	if _, err := saveFailure(t.Context(), errors.New("other")); err == nil {
		t.Fatal("an unknown error must not be reported as a tool result")
	}
}

func TestStartingOrApplyingARunOnAStarterNamesTheProvisionedRefusal(t *testing.T) {
	err := fmt.Errorf("check the run: %w", folders.ErrProvisioned)
	applied, ok := applyRefusal(t.Context(), err)
	if !ok {
		t.Fatal("apply_suggested_change does not report the provisioned refusal as a tool result")
	}
	started, failed := runFailure(t.Context(), err)
	if failed != nil {
		t.Fatalf("start_run: %v, want a tool result", failed)
	}
	for name, res := range map[string]Result{"apply_suggested_change": applied, "start_run": started} {
		if got := errorCode(t, res); got != folders.CodeProvisioned {
			t.Errorf("%s: code = %q, want provisioned", name, got)
		}
	}
}

func (r *accessRig) keepAnEmptyPanelUnverified(t *testing.T, id pgtype.UUID) {
	t.Helper()
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `UPDATE dashboard_versions SET unverified = '{"panels":[{"panel_id":"p1","issues":[{"path":"/panels/0/queries","code":"query_empty","message":"returned no data"}]}],"variables":[]}'::jsonb WHERE dashboard_id = $1`, id.String()); err != nil {
		t.Fatal(err)
	}
}

func unverifiedPanels(t *testing.T, res Result) int {
	t.Helper()
	out, ok := res.Content.(dashboardOut)
	if res.IsError || !ok {
		t.Fatalf("get_dashboard: %s", textOf(t, res))
	}
	if out.Unverified == nil {
		return 0
	}
	return len(out.Unverified.Panels)
}

func TestAStarterDoesNotShowAnEmptyPanelAsUnverifiedButItsCopyDoes(t *testing.T) {
	r := newAccessRig(t)
	starter := r.starter(t)
	ordinary := r.seed(t, "Ordinary board")
	r.keepAnEmptyPanelUnverified(t, starter)
	r.keepAnEmptyPanelUnverified(t, ordinary)

	if n := unverifiedPanels(t, r.call(t, adminCaller, nameGetDashboard, `{"dashboard_id":"`+starter.String()+`"}`)); n != 0 {
		t.Errorf("the starter shows %d unverified panels, want none", n)
	}
	if n := unverifiedPanels(t, r.call(t, adminCaller, nameGetDashboard, `{"dashboard_id":"`+ordinary.String()+`"}`)); n != 1 {
		t.Errorf("an ordinary dashboard shows %d unverified panels, want 1", n)
	}
	res := r.call(t, adminCaller, nameDuplicateDashboard, `{"dashboard_id":"`+starter.String()+`","title":"My starter"}`)
	saved, ok := res.Content.(savedOut)
	if res.IsError || !ok {
		t.Fatalf("duplicate_dashboard: %s", textOf(t, res))
	}
	if n := unverifiedPanels(t, r.call(t, adminCaller, nameGetDashboard, `{"dashboard_id":"`+saved.DashboardID+`"}`)); n != 1 {
		t.Errorf("the copy shows %d unverified panels, want the 1 it inherited", n)
	}
}
