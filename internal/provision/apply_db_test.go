package provision_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/provision"
)

type applyRig struct {
	*rig
	applier *provision.Applier
	editor  auth.Principal
	viewer  auth.Principal
}

func newApplyRig(t *testing.T) *applyRig {
	t.Helper()
	r := newRig(t)
	person := func(name string, role auth.Role) auth.Principal {
		row, err := r.db.Queries().UpsertUser(t.Context(), dbsqlc.UpsertUserParams{OidcIssuer: "issuer", OidcSubject: name, Email: name + "@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		return auth.Principal{Kind: auth.KindUser, ID: row.ID.String(), Name: name, Email: name + "@example.com", Role: role}
	}
	return &applyRig{
		rig:     r,
		applier: provision.NewApplier(r.db.DB, r.dashboards, folders.NewAccess(r.db.DB)),
		editor:  person("eda", auth.RoleEditor),
		viewer:  person("vic", auth.RoleViewer),
	}
}

func (r *applyRig) apply(t *testing.T, by auth.Principal, path, content string) provision.Result {
	t.Helper()
	res, err := r.applier.Apply(t.Context(), by, path, json.RawMessage(content))
	if err != nil {
		t.Fatalf("Apply %s: %v", path, err)
	}
	return res
}

func TestApplyCreatesThenSkipsAnUnchangedFileThenAddsOneVersion(t *testing.T) {
	r := newApplyRig(t)
	created := r.apply(t, r.editor, "Shop/checkout.json", specJSON("Checkout", "up"))
	if created.Outcome != provision.OutcomeCreated || created.Version != 1 || created.Code != "" {
		t.Fatalf("first apply = %+v", created)
	}
	dash, _ := r.latest(t, created.DashboardID)
	if dash.Folder.ID != r.folderID(t, "Shop") {
		t.Errorf("dashboard went to folder %q", dash.Folder.Title)
	}

	pretty, err := json.MarshalIndent(json.RawMessage(specJSON("Checkout", "up")), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if again := r.apply(t, r.editor, "Shop/checkout.json", string(pretty)); again.Outcome != provision.OutcomeUnchanged || again.Version != 1 || again.DashboardID != created.DashboardID {
		t.Errorf("reformatted apply = %+v, want unchanged", again)
	}

	updated := r.apply(t, r.editor, "Shop/checkout.json", specJSON("Checkout", "rate(up[5m])"))
	if updated.Outcome != provision.OutcomeUpdated || updated.Version != 2 || updated.DashboardID != created.DashboardID {
		t.Fatalf("changed apply = %+v", updated)
	}
	metas := r.versions(t, created.DashboardID)
	if len(metas) != 2 || metas[0].Summary != "Applied from Shop/checkout.json" || metas[0].Author.Name != "eda" {
		t.Errorf("versions = %+v", metas)
	}
}

func TestApplyAdoptsTheOneDashboardWithTheTitleAndSkipsIdenticalContent(t *testing.T) {
	r := newApplyRig(t)
	dash, _, err := r.dashboards.Create(t.Context(), r.editor, r.folderID(t, "General"), []byte(specJSON("Payments", "up")), "by hand")
	if err != nil {
		t.Fatal(err)
	}
	_, ver := r.latest(t, dash.ID)
	exported, err := json.MarshalIndent(ver.Spec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	same := r.apply(t, r.editor, "payments.json", string(exported))
	if same.Outcome != provision.OutcomeUnchanged || same.DashboardID != dash.ID || same.Version != 1 {
		t.Fatalf("exported content = %+v, want unchanged on the adopted dashboard", same)
	}
	changed := r.apply(t, r.editor, "renamed.json", specJSON("PAYMENTS", "rate(up[1m])"))
	if changed.Outcome != provision.OutcomeUpdated || changed.DashboardID != dash.ID || changed.Version != 2 {
		t.Fatalf("case-insensitive adoption = %+v", changed)
	}
	bindings, err := r.applier.Applied(t.Context(), r.viewer, r.folderID(t, "General"))
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Path != "renamed.json" || bindings[0].DashboardID != dash.ID {
		t.Errorf("bindings = %+v, want the binding moved to renamed.json", bindings)
	}
}

func TestApplyRefusesAmbiguousAndProvisionedDashboards(t *testing.T) {
	r := newApplyRig(t)
	general := r.folderID(t, "General")
	for _, title := range []string{"Twin", "twin"} {
		if _, _, err := r.dashboards.Create(t.Context(), r.editor, general, []byte(specJSON(title, "up")), "by hand"); err != nil {
			t.Fatal(err)
		}
	}
	if res := r.apply(t, r.editor, "twin.json", specJSON("TWIN", "up")); res.Outcome != provision.OutcomeFailed || res.Code != provision.CodeAmbiguous {
		t.Errorf("ambiguous apply = %+v", res)
	}

	r.write(t, "prov.json", specJSON("Provisioned", "up"))
	r.run(t)
	if res := r.apply(t, r.editor, "prov.json", specJSON("Provisioned", "rate(up[1m])")); res.Outcome != provision.OutcomeFailed || res.Code != provision.CodeProvisioned {
		t.Errorf("provisioned apply = %+v", res)
	}
}

func TestApplyEscapedFolderInvalidSpecAndRole(t *testing.T) {
	r := newApplyRig(t)
	res := r.apply(t, r.editor, "Team %2F Payments/api.json", specJSON("API", "up"))
	if res.Outcome != provision.OutcomeCreated {
		t.Fatalf("escaped folder apply = %+v", res)
	}
	if dash, _ := r.latest(t, res.DashboardID); dash.Folder.Title != "Team / Payments" {
		t.Errorf("folder = %q", dash.Folder.Title)
	}

	invalid := r.apply(t, r.editor, "broken.json", specJSON("Broken", brokenQuery))
	if invalid.Outcome != provision.OutcomeFailed || invalid.Code != provision.CodeInvalidSpec || len(invalid.Issues) == 0 {
		t.Errorf("failing query apply = %+v", invalid)
	}

	if denied := r.apply(t, r.viewer, "viewer.json", specJSON("Viewer", "up")); denied.Outcome != provision.OutcomeFailed || denied.Code != "forbidden_role" {
		t.Errorf("viewer apply = %+v", denied)
	}
	if update := r.apply(t, r.viewer, "Team %2F Payments/api.json", specJSON("API", "rate(up[1m])")); update.Code != "forbidden_role" {
		t.Errorf("viewer update = %+v", update)
	}

	if _, err := r.applier.Apply(t.Context(), r.editor, "../escape.json", json.RawMessage(specJSON("x", "up"))); err == nil {
		t.Error("a path outside the directory was accepted")
	}
}

func TestApplyHTTPResponsesMatchTheAPIDocument(t *testing.T) {
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	r := newApplyRig(t)
	general := r.folderID(t, "General").String()
	for _, title := range []string{"Twin", "twin"} {
		if _, _, err := r.dashboards.Create(t.Context(), r.editor, r.folderID(t, "General"), []byte(specJSON(title, "up")), "by hand"); err != nil {
			t.Fatal(err)
		}
	}
	body := func(path, spec string) string {
		raw, err := json.Marshal(map[string]any{"path": path, "spec": json.RawMessage(spec)})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	cases := []struct {
		name         string
		by           auth.Principal
		method, path string
		body         string
		status       int
	}{
		{"apply", r.editor, http.MethodPost, "/provision/apply", body("Shop/a.json", specJSON("A", "up")), http.StatusOK},
		{"apply again", r.editor, http.MethodPost, "/provision/apply", body("Shop/a.json", specJSON("A", "up")), http.StatusOK},
		{"apply a bad path", r.editor, http.MethodPost, "/provision/apply", body("a/b/c.json", specJSON("A", "up")), http.StatusBadRequest},
		{"apply without a spec", r.editor, http.MethodPost, "/provision/apply", `{"path":"a.json"}`, http.StatusBadRequest},
		{"apply an ambiguous title", r.editor, http.MethodPost, "/provision/apply", body("twin.json", specJSON("twin", "up")), http.StatusConflict},
		{"apply a failing spec", r.editor, http.MethodPost, "/provision/apply", body("broken.json", specJSON("Broken", brokenQuery)), http.StatusUnprocessableEntity},
		{"apply as a viewer", r.viewer, http.MethodPost, "/provision/apply", body("v.json", specJSON("V", "up")), http.StatusForbidden},
		{"applied", r.viewer, http.MethodGet, "/provision/applied?folder_id=" + general, "", http.StatusOK},
		{"applied without a folder", r.viewer, http.MethodGet, "/provision/applied", "", http.StatusBadRequest},
		{"applied in an unknown folder", r.viewer, http.MethodGet, "/provision/applied?folder_id=0b9e9a3e-5d5e-4f0e-9e1a-7d2f3c4b5a61", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			group := engine.Group("/api/v1", func(c *gin.Context) {
				c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), tc.by))
			})
			provision.NewApplyHandler(r.applier, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(group))
			req := httptest.NewRequest(tc.method, "/api/v1"+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			check := httptest.NewRequest(tc.method, "/api/v1"+tc.path, nil)
			route, params, err := router.FindRoute(check)
			if err != nil {
				t.Fatalf("no documented operation: %v", err)
			}
			options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true}
			input := &openapi3filter.ResponseValidationInput{
				RequestValidationInput: &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options},
				Status:                 rec.Code,
				Header:                 rec.Header(),
				Options:                options,
			}
			input.SetBodyBytes(rec.Body.Bytes())
			if err := openapi3filter.ValidateResponse(t.Context(), input); err != nil {
				t.Fatalf("status %d body %s breaks the document: %v", rec.Code, rec.Body.String(), err)
			}
		})
	}
}
