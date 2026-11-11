package webhooks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	attemptRetention = 7 * 24 * time.Hour
	pruneBatch       = 10000
)

func PruneJob(database *db.DB) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := prune(ctx, database.Queries(), time.Now().Add(-attemptRetention), pruneBatch)
		return err
	}
}

func prune(ctx context.Context, q *dbsqlc.Queries, cutoff time.Time, batch int32) (int64, error) {
	var total int64
	for ctx.Err() == nil {
		deleted, err := q.PruneWebhookDeliveries(ctx, dbsqlc.PruneWebhookDeliveriesParams{
			Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
			Batch:  batch,
		})
		if err != nil {
			return total, fmt.Errorf("prune webhook deliveries: %w", err)
		}
		total += deleted
		if deleted < int64(batch) {
			return total, nil
		}
	}
	return total, ctx.Err()
}
