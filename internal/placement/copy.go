package placement

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type Copied struct {
	Kind folders.ContentKind
	ID   pgtype.UUID
}

func Copy(ctx context.Context, q *dbsqlc.Queries, from Copied, folderID pgtype.UUID) (bool, error) {
	held, err := holdsRestrictedAnswer(ctx, q, from)
	if err != nil || !held {
		return false, err
	}
	folder, err := q.ShareFolderPlacement(ctx, folderID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, folders.RestrictedAnswerCopy(from.Kind)
	case err != nil:
		return false, fmt.Errorf("lock folder: %w", err)
	case !readOnlyByAdmins(folder):
		return false, folders.RestrictedAnswerCopy(from.Kind)
	}
	return true, nil
}

func CopyOnDashboard(ctx context.Context, q *dbsqlc.Queries, from Copied, dashboardID pgtype.UUID) (bool, error) {
	if !from.ID.Valid {
		return false, nil
	}
	folderID, err := q.ShareDashboardPlacement(ctx, dashboardID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("lock dashboard: %w", err)
	}
	return Copy(ctx, q, from, folderID)
}

func holdsRestrictedAnswer(ctx context.Context, q *dbsqlc.Queries, from Copied) (bool, error) {
	if !from.ID.Valid {
		return false, nil
	}
	held, err := q.DashboardHoldsRestrictedAnswer(ctx, from.ID)
	if from.Kind == folders.ContentObservation {
		held, err = q.ObservationHoldsRestrictedAnswer(ctx, from.ID)
	}
	if err != nil {
		return false, fmt.Errorf("read whether the %s holds an answer that read Admin-only data: %w", from.Kind.Noun(), err)
	}
	return held, nil
}
