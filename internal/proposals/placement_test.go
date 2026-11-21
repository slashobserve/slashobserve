package proposals

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestAProposalThatReadsAnAdminsOnlyDatasourceIsRefusedOutsideAnAdminsOnlyFolder(t *testing.T) {
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddAdminsOnly(datasource.Instance{UID: "audit", Datasource: fakeDatasource{}}); err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	dash := dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	service := New(database.DB, access, dash, logger)
	dash.SetProposer(service.Propose)
	r := &rig{db: database, service: service, dashboards: dash, access: access}

	id := r.createDashboard(t, panel{"rps", "up"})
	_, err = r.dashboards.Save(reading(t.Context()), commenter, dashboards.SaveInput{
		DashboardID: id, Parent: 1, Summary: "read audit", ProposeIfNeeded: true,
		Spec: bytes.ReplaceAll(specOf("t", panel{"rps", "up"}), []byte(`"datasource":"prom"`), []byte(`"datasource":"audit"`)),
	})
	var refusal *folders.RestrictedContentError
	if !errors.As(err, &refusal) || refusal.Datasource != "audit" {
		t.Fatalf("err = %v, want a restricted content refusal naming audit", err)
	}
	var proposals int
	if err := database.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM proposals`).Scan(&proposals); err != nil || proposals != 0 {
		t.Fatalf("proposals after the refusal = %d %v", proposals, err)
	}
}
