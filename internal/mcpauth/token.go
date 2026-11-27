package mcpauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

const (
	grantTypeCode    = "authorization_code"
	grantTypeRefresh = "refresh_token"
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type tokenForm struct {
	grantType    string
	code         string
	clientID     string
	redirectURI  string
	verifier     string
	resource     string
	refreshToken string
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if !s.preflightOrMethod(w, r) {
		return
	}
	if r.Header.Get("Authorization") != "" {
		writeOAuthError(w, errClientAuthentication)
		return
	}
	if !s.tokenLimit.allow("") {
		tooManyRequests(w, s.tokenLimit)
		return
	}
	values, bad := readForm(w, r)
	if bad != nil {
		writeOAuthError(w, bad)
		return
	}
	if _, secret := values["client_secret"]; secret {
		writeOAuthError(w, errClientAuthentication)
		return
	}
	form, ok := parseTokenForm(values)
	if !ok {
		writeOAuthError(w, invalidRequest("each parameter may be given only once"))
		return
	}
	if form.clientID == "" {
		writeOAuthError(w, invalidRequest("client_id is required"))
		return
	}
	if !s.tokenClient.allow(form.clientID) {
		tooManyRequests(w, s.tokenClient)
		return
	}
	var resp tokenResponse
	var err error
	switch form.grantType {
	case grantTypeCode:
		resp, err = s.exchangeCode(r.Context(), form)
	case grantTypeRefresh:
		resp, err = s.refresh(r.Context(), form)
	default:
		writeOAuthError(w, &oauthError{http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token"})
		return
	}
	var ended *endedGrant
	if errors.As(err, &ended) {
		s.refusedWith(r.Context(), audit.ActionOAuthExchange, ended.refusal(), s.registeredClient(r.Context(), form.clientID), map[string]any{"grant_type": form.grantType})
		writeEndedGrant(w, ended)
		return
	}
	var denied *oauthError
	if errors.As(err, &denied) {
		s.refusedWith(r.Context(), audit.ActionOAuthExchange, denied, s.registeredClient(r.Context(), form.clientID), map[string]any{"grant_type": form.grantType})
		writeOAuthError(w, denied)
		return
	}
	if err != nil {
		s.refused(r.Context(), audit.ActionOAuthExchange, audit.OutcomeFailed, "oauth_token_failed", s.registeredClient(r.Context(), form.clientID), map[string]any{"grant_type": form.grantType})
		s.fail(w, r, "issue oauth tokens", "oauth_token_failed", err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, resp)
}

var errClientAuthentication = &oauthError{http.StatusUnauthorized, "invalid_client", "clients are public and authenticate with PKCE only"}

func parseTokenForm(values url.Values) (tokenForm, bool) {
	var f tokenForm
	fields := map[string]*string{
		"grant_type":    &f.grantType,
		"code":          &f.code,
		"client_id":     &f.clientID,
		"redirect_uri":  &f.redirectURI,
		"code_verifier": &f.verifier,
		"resource":      &f.resource,
		"refresh_token": &f.refreshToken,
	}
	for name, dst := range fields {
		value, ok := single(values, name)
		if !ok {
			return tokenForm{}, false
		}
		*dst = value
	}
	return f, true
}

func (s *Server) exchangeCode(ctx context.Context, f tokenForm) (tokenResponse, error) {
	var resp tokenResponse
	if !opaquetoken.Shaped(f.code, codePrefix) {
		return resp, errInvalidGrant
	}
	hash := opaquetoken.Hash(f.code)
	var committed *oauthError
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		code, err := q.GetOAuthCode(ctx, hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidGrant
		}
		if err != nil {
			return err
		}
		if code.UsedAt.Valid {
			if err := s.revokeReusedCode(ctx, q, code.GrantID); err != nil {
				return err
			}
			committed = errCodeAlreadyUsed
			return nil
		}
		switch {
		case code.ClientID != f.clientID,
			code.RedirectUri != f.redirectURI,
			!pkceMatches(f.verifier, code.CodeChallenge):
			return errInvalidGrant
		case !code.ExpiresAt.Time.After(code.DbNow.Time):
			return errCodeExpired
		case f.resource != "" && !canonicalEqual(f.resource, code.Resource):
			return errInvalidTarget
		}
		if err := q.LockOAuthUser(ctx, code.UserID); err != nil {
			return err
		}
		if err := q.ReplaceLiveOAuthGrant(ctx, dbsqlc.ReplaceLiveOAuthGrantParams{ClientID: code.ClientID, UserID: code.UserID}); err != nil {
			return err
		}
		resp, err = s.connectGrant(ctx, q, code, hash)
		return err
	})
	if committed != nil {
		return tokenResponse{}, committed
	}
	return resp, err
}

func (s *Server) connectGrant(ctx context.Context, q *dbsqlc.Queries, code dbsqlc.GetOAuthCodeRow, hash []byte) (tokenResponse, error) {
	grant, err := q.CreateOAuthGrant(ctx, dbsqlc.CreateOAuthGrantParams{
		ClientID:  code.ClientID,
		UserID:    code.UserID,
		Resource:  code.Resource,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(refreshTTL), Valid: true},
	})
	if err != nil {
		return tokenResponse{}, err
	}
	_, err = q.UseOAuthCode(ctx, dbsqlc.UseOAuthCodeParams{CodeHash: hash, GrantID: grant.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return tokenResponse{}, errInvalidGrant
	}
	if err != nil {
		return tokenResponse{}, err
	}
	if err := recordConnected(ctx, q, grant.ID); err != nil {
		return tokenResponse{}, err
	}
	resp, err := s.issue(ctx, q, grant.ID, grant.CreatedAt.Time)
	if err != nil {
		return tokenResponse{}, err
	}
	return resp, s.recordToken(ctx, q, grant.ID, grantTypeCode)
}

func (s *Server) revokeReusedCode(ctx context.Context, q *dbsqlc.Queries, grantID pgtype.UUID) error {
	if err := revokeReused(ctx, q, grantID, "code_reuse"); err != nil {
		return err
	}
	s.logger.WarnContext(ctx, "oauth code reused; grant revoked", slog.String("grant_id", grantID.String()))
	return nil
}

func (s *Server) refresh(ctx context.Context, f tokenForm) (tokenResponse, error) {
	var resp tokenResponse
	if !opaquetoken.Shaped(f.refreshToken, refreshPrefix) {
		return resp, errInvalidGrant
	}
	hash := opaquetoken.Hash(f.refreshToken)
	var committed error
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.GetOAuthRefresh(ctx, hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidGrant
		}
		if err != nil {
			return err
		}
		if row.ClientID != f.clientID {
			return errInvalidGrant
		}
		if ended := endedRefresh(row); ended != nil {
			return ended
		}
		if row.RotatedAt.Valid {
			if row.DbNow.Time.Sub(row.RotatedAt.Time) <= refreshReuseLeeway {
				return errRefreshAlreadyUsed
			}
			if err := revokeReused(ctx, q, row.GrantID, "refresh_reuse"); err != nil {
				return err
			}
			s.logger.WarnContext(ctx, "oauth refresh token reused; grant revoked",
				slog.String("grant_id", row.GrantID.String()), slog.String("user_id", row.UserID.String()))
			committed = &endedGrant{state: GrantStateRevoked, revokedReason: "refresh_reuse"}
			return nil
		}
		if f.resource != "" && !canonicalEqual(f.resource, row.Resource) {
			return errInvalidTarget
		}
		if err := q.RotateOAuthRefresh(ctx, hash); err != nil {
			return err
		}
		resp, err = s.issue(ctx, q, row.GrantID, row.CreatedAt.Time)
		if err != nil {
			return err
		}
		return s.recordToken(ctx, q, row.GrantID, grantTypeRefresh)
	})
	if committed != nil {
		return tokenResponse{}, committed
	}
	return resp, err
}

