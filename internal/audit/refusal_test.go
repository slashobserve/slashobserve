package audit

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/forbidden"
)

func (s storeRig) dataOf(t *testing.T, request uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	row := s.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT data FROM audit_events WHERE request_id = $1`, request)
	if err := row.Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func (s storeRig) refuse(t *testing.T, status int, body []byte) map[string]any {
	t.Helper()
	engine := s.engine(t, func(ctx *gin.Context) { ctx.Data(status, "application/json", body) })
	_, request := serve(t, engine, http.MethodPost, "/api/v1/dashboards/"+dashboardID+"/versions")
	s.run(t)()
	return s.dataOf(t, request)
}

func TestARefusalRecordsOnlyTheRoleAndTheAccessItNeeded(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	cases := []struct {
		name string
		body forbidden.Body
		want map[string]any
	}{
		{"a role", forbidden.NeedsRole("editor"), map[string]any{"needed_role": "editor"}},
		{"an access level on a folder, without the folder title", forbidden.NeedsAccess("edit", "Payments"), map[string]any{"needed_access": "edit"}},
		{"neither", forbidden.Body{Error: forbidden.RoleMessage, Code: forbidden.Code}, map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, err := json.Marshal(c.body)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.refuse(t, http.StatusForbidden, body); !maps.Equal(got, c.want) {
				t.Fatalf("data = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOnlyADeniedEventCarriesTheNeededRole(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	body := []byte(`{"error":"x","code":"internal","needed_role":"admin"}`)
	if got := s.refuse(t, http.StatusForbidden, body); got["needed_role"] != "admin" {
		t.Fatalf("a denied request recorded %v, want needed_role admin", got)
	}
	if got := s.refuse(t, http.StatusInternalServerError, body); len(got) != 0 {
		t.Fatalf("a failed request recorded %v, want no data", got)
	}
}
