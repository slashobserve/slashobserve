package threads

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	actionRunWithdraw audit.Action = "run.withdraw"

	CodeOthersWroteHere = "others_wrote"
)

var (
	ErrNothingShared   = errors.New("no answer of yours is shared in this thread")
	ErrOthersWroteHere = errors.New("someone else wrote in this thread, so the answer stays shared there")
)

func (s *Service) Withdraw(ctx context.Context, by auth.Principal, threadID pgtype.UUID) error {
	row, err := s.db.Queries().GetThreadOfMessage(ctx, threadID)
	if err != nil {
		return threadLookup(err)
	}
	sc, err := s.scopeOfThread(ctx, by, row)
	if err != nil {
		return err
	}
	requester, err := db.ParseUUID(by.ID)
	if err != nil {
		return ErrNothingShared
	}
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := lockTarget(ctx, q, sc, false); err != nil {
			return err
		}
		if _, err := q.LockThreadForWrite(ctx, row.ID); err != nil {
			return threadLookup(err)
		}
		runs, err := q.LockSharedRunsInThread(ctx, dbsqlc.LockSharedRunsInThreadParams{ThreadID: row.ID, RequestedByKind: string(by.Kind), RequestedByID: requester})
		if err != nil {
			return fmt.Errorf("lock the runs shared in the thread: %w", err)
		}
		if len(runs) == 0 {
			return ErrNothingShared
		}
		others, err := s.threadsWithOtherAuthors(ctx, q, by, []pgtype.UUID{row.ID})
		if err != nil {
			return err
		}
		if others[row.ID] {
			return ErrOthersWroteHere
		}
		ids, asked := make([]pgtype.UUID, len(runs)), make([]pgtype.UUID, len(runs))
		for i, r := range runs {
			ids[i], asked[i] = r.ID, r.ThreadMessageID
		}
		if err := q.WithdrawRunShares(ctx, ids); err != nil {
			return fmt.Errorf("withdraw the shared runs: %w", err)
		}
		if err := s.deleteShares(ctx, q, by, sc, row, ids, asked); err != nil {
			return err
		}
		if err := s.hideAnswers(ctx, q, row, ids); err != nil {
			return err
		}
		return s.recordWithdraws(ctx, q, by, row, runs)
	})
	return threadGone(err)
}

func (s *Service) hideAnswers(ctx context.Context, q *dbsqlc.Queries, thread dbsqlc.Thread, runs []pgtype.UUID) error {
	subject := targetOf(thread)
	if subject.Kind != KindObservation {
		return nil
	}
	if s.hideAnswer == nil {
		return errors.New("threads cannot take an answer back from an observation: no answer writer configured")
	}
	for _, run := range runs {
		if err := s.hideAnswer(ctx, q, run, subject.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) deleteShares(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, sc scope, thread dbsqlc.Thread, runs, asked []pgtype.UUID) error {
	emit := emitter{scope: sc, thread: thread, actor: actorOf(ctx, by)}
	if slices.Contains(asked, thread.ID) {
		if err := q.DeleteThread(ctx, thread.ID); err != nil {
			return fmt.Errorf("delete the thread: %w", err)
		}
		return emit.deleted(ctx, q, thread.ID)
	}
	deleted, err := q.DeleteSharedMessages(ctx, dbsqlc.DeleteSharedMessagesParams{ThreadID: thread.ID, MessageIds: asked, RunIds: runs})
	if err != nil {
		return fmt.Errorf("delete the shared messages: %w", err)
	}
	for _, id := range deleted {
		if err := emit.deleted(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordWithdraws(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, thread dbsqlc.Thread, runs []dbsqlc.LockSharedRunsInThreadRow) error {
	cols, err := actorOf(ctx, by).Columns()
	if err != nil {
		return err
	}
	subject := targetOf(thread)
	for _, run := range runs {
		target := audit.Target{Kind: "run", ID: run.ID.String()}
		on := uuid.NullUUID{UUID: subject.ID.Bytes, Valid: true}
		if subject.Kind == KindDashboard {
			target.DashboardID = on
		} else {
			target.ObservationID = on
		}
		err := audit.Record(ctx, q, audit.Event{
			Action: actionRunWithdraw,
			Target: target,
			Thread: uuid.NullUUID{UUID: thread.ID.Bytes, Valid: true},
			Chat:   uuid.NullUUID{UUID: run.ChatID.Bytes, Valid: run.ChatID.Valid},
			Data:   map[string]any{"finding_id": run.FindingID.String()},
			By:     cols.AuditBy(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}