func endedRefresh(row dbsqlc.GetOAuthRefreshRow) *endedGrant {
	state := grantStateOf(grantFacts{revoked: row.RevokedAt.Valid, created: row.CreatedAt.Time, expires: row.ExpiresAt.Time, now: row.DbNow.Time})
	if state == GrantStateActive {
		return nil
	}
	return &endedGrant{state: state, revokedReason: row.RevokedReason.String}
}

func (s *Server) issue(ctx context.Context, q *dbsqlc.Queries, grantID pgtype.UUID, grantCreated time.Time) (tokenResponse, error) {
	now := time.Now()
	refreshExpires := now.Add(refreshTTL)
	if limit := grantCreated.Add(grantMaxAge); limit.Before(refreshExpires) {
		refreshExpires = limit
	}
	accessLife := min(accessTTL, refreshExpires.Sub(now))
	if accessLife <= 0 {
		return tokenResponse{}, errInvalidGrant
	}
	access, accessHash, err := opaquetoken.New(accessPrefix)
	if err != nil {
		return tokenResponse{}, err
	}
	refresh, refreshHash, err := opaquetoken.New(refreshPrefix)
	if err != nil {
		return tokenResponse{}, err
	}
	for _, t := range []dbsqlc.CreateOAuthTokenParams{
		{TokenHash: accessHash, GrantID: grantID, Kind: "access", ExpiresAt: pgtype.Timestamptz{Time: now.Add(accessLife), Valid: true}},
		{TokenHash: refreshHash, GrantID: grantID, Kind: "refresh", ExpiresAt: pgtype.Timestamptz{Time: refreshExpires, Valid: true}},
	} {
		if err := q.CreateOAuthToken(ctx, t); err != nil {
			return tokenResponse{}, err
		}
	}
	if err := q.SetOAuthGrantExpiry(ctx, dbsqlc.SetOAuthGrantExpiryParams{ID: grantID, ExpiresAt: pgtype.Timestamptz{Time: refreshExpires, Valid: true}}); err != nil {
		return tokenResponse{}, err
	}
	return tokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int(accessLife / time.Second), RefreshToken: refresh}, nil
}
