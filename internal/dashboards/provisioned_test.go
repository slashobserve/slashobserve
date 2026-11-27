package dashboards

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (r *rig) markStarter(t *testing.T, id pgtype.UUID) {
	t.Helper()
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `INSERT INTO starter_content (key, ref) VALUES ('audit/dashboards/test', $1)`, id.String()); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) starterDashboard(t *testing.T) pgtype.UUID {
	t.Helper()
	id := r.createDashboard(t)
	r.markStarter(t, id)
	return id
}

func (r *rig) bindToProvisioningFile(t *testing.T, id pgtype.UUID) {
	t.Helper()
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `INSERT INTO provisioned_dashboards (path, dashboard_id, content_sha256, folder_title) VALUES ('a.json', $1, 'sha', 'General')`, id.String()); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) bindToAppliedFile(t *testing.T, id pgtype.UUID) {
	t.Helper()
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `INSERT INTO applied_dashboards (path, dashboard_id, content_sha256, applied_by_id) VALUES ('a.json', $1, 'sha', $2)`, id.String(), adminUser.ID); err != nil {
		t.Fatal(err)
	}
}

type savedWithOrigin struct {
	Dashboard struct {
		Origin      string `json:"origin"`
		Provisioned bool   `json:"provisioned"`
		Permissions struct{ Edit bool }
	}
	Warning string `json:"warning"`
}

func TestADashboardReadsItsOriginAndOnlyAFileDashboardWarnsOnSave(t *testing.T) {
	r := newRig(t, false)
	b := boards{rig: r}
	cases := []struct {
		origin  string
		bind    func(*testing.T, pgtype.UUID)
		warning string
	}{
		{"editable", func(*testing.T, pgtype.UUID) {}, ""},
		{"file", r.bindToProvisioningFile, folders.FileOwnedWarning},
		{"applied", r.bindToAppliedFile, ""},
	}
	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			id := r.createDashboard(t)
			tc.bind(t, id)
			path := "/dashboards/" + id.String()

			var read savedWithOrigin
			rec := b.serve(adminUser, http.MethodGet, path, "")
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &read) != nil {
				t.Fatalf("get answered %d %s", rec.Code, rec.Body.String())
			}
			if read.Dashboard.Origin != tc.origin || read.Dashboard.Provisioned || !read.Dashboard.Permissions.Edit {
				t.Fatalf("dashboard = %+v, want origin %s, not provisioned, editable", read.Dashboard, tc.origin)
			}

			var saved savedWithOrigin
			rec = b.serve(adminUser, http.MethodPost, path+"/versions", `{"spec":`+string(panelsSpec("changed", "rps"))+`,"summary":"s","parent_version":1}`)
			if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &saved) != nil {
				t.Fatalf("save answered %d %s", rec.Code, rec.Body.String())
			}
			if saved.Warning != tc.warning || saved.Dashboard.Origin != tc.origin {
				t.Fatalf("save answered warning %q and origin %q, want %q and %q", saved.Warning, saved.Dashboard.Origin, tc.warning, tc.origin)
			}

			moved, _, err := r.service.Move(reading(t.Context()), testUser, id, r.newFolder(t, "Ops "+tc.origin))
			if err != nil {
				t.Fatal(err)
			}
			if got := moved.Origin.String(); got != tc.origin {
				t.Fatalf("the move answered origin %q, want %q", got, tc.origin)
			}
		})
	}
}

func TestAStarterDashboardReadsAsAStarter(t *testing.T) {
	r := newRig(t, false)
	b := boards{rig: r}
	id := r.starterDashboard(t)
	r.bindToProvisioningFile(t, id)

	var read savedWithOrigin
	rec := b.serve(adminUser, http.MethodGet, "/dashboards/"+id.String(), "")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &read) != nil {
		t.Fatalf("get answered %d %s", rec.Code, rec.Body.String())
	}
	if read.Dashboard.Origin != "starter" || !read.Dashboard.Provisioned {
		t.Fatalf("dashboard = %+v, want origin starter and provisioned", read.Dashboard)
	}
}

func codeOf(t *testing.T, body string) string {
	t.Helper()
	var parsed struct{ Code string }
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return parsed.Code
}

