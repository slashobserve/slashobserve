package claims

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const builtInAgentName = "Observe Agent"

type RunAccess struct {
	Request revisions.Request
	Mode    grantmode.Mode
	Agent   audit.Agent
}

func (s *Service) Authenticate(ctx context.Context, header string) (RunAccess, bool, error) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !opaquetoken.Shaped(token, tokenPrefix) {
		return RunAccess{}, false, nil
	}
	held, err := s.db.Queries().GetRunTokenRun(ctx, opaquetoken.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return RunAccess{}, false, nil
	}
	if err != nil {
		return RunAccess{}, false, err
	}
	requester, err := s.runs.Requester(ctx, held.AgentRequest)
	if errors.Is(err, auth.ErrPrincipalGone) {
		return RunAccess{}, false, nil
	}
	if err != nil {
		return RunAccess{}, false, err
	}
	req := revisions.RequestOf(held.AgentRequest)
	req.RequestedBy = requester
	return RunAccess{Request: req, Mode: grantmode.Parse(held.GrantMode), Agent: assignedAgent(held.AgentRequest, held.AssigneeName)}, true, nil
}

func assignedAgent(run dbsqlc.AgentRequest, name string) audit.Agent {
	id := uuid.NullUUID{UUID: run.AssignedID.Bytes, Valid: run.AssignedID.Valid}
	switch revisions.AssigneeKind(run.AssignedKind) {
	case revisions.AssignBuiltin:
		return audit.Agent{Name: builtInAgentName}
	case revisions.AssignGrant:
		return audit.Agent{ID: id, Name: origin.CleanClient(name)}
	}
	return audit.Agent{ID: id, Name: name}
}

func (s *Service) RecordStep(ctx context.Context, req revisions.Request, st revisions.Step) {
	s.steps.RecordStep(ctx, req, st)
}
