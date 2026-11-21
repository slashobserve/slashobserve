package mcp

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestAnMCPSaveOnAnAdminsOnlyDatasourceOutsideAnAdminsOnlyFolderAnswersRestrictedContent(t *testing.T) {
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: promStub{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := datasources.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: promStub{}}); err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(database.DB)
	base, err := url.Parse(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.New(tools.Deps{Datasources: datasources, Runner: runner, Dashboards: dashboards.New(database.DB, runner, access, logger), Folders: access, PublicURL: base, Logger: logger})
	authn := keyAuth{bearer(adminKey): Credential{Principal: adminKey}}
	f := &accessFixture{fixture: &fixture{handler: NewHandler(reg, authn, Options{PublicURL: base, Version: "test", Logger: logger}), tools: offeredOverMCP(reg)}, db: database, authn: authn, registry: reg}

	spec := strings.Replace(boardSpec("Salaries"), `"datasource":"prom"`, `"datasource":"audit"`, 1)
	res := f.as(t, adminKey, "create_dashboard", map[string]any{"spec": json.RawMessage(spec)})
	got := text(res)
	if !res.IsError || !strings.Contains(got, `"code":"restricted_content"`) || !strings.Contains(got, `datasource \"audit\"`) {
		t.Fatalf("create_dashboard over MCP = %s, want restricted_content naming the datasource", got)
	}
	var dashboardsSaved int
	if err := database.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM dashboards`).Scan(&dashboardsSaved); err != nil || dashboardsSaved != 0 {
		t.Fatalf("dashboards saved = %d (%v), want 0", dashboardsSaved, err)
	}
}
