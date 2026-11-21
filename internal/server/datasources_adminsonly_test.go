package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestAnAdminsOnlyDatasourceIsListedAndRecheckedOnlyForAnAdmin(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleCommenter, auth.RoleEditor, auth.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: unhealthyDatasource{}})
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: unhealthyDatasource{}}); err != nil {
				t.Fatal(err)
			}
			engine := datasourceEngine(t, role, registry)
			admin := role == auth.RoleAdmin

			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/datasources", nil))
			var listed struct {
				Datasources []struct {
					UID string `json:"uid"`
				} `json:"datasources"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
				t.Fatal(err)
			}
			var uids []string
			for _, d := range listed.Datasources {
				uids = append(uids, d.UID)
			}
			if slices.Contains(uids, "audit") != admin || !slices.Contains(uids, "loki") {
				t.Fatalf("listed %v", uids)
			}

			recheck := func(uid string) *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/datasources/"+uid+"/recheck", nil))
				return rec
			}
			got, missing := recheck("audit"), recheck("nope")
			if admin && got.Code != http.StatusOK {
				t.Fatalf("admin recheck = %d %s", got.Code, got.Body)
			}
			if !admin && (got.Code != missing.Code || got.Body.String() != missing.Body.String()) {
				t.Fatalf("recheck = %d %s, missing = %d %s", got.Code, got.Body, missing.Code, missing.Body)
			}
		})
	}
}

func TestOnlyAnAdminSeesTheAudienceOfEachDatasourceInTheListAndTheRecheck(t *testing.T) {
	registry, err := datasource.NewRegistry(nil, datasource.Instance{UID: "loki", Datasource: unhealthyDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: unhealthyDatasource{}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Put(datasource.Instance{UID: "payroll", Audience: datasource.AudienceAdmins, Datasource: unhealthyDatasource{}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Put(datasource.Instance{UID: "metrics", Datasource: unhealthyDatasource{}}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"loki": "everyone", "audit": "admins", "payroll": "admins", "metrics": "everyone"}

	for _, role := range []auth.Role{auth.RoleViewer, auth.RoleAdmin} {
		engine := datasourceEngine(t, role, registry)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/datasources", nil))
		var listed struct {
			Datasources []map[string]any `json:"datasources"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
			t.Fatal(err)
		}
		for _, d := range listed.Datasources {
			audience, present := d["audience"]
			if role != auth.RoleAdmin {
				if present {
					t.Errorf("%s sees the audience of %v", role, d["uid"])
				}
				continue
			}
			if audience != want[d["uid"].(string)] {
				t.Errorf("list audience of %v = %v", d["uid"], audience)
			}
		}
	}

	engine := datasourceEngine(t, auth.RoleAdmin, registry)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/datasources/payroll/recheck", nil))
	var rechecked map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rechecked); err != nil || rechecked["audience"] != "admins" {
		t.Fatalf("recheck = %d %s", rec.Code, rec.Body)
	}
}
