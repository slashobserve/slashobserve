package claims

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

type RunAccess struct {
	Request revisions.Request
	Mode    grantmode.Mode
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
	return RunAccess{Request: req, Mode: grantmode.Parse(held.GrantMode)}, true, nil
}

func (s *Service) RecordStep(ctx context.Context, req revisions.Request, st revisions.Step) {
	s.steps.RecordStep(ctx, req, st)
}
