package runsession

import (
	"context"
	"fmt"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func writesOf(row dbsqlc.GetRunWritesRow) tools.RunWrites {
	var adminsOnly []bool
	if row.InThread {
		adminsOnly = append(adminsOnly, row.ThreadAdminsOnly)
	}
	if row.IntoObservation {
		adminsOnly = append(adminsOnly, row.ObservationAdminsOnly)
	}
	if row.OnDashboard && revisions.Intent(row.Intent) != revisions.IntentAsk {
		adminsOnly = append(adminsOnly, row.DashboardAdminsOnly)
	}
	if row.Building {
		adminsOnly = append(adminsOnly, row.BuildAdminsOnly)
	}
	shared := len(adminsOnly) > 0
	return tools.RunWrites{Shared: shared, AdminsOnly: shared && !slices.Contains(adminsOnly, false)}
}

func (s *Service) runAccess(ctx context.Context, req revisions.Request) (datasource.Access, error) {
	id, err := db.ParseUUID(req.ID)
	if err != nil {
		return datasource.Access{}, fmt.Errorf("run session: run id: %w", err)
	}
	var row dbsqlc.GetRunWritesRow
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := q.ShareRunTargets(ctx, id); err != nil {
			return fmt.Errorf("lock where the run writes: %w", err)
		}
		row, err = q.GetRunWrites(ctx, id)
		if err != nil {
			return fmt.Errorf("read where the run writes: %w", err)
		}
		return nil
	})
	if err != nil {
		return datasource.Access{}, err
	}
	return tools.RunAccess(req, writesOf(row)), nil
}

func (s *Service) markReadRestricted(ctx context.Context, req revisions.Request) error {
	id, err := db.ParseUUID(req.ID)
	if err != nil {
		return fmt.Errorf("run session: run id: %w", err)
	}
	if err := s.db.Queries().MarkRunReadRestricted(context.WithoutCancel(ctx), id); err != nil {
		return fmt.Errorf("mark the run as having read Admin-only data: %w", err)
	}
	return nil
}
