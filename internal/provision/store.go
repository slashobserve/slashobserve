package provision

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

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
		return q.DeleteProvisionError(ctx, path)
	})
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
