package claims

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/db"
)

func PruneJob(database *db.DB) func(context.Context) error {
	return func(ctx context.Context) error {
		if _, err := database.Queries().PruneRunTokens(ctx); err != nil {
			return err
		}
		_, err := database.Queries().PruneAgentWorkers(ctx)
		return err
	}
}
