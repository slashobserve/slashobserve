package mcpauth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func clientTarget(id, name string) audit.Target {
	return audit.Target{Kind: "oauth_client", ID: id, Name: origin.CleanClient(name)}
}

func (s *Server) registeredClient(ctx context.Context, id string) audit.Target {
	client, err := s.db.Queries().GetOAuthClient(ctx, id)
	if err != nil {
		return audit.Target{Kind: "oauth_client"}
	}
	return clientTarget(client.ID, client.Name.String)
}

func (s *Server) refused(ctx context.Context, action audit.Action, outcome audit.Outcome, reason string, target audit.Target, data map[string]any) {
	audit.Update(ctx, func(sc *audit.Scope) { sc.Origin.Client = target.Name })
	err := audit.RecordNow(ctx, audit.Event{Action: action, Outcome: outcome, Reason: reason, Target: target, Data: data})
	if err != nil && !errors.Is(err, audit.ErrNotInstalled) {
		s.logger.WarnContext(ctx, "record oauth event", logattrs.Failure("audit_record_failed", err)...)
	}
}

func (s *Server) refusedWith(ctx context.Context, action audit.Action, e *oauthError, target audit.Target, data map[string]any) {
	outcome := audit.OutcomeDenied
	if e.status >= 500 {
		outcome = audit.OutcomeFailed
	}
	s.refused(ctx, action, outcome, e.code, target, data)
}

func (s *Server) onBehalfOf(ctx context.Context, q *dbsqlc.Queries, grantID pgtype.UUID) (audit.Target, error) {
	subject, err := q.GetOAuthGrantSubject(ctx, grantID)
	if err != nil {
		return audit.Target{}, err
	}
	person := auth.Principal{Kind: auth.KindUser, ID: subject.UserID.String(), Name: subject.UserName}
	auth.Authenticated(ctx, person, audit.AuthOAuth, grantID.String())
	audit.Update(ctx, func(sc *audit.Scope) { sc.Origin.Client = subject.ClientName.String })
	return audit.Target{Kind: "oauth_grant", ID: grantID.String(), Name: origin.CleanClient(subject.ClientName.String)}, nil
}

func (s *Server) recordToken(ctx context.Context, q *dbsqlc.Queries, grantID pgtype.UUID, grantType string) error {
	target, err := s.onBehalfOf(ctx, q, grantID)
	if err != nil {
		return err
	}
	return audit.Record(ctx, q, audit.Event{Action: audit.ActionOAuthExchange, Target: target, Data: map[string]any{"grant_type": grantType}})
}

func (s *Server) recordRevoked(ctx context.Context, q *dbsqlc.Queries, grantID pgtype.UUID) error {
	target, err := s.onBehalfOf(ctx, q, grantID)
	if err != nil {
		return err
	}
	return audit.Record(ctx, q, audit.Event{Action: audit.ActionOAuthRevoke, Target: target})
}
