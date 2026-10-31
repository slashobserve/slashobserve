package imports_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/imports"
)

const (
	privateGroup  = "payroll-team"
	hiddenTitle   = "Payroll import"
	openTitle     = "Open import"
	roleErrorCode = "forbidden_role"
)

var (
	editorNoGrant    = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Eve", Email: "eve@example.com", Role: auth.RoleEditor}
	commenterNoGrant = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Cam", Email: "cam@example.com", Role: auth.RoleCommenter}
	viewerNoGrant    = auth.Principal{Kind: auth.KindUser, ID: "44444444-4444-4444-8444-444444444444", Name: "Vic", Email: "vic@example.com", Role: auth.RoleViewer}
	groupViewer      = auth.Principal{Kind: auth.KindUser, ID: "66666666-6666-4666-8666-666666666666", Name: "Gia", Email: "gia@example.com", Role: auth.RoleViewer, Groups: []string{privateGroup}}
)

func (r *rig) serve(t *testing.T, by auth.Principal, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	imports.NewHandler(r.imports, r.access).Register(auth.NewRoutes(router.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})))
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func importBody(title string) map[string]string {
	return map[string]string{"json": string(grafanaJSON(title, "prom", 1))}
}

func TestImportsOfARestrictedFolderNeverLeak(t *testing.T) {
	r := newRig(t)
	open, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON(openTitle, "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON(hiddenTitle, "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	foldertest.Move(t, r.db, uuid(t, hidden.Dashboard.ID), foldertest.Restricted(t, r.db, "Payroll", privateGroup))

	for _, by := range []auth.Principal{editorNoGrant, commenterNoGrant, viewerNoGrant} {
		t.Run(by.Name, func(t *testing.T) {
			metas, _, err := r.imports.List(t.Context(), by, imports.ListParams{Limit: 50})
			if err != nil || len(metas) != 1 || metas[0].Title != openTitle {
				t.Fatalf("list = %+v, err %v, want only %q", metas, err, openTitle)
			}
			metas, _, err = r.imports.List(t.Context(), by, imports.ListParams{Limit: 50, DashboardID: uuid(t, hidden.Dashboard.ID)})
			if err != nil || len(metas) != 0 {
				t.Fatalf("list by the hidden dashboard = %+v, err %v, want nothing", metas, err)
			}

			rec := r.serve(t, by, http.MethodGet, "/imports", nil)
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), hiddenTitle) || strings.Contains(rec.Body.String(), hidden.Dashboard.ID) || !strings.Contains(rec.Body.String(), openTitle) {
				t.Errorf("GET /imports = %d %s", rec.Code, rec.Body)
			}
			rec = r.serve(t, by, http.MethodGet, "/dashboards/"+hidden.Dashboard.ID+"/import", nil)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), roleErrorCode) || strings.Contains(rec.Body.String(), hiddenTitle) {
				t.Errorf("GET the hidden dashboard's import = %d %s, want 403 %s", rec.Code, rec.Body, roleErrorCode)
			}
			rec = r.serve(t, by, http.MethodGet, "/dashboards/"+open.Dashboard.ID+"/import", nil)
			if rec.Code != http.StatusOK {
				t.Errorf("GET the open dashboard's import = %d %s", rec.Code, rec.Body)
			}
		})
	}

	for _, by := range []auth.Principal{groupViewer, ada} {
		t.Run(by.Name+" has access", func(t *testing.T) {
			metas, _, err := r.imports.List(t.Context(), by, imports.ListParams{Limit: 50})
			if err != nil || len(metas) != 2 {
				t.Fatalf("list = %+v, err %v, want both imports", metas, err)
			}
			rec := r.serve(t, by, http.MethodGet, "/dashboards/"+hidden.Dashboard.ID+"/import", nil)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), hiddenTitle) {
				t.Errorf("GET the hidden dashboard's import = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestViewersAndCommentersWithoutAGrantCannotImport(t *testing.T) {
	r := newRig(t)
	for _, by := range []auth.Principal{viewerNoGrant, commenterNoGrant} {
		t.Run(by.Name, func(t *testing.T) {
			for _, path := range []string{"/imports", "/imports/preview"} {
				rec := r.serve(t, by, http.MethodPost, path, importBody("Refused"))
				if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), roleErrorCode) {
					t.Errorf("POST %s = %d %s, want 403 %s", path, rec.Code, rec.Body, roleErrorCode)
				}
			}
			if metas, _, err := r.imports.List(t.Context(), ada, imports.ListParams{Limit: 50}); err != nil || len(metas) != 0 {
				t.Errorf("imports after the refusals = %+v, err %v", metas, err)
			}
		})
	}

	t.Run("an Editor imports", func(t *testing.T) {
		for path, want := range map[string]int{"/imports/preview": http.StatusOK, "/imports": http.StatusCreated} {
			if rec := r.serve(t, editorNoGrant, http.MethodPost, path, importBody("Allowed")); rec.Code != want {
				t.Errorf("POST %s = %d %s, want %d", path, rec.Code, rec.Body, want)
			}
		}
	})
}
