package mcpauth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	if !s.preflightOrMethod(w, r) {
		return
	}
	if !s.revokeLimit.allow("") {
		tooManyRequests(w, s.revokeLimit)
		return
	}
	values, bad := readForm(w, r)
	if bad != nil {
		writeOAuthError(w, bad)
		return
	}
	token, okToken := single(values, "token")
	clientID, okClient := single(values, "client_id")
	if !okToken || !okClient {
		writeOAuthError(w, invalidRequest("each parameter may be given only once"))
		return
	}
	if opaquetoken.Shaped(token, accessPrefix) || opaquetoken.Shaped(token, refreshPrefix) {
		if err := s.revokeToken(r, token, clientID); err != nil {
			s.fail(w, r, "revoke oauth token", "oauth_revoke_failed", err)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) revokeToken(r *http.Request, token, clientID string) error {
	ctx := r.Context()
	row, err := s.db.Queries().GetOAuthTokenGrant(ctx, opaquetoken.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.ClientID != clientID {
		s.refused(ctx, audit.ActionOAuthRevoke, audit.OutcomeDenied, "client_mismatch", clientTarget(clientID, ""), nil)
		return nil
	}
	revoked, err := s.revokeRecorded(ctx, row.GrantID, "client", nil, func(q *dbsqlc.Queries) (int64, error) {
		n, err := q.RevokeOAuthGrant(ctx, dbsqlc.RevokeOAuthGrantParams{ID: row.GrantID, Reason: "client"})
		if err != nil || n == 0 {
			return n, err
		}
		return n, s.recordRevoked(ctx, q, row.GrantID)
	})
	if err == nil && revoked > 0 {
		s.logger.InfoContext(ctx, "oauth grant revoked", slog.String("grant_id", row.GrantID.String()), slog.String("reason", "client"))
	}
	return err
}
