package inboundmcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	skipNotOffered = "the server does not offer it"
	skipChanged    = "its definition changed since it was approved, so an Admin has to review it"
	skipKnown      = "the server already knew it and it is not approved, so only an Admin approves it"
)

var provisioningFile = auth.Principal{Name: "provisioning file"}

type skippedTool struct {
	Name   string
	Reason string
}

func listedApprovals(stored []Tool, names []string, known map[string]bool) ([]Tool, []skippedTool) {
	byName := make(map[string]Tool, len(stored))
	for _, t := range stored {
		byName[t.Name] = t
	}
	var approve []Tool
	var skipped []skippedTool
	for _, name := range names {
		t, ok := byName[name]
		switch {
		case !ok:
			skipped = append(skipped, skippedTool{Name: name, Reason: skipNotOffered})
		case t.State == ToolOffered && known[name]:
			skipped = append(skipped, skippedTool{Name: name, Reason: skipKnown})
		case t.State == ToolOffered:
			approve = append(approve, t)
		case t.State == ToolRefused:
			skipped = append(skipped, skippedTool{Name: name, Reason: t.Refusal})
		case t.State == ToolChanged:
			skipped = append(skipped, skippedTool{Name: name, Reason: skipChanged})
		case t.State == ToolGone:
			skipped = append(skipped, skippedTool{Name: name, Reason: skipNotOffered})
		}
	}
	return approve, skipped
}

func (s *Service) ApproveListed(ctx context.Context, entries []config.MCPSource) error {
	var errs []error
	for _, e := range entries {
		if len(e.Approve) == 0 {
			continue
		}
		if err := s.approveListedOf(ctx, e); err != nil {
			errs = append(errs, fmt.Errorf("MCP source %q: %w", e.UID, err))
		}
	}
	return errors.Join(errs...)
}

func names(tools []Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		out[t.Name] = true
	}
	return out
}

func (s *Service) approveListedOf(ctx context.Context, e config.MCPSource) error {
	before, err := s.Tools(ctx, e.UID)
	if err != nil {
		return err
	}
	found, err := s.Discover(ctx, provisioningFile, e.UID)
	if err != nil {
		return err
	}
	if found.Source.State != StateReady {
		s.logger.WarnContext(ctx, "MCP source tools not approved from the provisioning file: discovery failed", slog.String("uid", e.UID), slog.String("failure", found.Source.Failure))
		return nil
	}
	var approvedNames []string
	var skipped []skippedTool
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		source, err := q.LockMCPSource(ctx, e.UID)
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
		stored := make([]Tool, len(rows))
		for i, r := range rows {
			stored[i] = toolOf(r)
		}
		var toApprove []Tool
		toApprove, skipped = listedApprovals(stored, e.Approve, names(before))
		now := time.Now()
		approvedNames = nil
		for _, t := range toApprove {
			next, err := approved(t, provisioningFile, now)
			if err != nil {
				return err
			}
			if err := q.PutMCPSourceTool(ctx, putParams(source.ID, next)); err != nil {
				return err
			}
			if err := recordFileApproval(ctx, q, source, next); err != nil {
				return err
			}
			approvedNames = append(approvedNames, next.Name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, sk := range skipped {
		s.logger.WarnContext(ctx, "MCP source tool listed in the provisioning file was not approved", slog.String("uid", e.UID), slog.String("tool", sk.Name), slog.String("reason", sk.Reason))
	}
	if len(approvedNames) > 0 {
		s.logger.InfoContext(ctx, "MCP source tools approved from the provisioning file", slog.String("uid", e.UID), slog.Int("count", len(approvedNames)))
	}
	return nil
}
