package imports_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/imports"
)

type existingBody struct {
	Existing *struct {
		DashboardID string `json:"dashboard_id"`
		Title       string `json:"title"`
		Version     int    `json:"version"`
		CanUpdate   bool   `json:"can_update"`
	} `json:"existing"`
}

func previewExisting(t *testing.T, r *rig, by auth.Principal, title string) existingBody {
	t.Helper()
	rec := r.serve(t, by, http.MethodPost, "/imports/preview", importBody(title))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status %d: %s", rec.Code, rec.Body.String())
	}
	var body existingBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestThePreviewNamesTheDashboardThatAlreadyHoldsTheGrafanaUID(t *testing.T) {
	r := newRig(t)
	if got := previewExisting(t, r, ada, "Shop"); got.Existing != nil {
		t.Fatalf("existing = %+v before any import, want null", got.Existing)
	}
	first, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	got := previewExisting(t, r, ada, "Shop")
	if got.Existing == nil || got.Existing.DashboardID != first.Dashboard.ID || got.Existing.Title != "Shop" || got.Existing.Version != 1 || !got.Existing.CanUpdate {
		t.Fatalf("existing = %+v, want the first import at version 1 that ada can update", got.Existing)
	}
}

func TestThePreviewHidesADashboardTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, r.db, uuid(t, first.Dashboard.ID), foldertest.Restricted(t, r.db, "Payroll", privateGroup))
	if got := previewExisting(t, r, editorNoGrant, "Shop"); got.Existing != nil {
		t.Fatalf("existing = %+v, want null for a caller without access", got.Existing)
	}
}

