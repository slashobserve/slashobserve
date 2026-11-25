package tools

import (
	"context"

	"github.com/google/uuid"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func attributed(ctx context.Context, tool string, run Run, inRun bool) context.Context {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return ctx
	}
	runID, err := uuid.Parse(run.RequestID)
	inRun = inRun && err == nil
	if inRun {
		ctx = actor.WithRun(ctx, runID.String())
	}
	from := origin.From(ctx)
	refine := func(s *audit.Scope) {
		s.Actor = auth.AuditActor(p)
		kind := string(from.Kind)
		if s.Origin.Kind == audit.OriginAgent {
			kind = audit.OriginAgent
		}
		s.Origin = audit.Origin{Kind: kind, Client: from.Client, Tool: tool}
		if inRun {
			s.Initiator = audit.InitiatorAgent
			s.RunID = uuid.NullUUID{UUID: runID, Valid: true}
		}
	}
	if _, scoped := audit.ScopeOf(ctx); scoped {
		audit.Update(ctx, refine)
		return ctx
	}
	scope := audit.Scope{Initiator: audit.InitiatorPerson, Auth: audit.Auth{Method: audit.AuthNone}}
	refine(&scope)
	return audit.Open(ctx, scope)
}
