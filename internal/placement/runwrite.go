package placement

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func RunOf(who actor.Actor) (pgtype.UUID, error) {
	if who.RunID == "" {
		return pgtype.UUID{}, nil
	}
	id, err := db.ParseUUID(who.RunID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("actor run id: %w", err)
	}
	return id, nil
}

func RunWrite(ctx context.Context, q *dbsqlc.Queries, who actor.Actor, folderID pgtype.UUID) error {
	run, err := RunOf(who)
	if err != nil || !run.Valid {
		return err
	}
	return Answer(ctx, q, run, folderID)
}

func RunWriteOnDashboard(ctx context.Context, q *dbsqlc.Queries, who actor.Actor, dashboardID pgtype.UUID) error {
	run, err := RunOf(who)
	if err != nil || !run.Valid {
		return err
	}
	return AnswerOnDashboard(ctx, q, run, dashboardID)
}
