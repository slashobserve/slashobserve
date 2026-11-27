package provision

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var errEventNotRecorded = errors.New("the audit event was not recorded")

type store struct {
	db *db.DB
}

func inTxWithEvent(ctx context.Context, database *db.DB, event audit.Event, write func(*dbsqlc.Queries) error) error {
	err := database.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := write(q); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, event); err != nil {
			return fmt.Errorf("%w: %w", errEventNotRecorded, err)
		}
		return nil
	})
	if !errors.Is(err, errEventNotRecorded) {
		return err
	}
	audit.Dropped(audit.DroppedRecordFailed)
	return database.InTx(ctx, write)
}

func (s store) dashboards(ctx context.Context) ([]dbsqlc.ProvisionedDashboard, error) {
	return s.db.Queries().ListProvisionedDashboards(ctx)
}

func bind(ctx context.Context, q *dbsqlc.Queries, path string, dashboardID pgtype.UUID, sum, folder string) error {
	if err := q.UpsertProvisionedDashboard(ctx, dbsqlc.UpsertProvisionedDashboardParams{Path: path, DashboardID: dashboardID, ContentSha256: sum, FolderTitle: folder}); err != nil {
		return err
	}
	return q.DeleteProvisionError(ctx, path)
}

func (s store) recordSaved(ctx context.Context, dashboardID pgtype.UUID) {
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		return audit.Record(ctx, q, audit.Event{Action: provisionAction, Target: dashboardTarget(dashboardID)})
	})
	if err != nil {
		audit.Dropped(audit.DroppedRecordFailed)
	}
}

func (s store) unchanged(ctx context.Context) error {
	event := audit.Event{Action: provisionAction, Reason: reasonUnchanged, Target: audit.Target{Kind: "dashboard"}}
	return inTxWithEvent(ctx, s.db, event, func(*dbsqlc.Queries) error { return nil })
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

func RecordApplied(ctx context.Context, q *dbsqlc.Queries, dashboardID pgtype.UUID) {
	if err := audit.Record(ctx, q, audit.Event{Action: provisionAction, Target: dashboardTarget(dashboardID)}); err != nil {
		audit.Dropped(audit.DroppedRecordFailed)
	}
}
