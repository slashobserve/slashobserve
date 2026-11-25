package folders

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func RunOf(ctx context.Context, who actor.Actor) (pgtype.UUID, error) {
	runID := cmp.Or(who.RunID, actor.RunIn(ctx))
	if runID == "" {
		return pgtype.UUID{}, nil
	}
	id, err := db.ParseUUID(runID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("actor run id: %w", err)
	}
	return id, nil
}

func RunAnswerKeptWithAdmins(ctx context.Context, q *dbsqlc.Queries, runID pgtype.UUID) (bool, error) {
	if !runID.Valid {
		return false, nil
	}
	kept, err := q.RunAnswerKeptWithAdmins(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read whether the run's answer stays where only Admins read: %w", err)
	}
	return kept, nil
}

func runWriteInto(ctx context.Context, q *dbsqlc.Queries, adminsOnly bool) error {
	run, err := RunOf(ctx, actor.Actor{})
	if err != nil || !run.Valid {
		return err
	}
	kept, err := RunAnswerKeptWithAdmins(ctx, q, run)
	if err != nil {
		return err
	}
	return ShowAnswer(kept, adminsOnly)
}
