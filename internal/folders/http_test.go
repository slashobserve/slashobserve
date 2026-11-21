package folders

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
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

func adminEngine(t *testing.T, d *dbtest.Database) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return adminEngineWith(t, d, NewHandler(d.DB, NewAccess(d.DB), slog.New(slog.DiscardHandler)))
}

var testAdmin = auth.Principal{Kind: auth.KindServiceAccount, ID: "00000000-0000-4000-8000-0000000000aa", Name: "root", Role: auth.RoleAdmin}

func adminEngineWith(t *testing.T, d *dbtest.Database, h *Handler) *gin.Engine {
	t.Helper()
	recorded, err := audit.New(audit.Config{DB: d.DB}).Middleware()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	api := engine.Group("/api/v1", recorded, func(c *gin.Context) {
		ctx := auth.WithPrincipal(c.Request.Context(), testAdmin)
		audit.Update(ctx, func(s *audit.Scope) { s.Actor = auth.AuditActor(testAdmin) })
		c.Request = c.Request.WithContext(ctx)
	})
	h.Register(auth.NewRoutes(api))
	return engine
}

func deleteFolder(t *testing.T, d *dbtest.Database, folderID string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	adminEngine(t, d).ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/folders/"+folderID, nil))
	return rec
}

func TestListingFoldersCanBeRevalidatedButNeverSharedBetweenPeople(t *testing.T) {
	d := dbtest.New(t)
	newFolder(t, d, "Revalidated")
	engine := adminEngine(t, d)
	get := func(ifNoneMatch string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/folders", nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	first := get("")
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") != "private, no-cache" || first.Header().Get("Vary") != "Authorization, Cookie" {
		t.Fatalf("first = %d %v", first.Code, first.Header())
	}
	tag := first.Header().Get("ETag")
	if again := get(tag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("revalidating = %d %q, want an empty 304", again.Code, again.Body)
	}

	newFolder(t, d, "Added after the first read")
	if changed := get(tag); changed.Code != http.StatusOK || changed.Header().Get("ETag") == tag {
		t.Fatalf("after a change = %d with tag %q, want 200 and a new tag", changed.Code, changed.Header().Get("ETag"))
	}
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
