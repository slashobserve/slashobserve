package revisions

import (
	"context"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s *Service) sweep(ctx context.Context, q *dbsqlc.Queries, savepoint db.Savepoint) error {
	exhausted, err := q.FailExhaustedAgentRequests(ctx, dbsqlc.FailExhaustedAgentRequestsParams{FromStates: RunMachine.From(ActionExhaust), MaxAttempts: MaxAttempts})
	if err != nil {
		return fmt.Errorf("fail exhausted agent requests: %w", err)
	}
	settled, err := q.SettleCanceledAgentRequests(ctx, dbsqlc.SettleCanceledAgentRequestsParams{FromStates: RunMachine.From(ActionSettle), Message: CanceledMessage})
	if err != nil {
		return fmt.Errorf("settle canceled agent requests: %w", err)
	}
	expired, err := q.ExpireUnclaimedAgentRequests(ctx, dbsqlc.ExpireUnclaimedAgentRequestsParams{
		FromStates: RunMachine.From(ActionExpire), Message: UnclaimedMessage, UnclaimedSeconds: UnclaimedFor.Seconds(),
	})
	if err != nil {
		return fmt.Errorf("expire unclaimed agent requests: %w", err)
	}
	for _, group := range []struct {
		rows   []dbsqlc.AgentRequest
		action Action
	}{{exhausted, ActionExhaust}, {settled, ActionSettle}, {expired, ActionExpire}} {
		for _, row := range group.rows {
			if err := s.recordEnded(ctx, q, savepoint, row, group.action); err != nil {
				return err
			}
		}
	}
	if err := s.retryThreadReplies(ctx, q, savepoint); err != nil {
		return err
	}
	s.deleteExpiredBuilds(ctx, savepoint)
	return nil
}

func (s *Service) retryThreadReplies(ctx context.Context, q *dbsqlc.Queries, savepoint db.Savepoint) error {
	if s.threads == nil {
		return nil
	}
	rows, err := q.ListRunsAwaitingReply(ctx, dbsqlc.ListRunsAwaitingReplyParams{
		Terminal: stateNames(RunMachine.Terminal), WithinSeconds: ReplyRetryFor.Seconds(), MaxRows: ReplyRetryBatch,
	})
	if err != nil {
		return fmt.Errorf("list runs awaiting a thread reply: %w", err)
	}
	for _, row := range rows {
		s.runEnded(ctx, savepoint, row)
	}
	return nil
}

func stateNames(states []State) []string {
	names := make([]string, len(states))
	for i, state := range states {
		names[i] = string(state)
	}
	return names
}

func (s *Service) Sweep(ctx context.Context) error {
	return s.db.InTxWithSavepoints(ctx, func(q *dbsqlc.Queries, savepoint db.Savepoint) error {
		if err := q.LockAgentRequestClaims(ctx); err != nil {
			return fmt.Errorf("lock agent request claims: %w", err)
		}
		return s.sweep(ctx, q, savepoint)
	})
}
