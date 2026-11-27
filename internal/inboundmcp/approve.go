package inboundmcp

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
)

func isAdmin(who lifecycle.Who) bool { return who == lifecycle.Admin }

func approverOf(by auth.Principal) string {
	if by.Kind == auth.KindUser {
		return by.ID
	}
	return ""
}

func (s *Service) lockedTool(ctx context.Context, q *dbsqlc.Queries, uid, name string) (dbsqlc.McpSource, Tool, error) {
	source, err := q.LockMCPSource(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.McpSource{}, Tool{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.McpSource{}, Tool{}, err
	}
	row, err := q.GetMCPSourceTool(ctx, dbsqlc.GetMCPSourceToolParams{SourceID: source.ID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.McpSource{}, Tool{}, ErrToolNotFound
	}
	if err != nil {
		return dbsqlc.McpSource{}, Tool{}, err
	}
	return source, toolOf(row), nil
}

func approved(t Tool, by auth.Principal, now time.Time) (Tool, error) {
	if t.State == ToolRefused {
		return Tool{}, &RefusedError{Tool: t.Name, Reason: t.Refusal}
	}
	if _, ok := ToolMachine.Find(t.State, ActionApprove); !ok {
		return Tool{}, ToolMachine.Refuse(t.State, ActionApprove, isAdmin)
	}
	if t.State == ToolChanged && t.Seen != nil {
		t.Pinned = *t.Seen
	}
	t.State, t.Seen = ToolApproved, nil
	t.ApprovedBy, t.ApprovedAt = approverOf(by), now.UTC().Truncate(time.Microsecond)
	return t, nil
}

func reviewedHash(t Tool) string {
	if t.State == ToolChanged && t.Seen != nil {
		return t.Seen.Hash
	}
	return t.Pinned.Hash
}

func (s *Service) Approve(ctx context.Context, by auth.Principal, uid, name, hash string) (Tool, error) {
	var out Tool
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		source, current, err := s.lockedTool(ctx, q, uid, name)
		if err != nil {
			return err
		}
		next, err := approved(current, by, time.Now())
		if err != nil {
			return err
		}
		if hash != "" && hash != reviewedHash(current) {
			return ErrToolChanged
		}
		if err := q.PutMCPSourceTool(ctx, putParams(source.ID, next)); err != nil {
			return err
		}
		out = next
		return recordTool(ctx, q, actionApprove, source, next)
	})
	if err != nil {
		return Tool{}, err
	}
	s.logger.InfoContext(ctx, "MCP source tool approved", slog.String("actor_id", by.ID), slog.String("uid", uid), slog.String("tool", name))
	return out, nil
}

func (s *Service) ApproveReadOnly(ctx context.Context, by auth.Principal, uid string) ([]Tool, error) {
	var out []Tool
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		source, err := q.LockMCPSource(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		rows, err := q.ListMCPSourceTools(ctx, source.ID)
		if err != nil {
			return err
		}
		now := time.Now()
		out = nil
		for _, row := range rows {
			current := toolOf(row)
			if current.State != ToolOffered || !current.Pinned.ReadOnly {
				continue
			}
			next, err := approved(current, by, now)
			if err != nil {
				return err
			}
			if err := q.PutMCPSourceTool(ctx, putParams(source.ID, next)); err != nil {
				return err
			}
			if err := recordTool(ctx, q, actionApprove, source, next); err != nil {
				return err
			}
			out = append(out, next)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "MCP source tools approved", slog.String("actor_id", by.ID), slog.String("uid", uid), slog.Int("count", len(out)))
	return out, nil
}

func (s *Service) Revoke(ctx context.Context, by auth.Principal, uid, name string) (Tool, error) {
	var out Tool
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		source, current, err := s.lockedTool(ctx, q, uid, name)
		if err != nil {
			return err
		}
		if _, ok := ToolMachine.Find(current.State, ActionRevoke); !ok {
			return ToolMachine.Refuse(current.State, ActionRevoke, isAdmin)
		}
		next := current
		if current.State == ToolChanged && current.Seen != nil {
			next.Pinned = *current.Seen
		}
		next.State, next.Seen, next.ApprovedBy, next.ApprovedAt = ToolOffered, nil, "", time.Time{}
		if err := q.PutMCPSourceTool(ctx, putParams(source.ID, next)); err != nil {
			return err
		}
		out = next
		return recordTool(ctx, q, actionRevoke, source, next)
	})
	if err != nil {
		return Tool{}, err
	}
	s.logger.InfoContext(ctx, "MCP source tool revoked", slog.String("actor_id", by.ID), slog.String("uid", uid), slog.String("tool", name))
	return out, nil
}
