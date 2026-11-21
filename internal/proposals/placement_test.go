package proposals

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestAProposalThatReadsAnAdminsOnlyDatasourceIsRefusedOutsideAnAdminsOnlyFolder(t *testing.T) {
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: fakeDatasource{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: fakeDatasource{}}); err != nil {
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

func TestARunThatReadAdminOnlyDataProposesOnlyWhereOnlyAdminsRead(t *testing.T) {
	r := newRig(t)
	admin := auth.Principal{Kind: auth.KindUser, ID: "88888888-8888-4888-8888-888888888888", Name: "Root", Role: auth.RoleAdmin}
	var adminsOnly pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO folders (title, admins_only) VALUES ('Audit notes', true) RETURNING id`).Scan(&adminsOnly); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name       string
		folder     pgtype.UUID
		restricted bool
		refused    bool
	}{
		{"a restricted run on a shared dashboard", pgtype.UUID{}, true, true},
		{"a restricted run on a dashboard only Admins read", adminsOnly, true, false},
		{"an unrestricted run on a shared dashboard", pgtype.UUID{}, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dash, _, err := r.dashboards.Create(reading(t.Context()), admin, c.folder, specOf("t", panel{"rps", "up"}), "create")
			if err != nil {
				t.Fatal(err)
			}
			var run pgtype.UUID
			if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO agent_requests (intent, dashboard_id, prompt, requested_by_kind, requested_by_id, requested_by_name, status, state, read_restricted)
				VALUES ('apply', $3::uuid, 'tidy it', 'user', $1::uuid, 'Root', 'running', 'running', $2) RETURNING id`, admin.ID, c.restricted, dash.ID).Scan(&run); err != nil {
				t.Fatal(err)
			}
			proposed, err := spec.Parse(specOf("t", panel{"rps", "up"}, panel{"errs", "rate(errors[5m])"}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = r.service.Propose(reading(t.Context()), admin, dashboards.ProposeInput{DashboardID: mustParse(t, dash.ID), BaseVersion: 1, Spec: proposed, Summary: "add errors", RequestID: run})
			if errors.Is(err, folders.ErrRestrictedAnswer) != c.refused || !c.refused && err != nil {
				t.Fatalf("propose = %v, want refused %v", err, c.refused)
			}
		})
	}
}
