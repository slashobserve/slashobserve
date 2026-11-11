package threads

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type Runs struct {
	db *db.DB
}

func NewRuns(database *db.DB) *Runs {
	return &Runs{db: database}
}

func (r *Runs) ForRun(ctx context.Context, dashboardID, commentID pgtype.UUID) (pgtype.UUID, string, error) {
	row, err := r.db.Queries().GetThreadOfMessage(ctx, commentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, "", revisions.ErrCommentNotFound
	}
	if err != nil {
		return pgtype.UUID{}, "", fmt.Errorf("read comment thread: %w", err)
	}
	if row.DashboardID != dashboardID {
		return pgtype.UUID{}, "", revisions.ErrCommentNotFound
	}
	return row.ID, row.PanelID.String, nil
}

func (r *Runs) ResolveForRun(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) error {
	if !run.CommentID.Valid || !run.DashboardID.Valid || !run.ResultVersion.Valid {
		return nil
	}
	if _, err := q.LockDashboardKeyShareSkipLocked(ctx, run.DashboardID); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("lock dashboard: %w", err)
	}
	resolver := revisions.RunActor(run)
	columns, err := resolver.Columns()
	if err != nil {
		return err
	}
	thread, err := q.ResolveThreadForRun(ctx, dbsqlc.ResolveThreadForRunParams{
		ID:                   run.CommentID,
		DashboardID:          run.DashboardID,
		ResolvedByKind:       columns.Kind,
		ResolvedByID:         columns.ID,
		ResolvedByName:       columns.Name,
		ResolvedOrigin:       columns.Origin,
		ResolvedOriginClient: columns.Client,
		ResolvedRunID:        columns.RunID,
		ResolvedByVersion:    run.ResultVersion.Int32,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve comment thread: %w", err)
	}
	sc := scope{target: Target{Kind: KindDashboard, ID: run.DashboardID}, owner: run.DashboardID}
	return emitter{scope: sc, thread: thread, actor: resolver}.statusChanged(ctx, q)
}
