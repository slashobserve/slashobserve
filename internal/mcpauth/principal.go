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
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

func (s *Server) Authenticate(ctx context.Context, header string) (auth.Principal, string, bool, error) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !opaquetoken.Shaped(token, accessPrefix) {
		return auth.Principal{}, "", false, nil
	}
	row, err := s.db.Queries().GetOAuthAccess(ctx, dbsqlc.GetOAuthAccessParams{TokenHash: opaquetoken.Hash(token), Resource: s.resource})
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, "", false, nil
	}
	if err != nil {
		return auth.Principal{}, "", false, err
	}
	principal, err := s.people.Principal(ctx, auth.KindUser, row.UserID.String())
	if errors.Is(err, auth.ErrPrincipalGone) {
		return auth.Principal{}, "", false, nil
	}
	if err != nil {
		return auth.Principal{}, "", false, err
	}
	if !row.LastUsedAt.Valid || time.Since(row.LastUsedAt.Time) > grantTouchInterval {
		s.touch(ctx, row.ID)
	}
	return principal, row.Name.String, true, nil
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
