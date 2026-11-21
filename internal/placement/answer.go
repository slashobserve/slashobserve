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

func Answer(ctx context.Context, q *dbsqlc.Queries, runID, folderID pgtype.UUID) error {
	restricted, err := readRestricted(ctx, q, runID)
	if err != nil || !restricted {
		return err
	}
	folder, err := q.ShareFolderPlacement(ctx, folderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return folders.ErrRestrictedAnswer
	}
	if err != nil {
		return fmt.Errorf("lock folder: %w", err)
	}
	return folders.ShowAnswer(restricted, folder.AdminsOnly)
}

func AnswerOnDashboard(ctx context.Context, q *dbsqlc.Queries, runID, dashboardID pgtype.UUID) error {
	restricted, err := readRestricted(ctx, q, runID)
	if err != nil || !restricted {
		return err
	}
	folderID, err := q.ShareDashboardPlacement(ctx, dashboardID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock dashboard: %w", err)
	}
	return Answer(ctx, q, runID, folderID)
}

func AppliedAnswer(ctx context.Context, q *dbsqlc.Queries, findingID, dashboardID pgtype.UUID) error {
	if !findingID.Valid {
		return nil
	}
	runID, err := q.FindingRun(ctx, findingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the run of the applied finding: %w", err)
	}
	return AnswerOnDashboard(ctx, q, runID, dashboardID)
}

func readRestricted(ctx context.Context, q *dbsqlc.Queries, runID pgtype.UUID) (bool, error) {
	if !runID.Valid {
		return false, nil
	}
	restricted, err := q.RunReadRestricted(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read whether the run read Admin-only data: %w", err)
	}
	return restricted, nil
}

func refuseRestrictedAnswerMove(ctx context.Context, q *dbsqlc.Queries, kind folders.ContentKind, to pgtype.UUID, holds func() (bool, error)) error {
	folder, err := q.ShareFolderPlacement(ctx, to)
	if err == nil && folder.AdminsOnly {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock folder: %w", err)
	}
	held, err := holds()
	if err != nil {
		return fmt.Errorf("read whether the %s holds an answer that read Admin-only data: %w", kind.Noun(), err)
	}
	if held {
		return folders.RestrictedAnswerMove(kind)
	}
	return nil
}

func restrictedAnswerOpening(ctx context.Context, q *dbsqlc.Queries, folderID pgtype.UUID) error {
	rows, err := q.RestrictedAnswerHolders(ctx, dbsqlc.RestrictedAnswerHoldersParams{FolderID: folderID, RowLimit: MaxListed})
	if err != nil {
		return fmt.Errorf("list content holding answers that read Admin-only data: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	items := make([]folders.ContentItem, len(rows))
	for i, r := range rows {
		items[i] = folders.ContentItem{Kind: folders.ContentKind(r.Kind), ID: r.ID.String(), Title: r.Title, FolderID: folderID.String()}
	}
	return folders.RestrictedAnswerOpening(items)
}
