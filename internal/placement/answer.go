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
	restricted, err := AnswerKeptWithAdmins(ctx, q, runID)
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
	restricted, err := AnswerKeptWithAdmins(ctx, q, runID)
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

func AnswerKeptWithAdmins(ctx context.Context, q *dbsqlc.Queries, runID pgtype.UUID) (bool, error) {
	if !runID.Valid {
		return false, nil
	}
	kept, err := q.RunAnswerKeptWithAdmins(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read whether the run's answer stays where only Admins read: %w", err)
	}
	return kept, nil
}

type keptWithAdmins struct{ Restricted, InFlight bool }

func refuseRestrictedAnswerMove(ctx context.Context, q *dbsqlc.Queries, kind folders.ContentKind, to pgtype.UUID, kept func() (keptWithAdmins, error), restrictedAnswer func() (folders.AnswerRef, error)) error {
	folder, err := q.ShareFolderPlacement(ctx, to)
	if err == nil && folder.AdminsOnly {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock folder: %w", err)
	}
	held, err := kept()
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("read whether the %s holds an answer that read Admin-only data: %w", kind.Noun(), err)
	case held.Restricted:
		answer, err := restrictedAnswer()
		if err != nil {
			return fmt.Errorf("read which answer the %s holds that read Admin-only data: %w", kind.Noun(), err)
		}
		return folders.RestrictedAnswerMove(kind, answer)
	case held.InFlight:
		return folders.RunInFlightMove(kind)
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
	items := make([]folders.ContentItem, 0, len(rows))
	for _, r := range rows {
		if r.Restricted == rows[0].Restricted {
			items = append(items, folders.ContentItem{Kind: folders.ContentKind(r.Kind), ID: r.ID.String(), Title: r.Title, FolderID: folderID.String()})
		}
	}
	if !rows[0].Restricted {
		return folders.RunInFlightOpening(items)
	}
	return folders.RestrictedAnswerOpening(items)
}

func answerRef(runID pgtype.UUID, prompt string, err error) (folders.AnswerRef, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return folders.AnswerRef{}, nil
	}
	if err != nil {
		return folders.AnswerRef{}, err
	}
	return folders.AnswerRef{RunID: runID.String(), Prompt: prompt}, nil
}
