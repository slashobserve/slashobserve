package datasourceadmin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (h *httpRig) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := h.database.SQLDB(t).QueryRowContext(t.Context(), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMakingADatasourceAdminsOnlyListsTheContentOthersCanStillRead(t *testing.T) {
	h := newHTTPRig(t)
	if created := h.call(t, http.MethodPost, "/api/v1/datasources", createBody("payroll", upstream(t, new(string)).URL)); created.Code != http.StatusCreated {
		t.Fatalf("create answered %d: %s", created.Code, created.Body)
	}
	dash := h.scalar(t, `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ('Salaries', 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`)
	h.database.Exec(t, `INSERT INTO dashboard_versions (dashboard_id, version, spec, summary, author_kind, author_id, author_name)
VALUES ($1::uuid, 1, '{"title":"Salaries","panels":[{"id":"p","type":"stat","datasource":"payroll","queries":[{"query":"up"}]}]}', 'seed', 'user', gen_random_uuid(), 'seed')`, dash)
	search := func(title, folder string) string {
		return h.scalar(t, `INSERT INTO saved_searches (kind, title, datasource_uid, query, time_from, time_to, folder_id, created_by_kind, created_by_id, created_by_name)
VALUES ('metrics', $1, 'payroll', 'up', 'now-1h', 'now', $2::uuid, 'user', gen_random_uuid(), 'Ada') RETURNING id::text`, title, folder)
	}
	personal := search("Mine", h.scalar(t, `SELECT id::text FROM folders WHERE personal AND owner_id = $1::uuid`, h.admin.ID))
	locked := foldertest.AdminsOnly(t, h.database, "Payroll")
	search("Locked", locked.String())

	refused := h.call(t, http.MethodPatch, "/api/v1/datasources/payroll", `{"audience":"admins"}`)
	var body struct {
		Code       string `json:"code"`
		Error      string `json:"error"`
		Datasource string `json:"datasource"`
		Hidden     int    `json:"hidden"`
		Items      []struct {
			Kind        string `json:"kind"`
			ID          string `json:"id"`
			FolderTitle string `json:"folder_title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(refused.Body.Bytes(), &body); err != nil || refused.Code != http.StatusConflict || body.Code != "restricted_content" ||
		body.Datasource != "payroll" || body.Hidden != 1 || len(body.Items) != 1 || body.Items[0].Kind != "dashboard" ||
		body.Items[0].ID != dash || body.Items[0].FolderTitle != "General" || !strings.Contains(body.Error, `"payroll"`) {
		t.Fatalf("refusal = %d %s", refused.Code, refused.Body)
	}
	if got := h.scalar(t, `SELECT audience FROM datasources WHERE uid = 'payroll'`); got != "everyone" {
		t.Fatalf("audience after the refusal = %q", got)
	}

	h.database.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1::uuid`, dash, locked)
	h.database.Exec(t, `DELETE FROM saved_searches WHERE id = $1::uuid`, personal)
	if done := h.call(t, http.MethodPatch, "/api/v1/datasources/payroll", `{"audience":"admins"}`); done.Code != http.StatusOK {
		t.Fatalf("after moving = %d %s", done.Code, done.Body)
	}
}

func TestTheInUseRefusalCountsItsUsersInTheRightNumber(t *testing.T) {
	one := (&InUseError{UID: "audit", Hidden: 1}).Error()
	many := (&InUseError{UID: "audit", Users: []User{{}}, Hidden: 2}).Error()
	if one != `datasource "audit" is used by 1 dashboard or saved search` || many != `datasource "audit" is used by 3 dashboards and saved searches` {
		t.Errorf("%q, %q", one, many)
	}
}
