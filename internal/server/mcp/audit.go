package mcp

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/runsession"
)

const actionToolCall audit.Action = "agent.tool_call"

var deniedCodes = []string{"forbidden_role", "forbidden", "user_session_required", revisions.CodeChatReadOnly, tools.CodeAdminsOnlyDatasource}

func openMCPScope(ctx context.Context) {
	audit.Update(ctx, func(s *audit.Scope) {
		s.Origin.Kind = string(origin.MCP)
	})
}

func attribute(ctx context.Context, cred Credential) {
	switch {
	case cred.OAuth && cred.Claimer != nil:
		auth.Authenticated(ctx, cred.Principal, audit.AuthOAuth, cred.Claimer.ID)
		audit.Update(ctx, func(s *audit.Scope) { s.Origin.Client = cred.Client })
	case cred.Run != nil:
		auth.Authenticated(ctx, cred.Principal, audit.AuthAPIKey, "")
		audit.Update(ctx, func(s *audit.Scope) { s.Agent = cred.Agent })
		if id, err := uuid.Parse(cred.Run.ID); err == nil {
			audit.Update(ctx, func(s *audit.Scope) { s.RunID = uuid.NullUUID{UUID: id, Valid: true} })
		}
		if cred.Run.Assignee == revisions.AssignBuiltin {
			audit.Update(ctx, func(s *audit.Scope) { s.Origin.Kind = audit.OriginAgent })
		}
	case cred.ClaimOnly:
		audit.Update(ctx, func(s *audit.Scope) { s.Auth = audit.Auth{Method: audit.AuthAPIKey} })
	}
}

func claimToolNames(all []tools.Tool) map[string]bool {
	names := map[string]bool{}
	for _, t := range all {
		if t.Claim {
			names[t.Name] = true
		}
	}
	return names
}

func (h *handler) recordToolCall(ctx context.Context, name string, kind callKind, took time.Duration, answer runsession.Answer, err error) {
	if kind == callClaimOnly || h.claimTools[name] {
		return
	}
	outcome, reason := audit.OutcomeSuccess, ""
	var data map[string]any
	switch {
	case err != nil:
		outcome, reason = audit.OutcomeFailed, tools.FailureCode(err)
	case answer.IsError:
		refusal := audit.ParseRefusal(answer.Content)
		reason = refusal.Code
		outcome = audit.OutcomeFailed
		if slices.Contains(deniedCodes, reason) {
			outcome = audit.OutcomeDenied
			data = refusal.Needs()
		}
	}
	audit.Update(ctx, func(s *audit.Scope) {
		if s.Origin.Tool == "" {
			s.Origin.Tool = name
		}
	})
	recordErr := audit.RecordNow(ctx, audit.Event{
		Action:   actionToolCall,
		Outcome:  outcome,
		Reason:   reason,
		Target:   audit.Target{Kind: "tool", ID: name},
		Data:     data,
		Duration: took,
	})
	if recordErr != nil && !errors.Is(recordErr, audit.ErrNotInstalled) {
		h.logger.WarnContext(ctx, "record mcp tool call", append(logattrs.Failure("audit_record_failed", recordErr), slog.String("tool", name))...)
	}
}
