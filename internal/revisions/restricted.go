package revisions

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func recordEnqueued(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) (dbsqlc.AgentRequest, error) {
	row, err := inheritReadRestricted(ctx, q, row)
	if err != nil {
		return row, err
	}
	return row, recordRun(ctx, q, row, ActionEnqueue)
}

func inheritReadRestricted(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) (dbsqlc.AgentRequest, error) {
	if row.ReadRestricted {
		return row, nil
	}
	carries, err := carriesRestrictedAnswer(ctx, q, row)
	if err != nil || !carries {
		return row, err
	}
	if err := q.MarkRunReadRestricted(ctx, row.ID); err != nil {
		return row, fmt.Errorf("mark a run that starts from an answer that read Admin-only data: %w", err)
	}
	row.ReadRestricted = true
	return row, nil
}

func carriesRestrictedAnswer(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) (bool, error) {
	checks := []func() (bool, error){func() (bool, error) { return q.RunInheritsReadRestricted(ctx, row.ID) }}
	if row.DashboardID.Valid {
		checks = append(checks, func() (bool, error) { return q.DashboardHoldsRestrictedAnswer(ctx, row.DashboardID) })
	}
	for _, id := range []pgtype.UUID{row.ObservationID, row.SubjectObservationID} {
		if id.Valid {
			checks = append(checks, func() (bool, error) { return q.ObservationHoldsRestrictedAnswer(ctx, id) })
		}
	}
	for _, check := range checks {
		carries, err := check()
		if err != nil {
			return false, fmt.Errorf("read whether the run starts from an answer that read Admin-only data: %w", err)
		}
		if carries {
			return true, nil
		}
	}
	return false, nil
}
