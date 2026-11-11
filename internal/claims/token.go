package claims

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func (s *Service) Authenticate(ctx context.Context, header string) (revisions.Request, bool, error) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !opaquetoken.Shaped(token, tokenPrefix) {
		return revisions.Request{}, false, nil
	}
	held, err := s.db.Queries().GetRunTokenRun(ctx, opaquetoken.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return revisions.Request{}, false, nil
	}
	if err != nil {
		return revisions.Request{}, false, err
	}
	requester, err := s.runs.Requester(ctx, held.AgentRequest)
	if errors.Is(err, auth.ErrPrincipalGone) {
		return revisions.Request{}, false, nil
	}
	if err != nil {
		return revisions.Request{}, false, err
	}
	req := revisions.RequestOf(held.AgentRequest)
	req.RequestedBy = requester
	return req, true, nil
}

func (s *Service) RecordStep(ctx context.Context, req revisions.Request, st revisions.Step) {
	s.steps.RecordStep(ctx, req, st)
}
