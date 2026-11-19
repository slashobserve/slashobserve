package dashboards

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func wantPatchError(t *testing.T, err error) *spec.PatchError {
	t.Helper()
	var patchErr *spec.PatchError
	if !errors.As(err, &patchErr) {
		t.Fatalf("err = %v, want PatchError", err)
	}
	return patchErr
}

func TestSavePatchRemovesAPanelWithoutRunningAQuery(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(3)})
	p.reset()

	res, err := r.service.SavePatch(t.Context(), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: spec.PanelPatch{Delete: []string{"p1"}}, Summary: "Remove p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.texts(); len(got) != 0 {
		t.Fatalf("queries run %v, want none", got)
	}
	if res.Version.Number != 2 {
		t.Fatalf("version %d, want 2", res.Version.Number)
	}
	if _, found := res.Version.Spec.FindPanel("p1"); found {
		t.Fatal("p1 is still in version 2")
	}
	if len(res.Version.Spec.AllPanels()) != 2 {
		t.Fatalf("version 2 has %d panels, want 2", len(res.Version.Spec.AllPanels()))
	}
}

func TestSavePatchDuplicatesAPanelRunningOnlyTheCopy(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(3)})
	p.reset()

	copied := panelDef{id: "p0_copy", query: "copy0"}
	res, err := r.service.SavePatch(t.Context(), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: upsert(3, copied), Summary: "Duplicate p0"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.texts(); len(got) != 1 || got[0] != "copy0" {
		t.Fatalf("queries run %v, want only copy0", got)
	}
	if _, found := res.Version.Spec.FindPanel("p0_copy"); !found || len(res.Version.Spec.AllPanels()) != 4 {
		t.Fatalf("version %d has %d panels, want the copy beside the 3 originals", res.Version.Number, len(res.Version.Spec.AllPanels()))
	}
}

func TestSavePatchReplaysARetriedPatch(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})
	in := PatchInput{DashboardID: id, Parent: 1, Patch: spec.PanelPatch{Delete: []string{"p0"}}, Summary: "Remove p0"}

	first, err := r.service.SavePatch(t.Context(), testUser, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := r.service.SavePatch(t.Context(), testUser, in)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replayed || again.Version.Number != first.Version.Number {
		t.Fatalf("retry saved version %d (replayed %v), want a replay of %d", again.Version.Number, again.Replayed, first.Version.Number)
	}
}

func TestSavePatchNamesTheFailingPanel(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(2)})
	p.setFail("broken", true)

	_, err := r.service.SavePatch(t.Context(), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: upsert(2, panelDef{id: "p2", query: "broken"}), Summary: "Add p2"})
	patchErr := wantPatchError(t, err)
	if len(patchErr.Issues) == 0 || !strings.HasPrefix(patchErr.Issues[0].Path, "/panels/2") || patchErr.PanelIDs["/panels/2"] != "p2" {
		t.Fatalf("issues %+v ids %v, want an issue on /panels/2 owned by p2", patchErr.Issues, patchErr.PanelIDs)
	}
}

func TestSavePatchRejectsWhatTheCheckRefusesBeforeSaving(t *testing.T) {
	r, p := newProbeRig(t)
	id := r.createSpec(t, specDef{panels: numberedPanels(1)})
	p.reset()
	refuse := func(next, _ *spec.Dashboard) []spec.Issue {
		return []spec.Issue{{Path: "/panels/0", Code: spec.CodeSchema, Message: "refused"}}
	}

	_, err := r.service.SavePatch(t.Context(), testUser, PatchInput{DashboardID: id, Parent: 1, Patch: upsert(0, panelDef{id: "p0", query: "m0b"}), Summary: "Edit", Check: refuse})
	if patchErr := wantPatchError(t, err); patchErr.Issues[0].Message != "refused" || patchErr.PanelIDs["/panels/0"] != "p0" {
		t.Fatalf("patch error %+v, want the check's issue on p0", patchErr)
	}
	if got := p.texts(); len(got) != 0 {
		t.Fatalf("queries run %v, want none", got)
	}
	if _, latest, err := r.service.Get(t.Context(), id); err != nil || latest.Number != 1 {
		t.Fatalf("latest version %d (err %v), want 1", latest.Number, err)
	}
}

func TestPatchSizeErrorNamesTheFieldOverItsLimit(t *testing.T) {
	if err := PatchSizeError(spec.PanelPatch{Delete: make([]string, MaxDeletePanels)}); err != nil {
		t.Fatalf("a patch at the limit answered %v", err)
	}
	err := PatchSizeError(spec.PanelPatch{Upsert: make([]json.RawMessage, MaxUpsertPanels+1)})
	if err == nil || err.Message != "upsert holds 51 items; send at most 50 per call" {
		t.Fatalf("err = %v", err)
	}
}

func TestPanelsRouteSavesReplaysAndRefuses(t *testing.T) {
	b := seedBoards(t)
	path := "/dashboards/" + b.open.String() + "/panels"
	copied := `{"upsert":[{"id":"p-copy","type":"timeseries","title":"p copy","gridPos":{"x":0,"y":8,"w":12,"h":8},"datasource":"prom","queries":[{"query":"up"}]}],"summary":"Duplicate p","parent_version":1}`

	saved := b.serve(testUser, http.MethodPost, path, copied)
	if saved.Code != http.StatusCreated || !strings.HasSuffix(saved.Header().Get("Location"), "/versions/2") {
		t.Fatalf("save answered %d at %q: %s", saved.Code, saved.Header().Get("Location"), saved.Body)
	}
	replay := b.serve(testUser, http.MethodPost, path, copied)
	if replay.Code != http.StatusOK || replay.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("retry answered %d: %s", replay.Code, replay.Body)
	}

	for _, tc := range []struct {
		name   string
		by     auth.Principal
		body   string
		status int
		code   string
	}{
		{"a viewer", viewerNoGrant, `{"delete":["p-copy"],"summary":"x","parent_version":2}`, http.StatusForbidden, roleErrorCode},
		{"an unknown panel", testUser, `{"delete":["nope"],"summary":"x","parent_version":2}`, http.StatusUnprocessableEntity, "invalid_spec"},
		{"an unknown field", testUser, `{"remove":["p"],"summary":"x","parent_version":2}`, http.StatusBadRequest, "invalid_request"},
		{"no parent", testUser, `{"delete":["p-copy"],"summary":"x"}`, http.StatusBadRequest, "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := b.serve(tc.by, http.MethodPost, path, tc.body)
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != tc.status || body.Code != tc.code {
				t.Fatalf("answered %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
}
