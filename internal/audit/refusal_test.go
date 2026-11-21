package audit

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/forbidden"
)

func (s storeRig) dataOf(t *testing.T, request uuid.UUID, key string) string {
	t.Helper()
	var value string
	row := s.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT coalesce(data->>$2, '-') FROM audit_events WHERE request_id = $1`, request, key)
	if err := row.Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestARefusalRecordsTheRoleAndTheAccessItNeeded(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	cases := []struct {
		name, body, key, want string
	}{
		{"role", `{"error":"your role does not allow this","code":"forbidden_role","needed_role":"editor"}`, "needed_role", "editor"},
		{"access", `{"error":"your role does not allow this","code":"forbidden_role","needed_access":"edit","folder_title":"Payments"}`, "needed_access", "edit"},
		{"folder title is not kept", `{"error":"your role does not allow this","code":"forbidden_role","needed_access":"edit","folder_title":"Payments"}`, "folder_title", "-"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			engine := s.engine(t, func(ctx *gin.Context) { ctx.Data(http.StatusForbidden, "application/json", []byte(c.body)) })
			_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
			s.run(t)()
			if got := s.dataOf(t, request, c.key); got != c.want {
				t.Fatalf("%s = %q, want %q", c.key, got, c.want)
			}
		})
	}
}

func TestOnlyADeniedEventCarriesTheNeededRole(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	engine := s.engine(t, func(ctx *gin.Context) {
		ctx.Data(http.StatusInternalServerError, "application/json", []byte(`{"error":"boom","code":"internal","needed_role":"admin"}`))
	})
	_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	s.run(t)()
	if got := s.dataOf(t, request, "needed_role"); got != "-" {
		t.Fatalf("a failed request recorded needed_role %q", got)
	}
}

func TestAViewerRefusedAnEditorRouteAndAnEditorRefusedAnAdminRouteRecordTheRoleTheyLacked(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	for _, role := range []string{"editor", "admin"} {
		t.Run(role, func(t *testing.T) {
			body, err := json.Marshal(forbidden.NeedsRole(role))
			if err != nil {
				t.Fatal(err)
			}
			engine := s.engine(t, func(ctx *gin.Context) { ctx.Data(http.StatusForbidden, "application/json", body) })
			_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
			s.run(t)()
			if got := s.dataOf(t, request, "needed_role"); got != role {
				t.Fatalf("needed_role = %q, want %q", got, role)
			}
		})
	}
}
