package placement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

func Answer(ctx context.Context, q *dbsqlc.Queries, runID, folderID pgtype.UUID) error {
	restricted, err := folders.RunAnswerKeptWithAdmins(ctx, q, runID)
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
	restricted, err := folders.RunAnswerKeptWithAdmins(ctx, q, runID)
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

type keptWithAdmins struct{ Restricted, InFlight bool }

func refuseRestrictedAnswerMove(ctx context.Context, q *dbsqlc.Queries, kind folders.ContentKind, to pgtype.UUID, kept func() (keptWithAdmins, error), restrictedAnswer func() (folders.AnswerRef, error)) error {
	folder, err := q.ShareFolderPlacement(ctx, to)
	if err == nil && folder.AdminsOnly {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock folder: %w", err)
	}
	confinement, err := confinementOf(kind, kept)
	if err != nil {
		return err
	}
	switch confinement {
	case folders.ConfinedPermanent:
		answer, err := restrictedAnswer()
		if err != nil {
			slog.Default().WarnContext(ctx, "could not name the answer that stops a move", logattrs.Failure("restricted_answer_unnamed", err, slog.String("kind", string(kind)))...)
			answer = folders.AnswerRef{}
		}
		return folders.RestrictedAnswerMove(kind, answer)
	case folders.ConfinedRunInFlight:
		return folders.RunInFlightMove(kind)
	}
	return nil
}

func confinementOf(kind folders.ContentKind, kept func() (keptWithAdmins, error)) (folders.Confinement, error) {
	held, err := kept()
	if errors.Is(err, pgx.ErrNoRows) {
		return folders.NotConfined, nil
	}
	if err != nil {
		return folders.NotConfined, fmt.Errorf("read whether the %s holds an answer that read Admin-only data: %w", kind.Noun(), err)
	}
	return folders.ConfinementOf(held.Restricted, held.InFlight), nil
}

func observationKept(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) func() (keptWithAdmins, error) {
	return func() (keptWithAdmins, error) {
		row, err := q.ObservationKeptWithAdmins(ctx, id)
		return keptWithAdmins(row), err
	}
}

func ObservationConfinement(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) (folders.Confinement, error) {
	return confinementOf(folders.ContentObservation, observationKept(ctx, q, id))
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

type answerRun struct {
	ID          pgtype.UUID
	Prompt      string
	RequesterOf string
	RequesterID pgtype.UUID
}

func answerRef(ctx context.Context, run answerRun, err error) (folders.AnswerRef, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return folders.AnswerRef{}, nil
	}
	if err != nil {
		return folders.AnswerRef{}, err
	}
	caller, ok := auth.FromContext(ctx)
	if !mayNameAnswer(caller, ok, run) {
		return folders.AnswerRef{}, nil
	}
	return folders.AnswerRef{RunID: run.ID.String(), Prompt: run.Prompt}, nil
}

func mayNameAnswer(caller auth.Principal, known bool, run answerRun) bool {
	if !known {
		return false
	}
	return caller.Role == auth.RoleAdmin || (string(caller.Kind) == run.RequesterOf && caller.ID == run.RequesterID.String())
}
