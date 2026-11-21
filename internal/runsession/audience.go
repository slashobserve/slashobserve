package runsession

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func writesOf(row dbsqlc.GetRunWritesRow) tools.RunWrites {
	var w tools.RunWrites
	adminsOnly := true
	add := func(admins bool, folder pgtype.UUID) {
		w.Shared, adminsOnly = true, adminsOnly && admins
		if folder.Valid && !slices.Contains(w.Folders, folder) {
			w.Folders = append(w.Folders, folder)
		}
	}
	if row.InThread {
		add(row.ThreadAdminsOnly, row.ThreadFolderID)
	}
	if row.IntoObservation {
		add(row.ObservationAdminsOnly, row.ObservationFolderID)
	}
	if row.OnDashboard && revisions.Intent(row.Intent) != revisions.IntentAsk {
		add(row.DashboardAdminsOnly, row.DashboardFolderID)
	}
	if row.Building {
		add(row.BuildAdminsOnly, row.BuildFolderID)
	}
	w.AdminsOnly = w.Shared && adminsOnly
	return w
}

func (s *Service) runWrites(ctx context.Context, req revisions.Request) (tools.RunWrites, error) {
	id, err := db.ParseUUID(req.ID)
	if err != nil {
		return tools.RunWrites{}, fmt.Errorf("run session: run id: %w", err)
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
		return tools.RunWrites{}, err
	}
	return writesOf(row), nil
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
