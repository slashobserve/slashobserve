package folders

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

const deleteRefusal = "move or delete the dashboards, findings, saved searches and observations in this folder first"

func newFolder(t *testing.T, d *dbtest.Database, title string) string {
	t.Helper()
	var id string
	if err := d.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title) VALUES ($1) RETURNING id::text`, title).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func deleteFolder(t *testing.T, d *dbtest.Database, folderID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	admin := auth.Principal{Kind: auth.KindServiceAccount, ID: "00000000-0000-4000-8000-0000000000aa", Name: "root", Role: auth.RoleAdmin}
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), admin))
	})
	NewHandler(d.DB, NewAccess(d.DB), slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/folders/"+folderID, nil))
	return rec
}

func TestDeletingAFolderThatHoldsAFindingOrASavedSearchIsRefused(t *testing.T) {
	d := dbtest.New(t)
	cases := []struct {
		name, insert string
	}{
		{"finding", `WITH o AS (INSERT INTO observations (folder_id, title, created_by_kind, created_by_id, created_by_name)
  VALUES ($1::uuid, 't', 'user', gen_random_uuid(), 'Ada') RETURNING id)
INSERT INTO findings (id, observation_id, title, markdown, time_from, time_to, created_by_kind, created_by_id, created_by_name)
SELECT o.id, o.id, 't', 'm', now() - interval '1 hour', now(), 'user', gen_random_uuid(), 'Ada' FROM o`},
		{"saved search", `INSERT INTO saved_searches (folder_id, kind, title, datasource_uid, query, time_from, time_to, created_by_kind, created_by_id, created_by_name)
VALUES ($1::uuid, 'logs', 't', 'loki', '{app="x"}', 'now-1h', 'now', 'user', gen_random_uuid(), 'Ada')`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := newFolder(t, d, "Holds a "+tc.name)
			d.Exec(t, tc.insert, id)

			rec := deleteFolder(t, d, id)
			var body struct{ Code, Error string }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", rec.Body, err)
			}
			if rec.Code != http.StatusConflict || body.Code != "folder_not_empty" || body.Error != deleteRefusal {
				t.Fatalf("delete = %d %s, want 409 folder_not_empty %q", rec.Code, rec.Body, deleteRefusal)
			}
		})
	}
}

func TestDeletingAnEmptyFolderStillWorks(t *testing.T) {
	d := dbtest.New(t)
	if rec := deleteFolder(t, d, newFolder(t, d, "Empty")); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s, want 204", rec.Code, rec.Body)
	}
}
