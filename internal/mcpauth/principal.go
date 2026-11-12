package mcpauth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

type Access struct {
	Principal auth.Principal
	Client    string
	GrantID   string
	Mode      grantmode.Mode
}

func (s *Server) Authenticate(ctx context.Context, header string) (Access, bool, error) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !opaquetoken.Shaped(token, accessPrefix) {
		return Access{}, false, nil
	}
	row, err := s.db.Queries().GetOAuthAccess(ctx, dbsqlc.GetOAuthAccessParams{TokenHash: opaquetoken.Hash(token), Resource: s.resource})
	if errors.Is(err, pgx.ErrNoRows) {
		return Access{}, false, nil
	}
	if err != nil {
		return Access{}, false, err
	}
	principal, err := s.people.Principal(ctx, auth.KindUser, row.UserID.String())
	if errors.Is(err, auth.ErrPrincipalGone) {
		return Access{}, false, nil
	}
	if err != nil {
		return Access{}, false, err
	}
	if !row.LastUsedAt.Valid || time.Since(row.LastUsedAt.Time) > grantTouchInterval {
		s.touch(ctx, row.ID)
	}
	return Access{Principal: principal, Client: row.Name.String, GrantID: row.ID.String(), Mode: grantmode.Parse(row.Mode)}, true, nil
}

func (s *Server) touch(ctx context.Context, grantID pgtype.UUID) {
	tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), grantTouchTimeout)
	defer cancel()
	err := s.db.Queries().TouchOAuthGrant(tctx, dbsqlc.TouchOAuthGrantParams{
		ID:     grantID,
		MinAge: pgtype.Interval{Microseconds: grantTouchInterval.Microseconds(), Valid: true},
	})
	if err != nil {
		s.logger.WarnContext(ctx, "update oauth grant last used", logattrs.Failure("oauth_grant_touch_failed", err)...)
	}
}
