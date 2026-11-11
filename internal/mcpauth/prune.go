package mcpauth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/slashobserve/slashobserve/internal/db"
)

func PruneJob(database *db.DB) func(context.Context) error {
	return func(ctx context.Context) error {
		q := database.Queries()
		codes, err := pruneAll(ctx, q.PruneOAuthCodes)
		if err != nil {
			return err
		}
		tokens, err := pruneAll(ctx, q.PruneOAuthTokens)
		if err != nil {
			return err
		}
		grants, err := pruneAll(ctx, q.PruneOAuthGrants)
		if err != nil {
			return err
		}
		clients, err := pruneAll(ctx, q.PruneOAuthClients)
		if err != nil {
			return err
		}
		if err := q.DeleteExpiredOAuthRequests(ctx); err != nil {
			return err
		}
		slog.InfoContext(ctx, "oauth prune finished",
			slog.Int64("codes", codes), slog.Int64("tokens", tokens), slog.Int64("grants", grants), slog.Int64("clients", clients))
		return nil
	}
}

func pruneAll(ctx context.Context, prune func(context.Context) (int64, error)) (int64, error) {
	var total int64
	for ctx.Err() == nil {
		n, err := prune(ctx)
		if err != nil {
			return total, fmt.Errorf("prune oauth rows: %w", err)
		}
		total += n
		if n < pruneBatch {
			return total, nil
		}
	}
	return total, ctx.Err()
}
