package placement

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

func RunWrite(ctx context.Context, q *dbsqlc.Queries, who actor.Actor, folderID pgtype.UUID) error {
	run, err := folders.RunOf(ctx, who)
	if err != nil || !run.Valid {
		return err
	}
	return Answer(ctx, q, run, folderID)
}

func RunWriteOnDashboard(ctx context.Context, q *dbsqlc.Queries, who actor.Actor, dashboardID pgtype.UUID) error {
	run, err := folders.RunOf(ctx, who)
	if err != nil || !run.Valid {
		return err
	}
	return AnswerOnDashboard(ctx, q, run, dashboardID)
}