func TestAStarterDashboardRefusesEveryChangeToItsContentOverHTTP(t *testing.T) {
	r := newRig(t, false)
	b := boards{rig: r}
	id := r.starterDashboard(t).String()
	general := r.generalFolder(t).String()
	spec := string(panelsSpec("changed", "rps"))
	panel := `{"id":"extra","type":"timeseries","title":"extra","gridPos":{"x":0,"y":8,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}`
	requests := []struct{ name, method, path, body string }{
		{"save a version", http.MethodPost, "/dashboards/" + id + "/versions", `{"spec":` + spec + `,"summary":"s","parent_version":1}`},
		{"save a version and propose if needed", http.MethodPost, "/dashboards/" + id + "/versions", `{"spec":` + spec + `,"summary":"s","parent_version":1,"propose_if_needed":true}`},
		{"patch panels", http.MethodPost, "/dashboards/" + id + "/panels", `{"upsert":[` + panel + `],"summary":"s","parent_version":1}`},
		{"restore a version", http.MethodPost, "/dashboards/" + id + "/versions/1/restore", `{"parent_version":1}`},
		{"move", http.MethodPatch, "/dashboards/" + id, `{"folder_id":"` + general + `"}`},
	}
	for _, tt := range requests {
		t.Run(tt.name, func(t *testing.T) {
			rec := b.serve(adminUser, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusConflict || codeOf(t, rec.Body.String()) != folders.CodeProvisioned {
				t.Fatalf("answered %d %s, want 409 provisioned", rec.Code, rec.Body.String())
			}
		})
	}
	if n := r.latestVersion(t, id); n != 1 {
		t.Fatalf("the dashboard is at version %d, want 1", n)
	}
}

func (r *rig) latestVersion(t *testing.T, id string) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT latest_version FROM dashboards WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAStarterDashboardIsMarkedReadOnlyAndStillReadableCommentableAndDuplicable(t *testing.T) {
	r := newRig(t, false)
	b := boards{rig: r}
	id := r.starterDashboard(t).String()

	rec := b.serve(adminUser, http.MethodGet, "/dashboards/"+id, "")
	var got struct {
		Dashboard struct {
			Provisioned bool `json:"provisioned"`
			Permissions struct{ Comment, Edit, Manage bool }
		}
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("get answered %d %s", rec.Code, rec.Body.String())
	}
	if !got.Dashboard.Provisioned || got.Dashboard.Permissions.Edit || !got.Dashboard.Permissions.Comment {
		t.Fatalf("dashboard = %+v, want provisioned, not editable, commentable", got.Dashboard)
	}
	for _, path := range []string{"/versions", "/versions/1"} {
		if rec := b.serve(adminUser, http.MethodGet, "/dashboards/"+id+path, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s answered %d %s", path, rec.Code, rec.Body.String())
		}
	}

	rec = b.serve(adminUser, http.MethodPost, "/dashboards/"+id+"/duplicate", `{"title":"Mine"}`)
	var copied struct {
		Dashboard struct {
			ID          string `json:"id"`
			Provisioned bool   `json:"provisioned"`
			Permissions struct{ Edit bool }
		}
	}
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &copied) != nil {
		t.Fatalf("duplicate answered %d %s", rec.Code, rec.Body.String())
	}
	if copied.Dashboard.Provisioned || !copied.Dashboard.Permissions.Edit {
		t.Fatalf("the copy = %+v, want an ordinary editable dashboard", copied.Dashboard)
	}
	spec := strings.Replace(string(panelsSpec("t", "rps")), `"t"`, `"Mine edited"`, 1)
	rec = b.serve(adminUser, http.MethodPost, "/dashboards/"+copied.Dashboard.ID+"/versions", `{"spec":`+spec+`,"summary":"s","parent_version":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("saving the copy answered %d %s", rec.Code, rec.Body.String())
	}
}

func TestAStarterDashboardCanStillBeDeleted(t *testing.T) {
	r := newRig(t, false)
	b := boards{rig: r}
	id := r.starterDashboard(t).String()
	if rec := b.serve(adminUser, http.MethodDelete, "/dashboards/"+id, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete answered %d %s", rec.Code, rec.Body.String())
	}
}

func TestAStarterDashboardRefusesSavesAndProposalsFromTheService(t *testing.T) {
	r := newRig(t, false)
	id := r.starterDashboard(t)
	ctx := reading(t.Context())
	callers := []auth.Principal{adminUser, testUser, commenterNoGrant}
	for _, by := range callers {
		_, err := r.service.Save(ctx, by, SaveInput{DashboardID: id, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "s", ProposeIfNeeded: true})
		if !errors.Is(err, folders.ErrProvisioned) {
			t.Errorf("%s: save err = %v, want the provisioned refusal", by.Name, err)
		}
	}
	if _, err := r.service.UpdateProvisioned(ctx, adminUser, ReleaseUpdate{DashboardID: id, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "release", Home: r.generalFolder(t)}); err != nil {
		t.Fatalf("the release update: %v", err)
	}
	if n := r.latestVersion(t, id.String()); n != 2 {
		t.Fatalf("version = %d after the release update, want 2", n)
	}
}

func TestAStarterDashboardTakesNoAgentRunThatChangesIt(t *testing.T) {
	r := newRig(t, true)
	id := r.starterDashboard(t)
	ctx := reading(t.Context())
	for _, intent := range []revisions.Intent{revisions.IntentApply, revisions.IntentSuggest} {
		if err := r.revisions.CheckRun(ctx, revisions.RunTarget{Intent: intent, DashboardID: id}, adminUser); !errors.Is(err, folders.ErrProvisioned) {
			t.Errorf("%s: err = %v, want the provisioned refusal", intent, err)
		}
	}
	for name, ask := range map[string]revisions.RunTarget{
		"an ask":           {Intent: revisions.IntentAsk, DashboardID: id},
		"an investigation": {Intent: revisions.IntentAsk, DashboardID: id, Files: true},
	} {
		if err := r.revisions.CheckRun(ctx, ask, adminUser); err != nil {
			t.Errorf("%s: %v, want a run that only reads a starter to be allowed", name, err)
		}
	}
}

func (r *rig) dashboardsIn(t *testing.T, folder pgtype.UUID, title string) int {
	t.Helper()
	var n int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM dashboards WHERE folder_id = $1 AND title = $2`, folder.String(), title).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAFailedReleaseUpdateLeavesNoCopyAndDoesNotMoveTheStarter(t *testing.T) {
	r := newPlacementRig(t)
	ops := r.folder(t, "Ops", false)
	home := r.folder(t, "Audit", true)
	created, _, err := r.service.Create(reading(t.Context()), placementAdmin, ops, panelsSpec("Overview", "rps"), "by hand")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.markStarter(t, id)
	failure := errors.New("the update failed after the copy")
	for range 3 {
		_, err = r.service.UpdateProvisioned(reading(t.Context()), placementAdmin, ReleaseUpdate{
			DashboardID: id, Parent: 1, Spec: auditSpec("Overview"), Summary: "release", Home: home, EditedCopySuffix: " (your copy)",
			InTx: func(*dbsqlc.Queries, int) error { return failure },
		})
		if !errors.Is(err, failure) {
			t.Fatalf("err = %v, want the failure", err)
		}
	}
	if n := r.dashboardsIn(t, ops, "Overview (your copy)") + r.dashboardsIn(t, home, "Overview (your copy)"); n != 0 {
		t.Fatalf("%d copies after three failed updates, want 0", n)
	}
	if n := r.dashboardsIn(t, ops, "Overview"); n != 1 {
		t.Fatal("the starter left its folder although the update failed")
	}
}

func TestAReleaseUpdateMovesTheStarterBackAndKeepsTheEditedCopyWhereItWas(t *testing.T) {
	r := newPlacementRig(t)
	ops := r.folder(t, "Ops", false)
	home := r.folder(t, "Audit", true)
	created, _, err := r.service.Create(reading(t.Context()), placementAdmin, ops, panelsSpec("Overview", "rps"), "by hand")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.markStarter(t, id)

	if _, err := r.service.UpdateProvisioned(reading(t.Context()), placementAdmin, ReleaseUpdate{
		DashboardID: id, Parent: 1, Spec: auditSpec("Overview"), Summary: "release", Home: home, EditedCopySuffix: " (your copy)",
	}); err != nil {
		t.Fatal(err)
	}
	if n := r.dashboardsIn(t, home, "Overview"); n != 1 {
		t.Fatal("the starter is not back in the Audit folder")
	}
	if n := r.dashboardsIn(t, ops, "Overview (your copy)"); n != 1 {
		t.Fatal("the edited copy is not in the folder the starter was in")
	}
}

func TestAReleaseUpdateWhoseCopyCannotStayPutsTheCopyInTheHomeFolder(t *testing.T) {
	r := newPlacementRig(t)
	ops := r.folder(t, "Ops", false)
	home := r.folder(t, "Audit", true)
	created, _, err := r.service.Create(reading(t.Context()), placementAdmin, home, auditSpec("Overview"), "by hand")
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.ParseUUID(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.SQLDB(t).ExecContext(t.Context(), `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, id.String(), ops.String()); err != nil {
		t.Fatal(err)
	}
	r.markStarter(t, id)

	if _, err := r.service.UpdateProvisioned(reading(t.Context()), placementAdmin, ReleaseUpdate{
		DashboardID: id, Parent: 1, Spec: auditSpec("Overview"), Summary: "release", Home: home, EditedCopySuffix: " (your copy)",
	}); err != nil {
		t.Fatal(err)
	}
	if n := r.dashboardsIn(t, home, "Overview (your copy)"); n != 1 {
		t.Fatal("a copy that holds Admin-only content is not in the Audit folder")
	}
	if n := r.dashboardsIn(t, ops, "Overview (your copy)"); n != 0 {
		t.Fatal("a copy that holds Admin-only content was left in a folder others read")
	}
}

func TestTheDashboardListMarksAStarterAndShowsItNotEditable(t *testing.T) {
	r := newPlacementRig(t)
	b := boards{rig: r}
	starter := r.starterDashboard(t).String()
	ordinary := r.createDashboard(t).String()
	rec := b.serve(placementAdmin, http.MethodGet, "/dashboards", "")
	var list struct {
		Dashboards []struct {
			ID          string
			Provisioned bool
			Permissions struct{ Edit bool }
		}
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("list answered %d %s", rec.Code, rec.Body.String())
	}
	seen := 0
	for _, item := range list.Dashboards {
		switch item.ID {
		case starter:
			seen++
			if !item.Provisioned || item.Permissions.Edit {
				t.Errorf("starter item = %+v, want provisioned and not editable", item)
			}
		case ordinary:
			seen++
			if item.Provisioned || !item.Permissions.Edit {
				t.Errorf("ordinary item = %+v, want editable", item)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("the list holds %d of the 2 dashboards", seen)
	}
}

func (r *rig) keepAnEmptyPanelUnverified(t *testing.T, id pgtype.UUID) {
	t.Helper()
	r.db.Exec(t, `UPDATE dashboard_versions SET unverified = '{"panels":[{"panel_id":"p0","issues":[{"path":"/panels/0/queries","code":"query_empty","message":"returned no data"}]}],"variables":[]}'::jsonb WHERE dashboard_id = $1`, id.String())
}

func unverifiedPanelsOver(t *testing.T, b boards, path string) int {
	t.Helper()
	rec := b.serve(adminUser, http.MethodGet, path, "")
	var got struct {
		Version struct {
			Unverified struct{ Panels []struct{ PanelID string } }
		}
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("GET %s answered %d %s", path, rec.Code, rec.Body.String())
	}
	return len(got.Version.Unverified.Panels)
}

func TestAStarterDoesNotShowAnEmptyPanelAsUnverifiedButItsCopyDoes(t *testing.T) {
	r := newRig(t, false)
	b := boards{rig: r}
	id := r.starterDashboard(t)
	r.keepAnEmptyPanelUnverified(t, id)

	if n := unverifiedPanelsOver(t, b, "/dashboards/"+id.String()); n != 0 {
		t.Errorf("the starter shows %d unverified panels, want none", n)
	}
	if n := unverifiedPanelsOver(t, b, "/dashboards/"+id.String()+"/versions/1"); n != 0 {
		t.Errorf("version 1 of the starter shows %d unverified panels, want none", n)
	}
	rec := b.serve(adminUser, http.MethodPost, "/dashboards/"+id.String()+"/duplicate", `{"title":"Mine"}`)
	var copied struct{ Dashboard struct{ ID string } }
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &copied) != nil {
		t.Fatalf("duplicate answered %d %s", rec.Code, rec.Body.String())
	}
	if n := unverifiedPanelsOver(t, b, "/dashboards/"+copied.Dashboard.ID); n != 1 {
		t.Errorf("the copy shows %d unverified panels, want the 1 it inherited", n)
	}
}