func TestAnImportCanUpdateTheDashboardItAlreadyMade(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]string{"json": string(grafanaJSON("Shop v2", "prom", 2)), "update_dashboard": first.Dashboard.ID}
	rec := r.serve(t, ada, http.MethodPost, "/imports", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var saved struct {
		Dashboard struct{ ID, Title string } `json:"dashboard"`
		Version   struct{ Version int }      `json:"version"`
		Import    struct{ ID string }        `json:"import"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Dashboard.ID != first.Dashboard.ID || saved.Version.Version != 2 || saved.Import.ID != first.Record.ID {
		t.Fatalf("saved %+v, want version 2 of the first dashboard and its import record", saved)
	}
	if latest, err := r.dashboard.LatestVersion(reading(t.Context()), uuid(t, first.Dashboard.ID)); err != nil || latest != 2 {
		t.Fatalf("latest = %d, err %v", latest, err)
	}
}

func TestAnUpdateNeedsAnImportTheCallerMayEdit(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, r.db, uuid(t, first.Dashboard.ID), foldertest.Restricted(t, r.db, "Payroll", privateGroup))
	body := map[string]string{"json": string(grafanaJSON("Shop v2", "prom", 1)), "update_dashboard": first.Dashboard.ID}
	if rec := r.serve(t, editorNoGrant, http.MethodPost, "/imports", body); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d for a caller without access, want 404: %s", rec.Code, rec.Body.String())
	}
	body["update_dashboard"] = "not-a-uuid"
	if rec := r.serve(t, ada, http.MethodPost, "/imports", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d for a bad id, want 400: %s", rec.Code, rec.Body.String())
	}
	if latest, err := r.dashboard.LatestVersion(reading(t.Context()), uuid(t, first.Dashboard.ID)); err != nil || latest != 1 {
		t.Fatalf("latest = %d, err %v, want the refused updates to save nothing", latest, err)
	}
}

func TestRefuseExistingAnswersWithTheDashboardInsteadOfMakingACopy(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"json": string(grafanaJSON("Shop", "prom", 1)), "refuse_existing": true}
	rec := r.serve(t, ada, http.MethodPost, "/imports", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var refused struct {
		Code     string `json:"code"`
		Existing struct {
			DashboardID string `json:"dashboard_id"`
			Version     int    `json:"version"`
			CanUpdate   bool   `json:"can_update"`
		} `json:"existing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &refused); err != nil {
		t.Fatal(err)
	}
	if refused.Code != "already_imported" || refused.Existing.DashboardID != first.Dashboard.ID || refused.Existing.Version != 1 || !refused.Existing.CanUpdate {
		t.Fatalf("refused = %+v, want already_imported naming the first dashboard", refused)
	}
	list := r.serve(t, ada, http.MethodGet, "/imports", nil)
	var listed struct {
		Imports []json.RawMessage `json:"imports"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || len(listed.Imports) != 1 {
		t.Fatalf("imports after the refusal: %s (err %v), want only the first", list.Body.String(), err)
	}
}

func TestRefuseExistingStillImportsANewFileAndAllowsAnUpdate(t *testing.T) {
	r := newRig(t)
	if rec := r.serve(t, ada, http.MethodPost, "/imports", map[string]any{"json": string(grafanaJSON("Shop", "prom", 1)), "refuse_existing": true}); rec.Code != http.StatusCreated {
		t.Fatalf("first import status %d: %s", rec.Code, rec.Body.String())
	}
	first, ok, err := r.imports.DashboardByGrafanaUID(reading(t.Context()), "abc")
	if err != nil || !ok {
		t.Fatalf("holder = %+v ok %v err %v", first, ok, err)
	}
	body := map[string]any{"json": string(grafanaJSON("Shop", "prom", 2)), "refuse_existing": true, "update_dashboard": first.DashboardID.String()}
	if rec := r.serve(t, ada, http.MethodPost, "/imports", body); rec.Code != http.StatusOK {
		t.Fatalf("update status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefuseExistingDoesNotRevealADashboardTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, r.db, uuid(t, first.Dashboard.ID), foldertest.Restricted(t, r.db, "Payroll", privateGroup))
	rec := r.serve(t, editorNoGrant, http.MethodPost, "/imports", map[string]any{"json": string(grafanaJSON("Shop", "prom", 1)), "refuse_existing": true})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAnUpdateNeedsTheFileTheDashboardWasImportedFrom(t *testing.T) {
	r := newRig(t)
	first, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	other := []byte(strings.Replace(string(grafanaJSON("Other", "prom", 2)), `"uid":"abc"`, `"uid":"zzz"`, 1))
	rec := r.serve(t, ada, http.MethodPost, "/imports", map[string]string{"json": string(other), "update_dashboard": first.Dashboard.ID})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"grafana_uid_mismatch"`) || !strings.Contains(rec.Body.String(), `abc`) || !strings.Contains(rec.Body.String(), `zzz`) {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if latest, err := r.dashboard.LatestVersion(reading(t.Context()), uuid(t, first.Dashboard.ID)); err != nil || latest != 1 {
		t.Fatalf("latest = %d, err %v, want the dashboard untouched", latest, err)
	}
}

func TestAnUpdateOfACopyMadeAsNewChecksTheFilesUIDToo(t *testing.T) {
	r := newRig(t)
	if _, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)}); err != nil {
		t.Fatal(err)
	}
	copied, err := r.imports.Import(reading(t.Context()), ada, imports.Request{JSON: grafanaJSON("Shop copy", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	other := []byte(strings.Replace(string(grafanaJSON("Other", "prom", 2)), `"uid":"abc"`, `"uid":"zzz"`, 1))
	if rec := r.serve(t, ada, http.MethodPost, "/imports", map[string]string{"json": string(other), "update_dashboard": copied.Dashboard.ID}); rec.Code != http.StatusConflict {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec := r.serve(t, ada, http.MethodPost, "/imports", map[string]string{"json": string(grafanaJSON("Shop copy v2", "prom", 2)), "update_dashboard": copied.Dashboard.ID}); rec.Code != http.StatusOK {
		t.Fatalf("same-uid update status %d: %s", rec.Code, rec.Body.String())
	}
}
