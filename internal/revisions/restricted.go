package revisions

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/placement"
)

func recordEnqueued(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) (dbsqlc.AgentRequest, error) {
	row, err := placement.InheritRestricted(ctx, q, row)
	if err != nil {
		return row, err
	}
	return row, recordRun(ctx, q, row, ActionEnqueue)
}
