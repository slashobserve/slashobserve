package provision_test

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/provision"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestAProvisionedFileOnAnAdminsOnlyDatasourceOutsideAnAdminsOnlyFolderIsRefusedAsRestrictedContent(t *testing.T) {
	r := newRig(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: promStub{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: promStub{}}); err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(r.db.DB)
	dash := dashboards.New(r.db.DB, queryproxy.NewRunner(registry, logger), access, logger)
	salaries := strings.Replace(specJSON("Salaries", "up"), `"datasource":"prom"`, `"datasource":"audit"`, 1)
	wantMessage := func(what, message string) {
		t.Helper()
		if !strings.Contains(message, `datasource "audit"`) || !strings.Contains(message, "folder only Admins can read") {
			t.Fatalf("%s message = %q, want the restricted_content text naming the datasource", what, message)
		}
	}

	user, err := r.db.Queries().UpsertUser(reading(t.Context()), dbsqlc.UpsertUserParams{OidcIssuer: "issuer", OidcSubject: "root", Email: "root@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	admin := auth.Principal{Kind: auth.KindUser, ID: user.ID.String(), Name: "root", Role: auth.RoleAdmin}
	applied, err := provision.NewApplier(r.db.DB, dash, access).Apply(reading(t.Context()), admin, "salaries.json", json.RawMessage(salaries))
	if err != nil {
		t.Fatal(err)
	}
	if applied.Outcome != provision.OutcomeFailed || applied.Code != provision.CodeRestricted {
		t.Fatalf("apply = %+v, want failed with %s", applied, provision.CodeRestricted)
	}
	wantMessage("apply", applied.Message)

	r.write(t, "salaries.json", salaries)
	if err := provision.New(logger, dash, access, r.db.DB, r.dir).Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := r.report(t)
	if len(report.Dashboards) != 0 || len(report.Errors) != 1 || report.Errors[0].Path != "salaries.json" {
		t.Fatalf("report = %+v, want one refused file", report)
	}
	wantMessage("directory", report.Errors[0].Message)
}
