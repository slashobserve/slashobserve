package provision

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type store struct {
	db *db.DB
}

func (s store) dashboards(ctx context.Context) ([]dbsqlc.ProvisionedDashboard, error) {
	return s.db.Queries().ListProvisionedDashboards(ctx)
}

func (s store) saved(ctx context.Context, path string, dashboardID pgtype.UUID, sum, folder string) error {
	return s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.UpsertProvisionedDashboard(ctx, dbsqlc.UpsertProvisionedDashboardParams{Path: path, DashboardID: dashboardID, ContentSha256: sum, FolderTitle: folder}); err != nil {
			return err
		}
		if err := q.DeleteProvisionError(ctx, path); err != nil {
			return err
		}
		return audit.Record(ctx, q, audit.Event{Action: provisionAction, Target: dashboardTarget(dashboardID)})
	})
}

func (s store) unchanged(ctx context.Context) error {
	return s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		return audit.Record(ctx, q, audit.Event{Action: provisionAction, Reason: reasonUnchanged, Target: audit.Target{Kind: "dashboard"}})
	})
}

func dashboardTarget(id pgtype.UUID) audit.Target {
	return audit.Target{Kind: "dashboard", ID: id.String(), DashboardID: uuid.NullUUID{UUID: id.Bytes, Valid: true}}
}

func (s store) dashboardFailed(ctx context.Context, path, message string) error {
	return s.db.Queries().SetProvisionedDashboardError(ctx, dbsqlc.SetProvisionedDashboardErrorParams{Path: path, LastError: message})
}

func (s store) fileFailed(ctx context.Context, path, message string) error {
	return s.db.Queries().UpsertProvisionError(ctx, dbsqlc.UpsertProvisionErrorParams{Path: path, Message: message})
}

func (s store) errors(ctx context.Context) ([]dbsqlc.ProvisionError, error) {
	return s.db.Queries().ListProvisionErrors(ctx)
}

func (s store) clearError(ctx context.Context, path string) error {
	return s.db.Queries().DeleteProvisionError(ctx, path)
}
