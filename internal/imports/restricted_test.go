package imports_test

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func restrictedRig(t *testing.T) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: promStub{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: promStub{}}); err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	dash := dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	return &rig{db: database, dashboard: dash, access: access, imports: imports.New(database.DB, dash, access, registry, logger)}
}

func TestAGrafanaImportOnAnAdminsOnlyDatasourceOutsideAnAdminsOnlyFolderAnswersRestrictedContent(t *testing.T) {
	r := restrictedRig(t)

	rec := r.serve(t, ada, http.MethodPost, "/imports", map[string]string{"json": string(grafanaJSON("Salaries", "audit", 1))})

	body := rec.Body.String()
	if rec.Code != http.StatusConflict || !strings.Contains(body, `"code":"restricted_content"`) ||
		!strings.Contains(body, `datasource \"audit\"`) || !strings.Contains(body, "folder only Admins can read") {
		t.Fatalf("POST /imports = %d %s, want 409 restricted_content naming the datasource", rec.Code, body)
	}
	var saved int
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM dashboards`).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if saved != 0 {
		t.Fatalf("dashboards saved = %d, want 0", saved)
	}
}
