package threads

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type Runs struct{}

func NewRuns() *Runs {
	return &Runs{}
}

func (r *Runs) RunEnded(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) error {
	if !run.ThreadMessageID.Valid {
		return nil
	}
	return replyForRun(ctx, q, run, run.ThreadMessageID)
}
