package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

const (
	loginStateTTL   = 10 * time.Minute
	maxLoginStates  = 10000
	maxReturnToSize = 2048

	maxDisplayNameRunes = 256
	maxEmailBytes       = 254
)

var oauthErrorPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

type loginFailure struct {
	code          string
	reason        string
	err           error
	attrs         []any
	target        audit.Target
	providerError string
}

func (f *loginFailure) providerErrorCode() string {
	if oauthErrorPattern.MatchString(f.providerError) {
		return f.providerError
	}
	var retrieveErr *oauth2.RetrieveError
	if errors.As(f.err, &retrieveErr) && oauthErrorPattern.MatchString(retrieveErr.ErrorCode) {
		return retrieveErr.ErrorCode
	}
	return ""
}

func (f *loginFailure) result() audit.Result {
	result := audit.Result{Action: "auth.login_failed", Outcome: f.outcome(), Reason: f.code, Target: f.target}
	if code := f.providerErrorCode(); code != "" {
		result.Data = map[string]any{"provider_error": code}
	}
	return result
}

func refuseLogin(c *gin.Context, code string) {
	audit.Conclude(c.Request.Context(), (&loginFailure{code: code}).result())
	c.Redirect(http.StatusFound, loginErrorPath(code))
}

func (f *loginFailure) outcome() audit.Outcome {
	switch f.code {
	case "internal", "unavailable", "exchange_failed":
		return audit.OutcomeFailed
	}
	return audit.OutcomeDenied
}

func (s *Service) login(c *gin.Context) {
	ctx := c.Request.Context()
	provider, err := s.provider(ctx)
	if errors.Is(err, errSignInOff) {
		refuseLogin(c, "sign_in_off")
		return
	}
	if err != nil {
		s.logger.WarnContext(ctx, "oidc discovery failed", logattrs.Failure("oidc_unavailable", err)...)
		refuseLogin(c, "unavailable")
		return
	}

	returnTo := safeReturnTo(c.Query("return_to"))
	state, stateHash, nonce, verifier, err := newLoginSecrets()
	if err != nil {
		s.logger.ErrorContext(ctx, "generate login state", logattrs.Failure("login_state_failed", err)...)
		refuseLogin(c, "internal")
		return
	}
	if err := s.queries.DeleteExpiredLoginStates(ctx); err != nil {
		s.logger.WarnContext(ctx, "delete expired login states", logattrs.Failure("login_state_cleanup_failed", err)...)
	}
	stored, err := s.queries.CreateLoginState(ctx, dbsqlc.CreateLoginStateParams{
		StateHash:      stateHash,
		Nonce:          nonce,
		CodeVerifier:   verifier,
		ReturnTo:       returnTo,
		ExpiresAt:      pgtype.Timestamptz{Time: time.Now().Add(loginStateTTL), Valid: true},
		MaxOutstanding: maxLoginStates,
	})
	if err == nil && stored == 0 {
		s.logger.WarnContext(ctx, "too many pending logins; refusing a new one", slog.Int("limit", maxLoginStates))
		refuseLogin(c, "unavailable")
		return
	}
	if err != nil {
		if clientgone.Handle(c, s.logger, "store login state", err) {
			return
		}
		s.logger.ErrorContext(ctx, "store login state", logattrs.Failure("login_state_failed", err)...)
		refuseLogin(c, "internal")
		return
	}

	s.setCookie(c, s.loginCookie, state, int(loginStateTTL.Seconds()))
	authURL := s.oauthConfig(provider).AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	c.Redirect(http.StatusFound, authURL)
}

func newLoginSecrets() (state string, stateHash []byte, nonce, verifier string, err error) {
	state, stateHash, err = opaquetoken.New("")
	if err != nil {
		return "", nil, "", "", err
	}
	nonce, _, err = opaquetoken.New("")
	if err != nil {
		return "", nil, "", "", err
	}
	return state, stateHash, nonce, oauth2.GenerateVerifier(), nil
}

func (s *Service) callback(c *gin.Context) {
	ctx := c.Request.Context()
	state := c.Query("state")
	cookie, cookieErr := c.Request.Cookie(s.loginCookie)
	s.clearCookie(c, s.loginCookie)

	if cookieErr != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		s.refuse(c, &loginFailure{code: "state_mismatch"})
		return
	}
	stored, err := s.queries.ConsumeLoginState(ctx, opaquetoken.Hash(state))
	if errors.Is(err, pgx.ErrNoRows) {
		s.refuse(c, &loginFailure{code: "state_mismatch"})
		return
	}
	if err != nil {
		s.refuse(c, &loginFailure{code: "internal", err: err})
		return
	}
	if c.Query("error") != "" {
		s.refuse(c, &loginFailure{code: "access_denied", providerError: c.Query("error")})
		return
	}

	token, failure := s.completeLogin(c, stored)
	if failure != nil {
		s.refuse(c, failure)
		return
	}
	s.concludeSignIn(ctx, token)
	if old, err := c.Request.Cookie(s.sessionCookie); err == nil && old.Value != "" {
		if err := s.queries.DeleteSession(ctx, opaquetoken.Hash(old.Value)); err != nil {
			s.logger.WarnContext(ctx, "delete previous session", logattrs.Failure("session_cleanup_failed", err)...)
		}
	}
	s.setCookie(c, s.sessionCookie, token, int(s.cfg.SessionTTL.Seconds()))
	c.Redirect(http.StatusFound, stored.ReturnTo)
}

func (s *Service) concludeSignIn(ctx context.Context, token string) {
	row, err := s.queries.GetSessionUser(ctx, opaquetoken.Hash(token))
	if err != nil {
		s.logger.WarnContext(ctx, "read the new session for the audit trail", logattrs.Failure("session_lookup_failed", err)...)
		return
	}
	s.sessionPrincipalOf(ctx, row)
	audit.Conclude(ctx, audit.Result{Action: "auth.login", Outcome: audit.OutcomeSuccess, Target: audit.Target{Kind: "session", ID: row.SessionID.String()}})
}

func (s *Service) completeLogin(c *gin.Context, stored dbsqlc.ConsumeLoginStateRow) (string, *loginFailure) {
	ctx := c.Request.Context()
	provider, err := s.provider(ctx)
	if err != nil {
		return "", &loginFailure{code: "unavailable", err: err}
	}
	oauthToken, err := s.oauthConfig(provider).Exchange(s.backChannel(ctx), c.Query("code"), oauth2.VerifierOption(stored.CodeVerifier))
	if err != nil {
		return "", &loginFailure{code: "exchange_failed", err: err}
	}
	rawIDToken, _ := oauthToken.Extra("id_token").(string)
	if rawIDToken == "" {
		return "", &loginFailure{code: "invalid_token", reason: "id_token_missing"}
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: s.cfg.ClientID}).Verify(s.backChannel(ctx), rawIDToken)
	if err != nil {
		return "", &loginFailure{code: "invalid_token", err: err}
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(stored.Nonce)) != 1 {
		return "", &loginFailure{code: "invalid_token", reason: "nonce_mismatch"}
	}

	var claims struct {
		Email         string          `json:"email"`
		EmailVerified bool            `json:"email_verified"`
		Name          string          `json:"name"`
		Groups        json.RawMessage `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", &loginFailure{code: "invalid_token", err: err}
	}
	if failure := s.emailRefusal(claims.Email, claims.EmailVerified); failure != nil {
		return "", failure
	}

	return s.signIn(ctx, identity{
		Issuer:        idToken.Issuer,
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          truncateRunes(strings.ReplaceAll(claims.Name, "\x00", ""), maxDisplayNameRunes),
		Groups:        groupsClaim(claims.Groups),
	})
}

type identity struct {
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Groups        []string
}

func (s *Service) signIn(ctx context.Context, id identity) (string, *loginFailure) {
	user, err := s.upsertUser(ctx, dbsqlc.UpsertUserParams{
		OidcIssuer:  id.Issuer,
		OidcSubject: id.Subject,
		Email:       id.Email,
		Name:        id.Name,
		Groups:      id.Groups,
	})
	if err != nil {
		return "", &loginFailure{code: "internal", err: err}
	}
	if user.DeactivatedAt.Valid {
		return "", &loginFailure{code: "account_deactivated", attrs: []any{slog.String("user_id", user.ID.String())}, target: audit.Target{Kind: "user", ID: user.ID.String()}}
	}
	if user.Inserted && id.EmailVerified {
		manual, err := s.queries.AcceptInvite(ctx, dbsqlc.AcceptInviteParams{ID: user.ID, Email: id.Email})
		switch {
		case err == nil:
			user.ManualRole = manual
			s.logger.InfoContext(ctx, "invite accepted", slog.String("user_id", user.ID.String()), slog.String("role", manual.String))
		case !errors.Is(err, pgx.ErrNoRows):
			return "", &loginFailure{code: "internal", err: err}
		}
	}
	if err := s.assignRole(ctx, user, id.Groups, id.EmailVerified); err != nil {
		return "", &loginFailure{code: "internal", err: err}
	}
	token, hash, err := opaquetoken.New("")
	if err != nil {
		return "", &loginFailure{code: "internal", err: err}
	}
	err = s.queries.CreateSession(ctx, dbsqlc.CreateSessionParams{
		TokenHash: hash,
		UserID:    user.ID,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.cfg.SessionTTL), Valid: true},
	})
	if err != nil {
		return "", &loginFailure{code: "internal", err: err}
	}
	if err := s.queries.DeleteExpiredSessions(ctx); err != nil {
		s.logger.WarnContext(ctx, "delete expired sessions", logattrs.Failure("session_cleanup_failed", err)...)
	}
	return token, nil
}

const firstUserIndex = "users_first_user_idx"

func (s *Service) upsertUser(ctx context.Context, params dbsqlc.UpsertUserParams) (dbsqlc.UpsertUserRow, error) {
	user, err := s.queries.UpsertUser(ctx, params)
	var pgErr *pgconn.PgError
	if isUniqueViolation(err) && errors.As(err, &pgErr) && pgErr.ConstraintName == firstUserIndex {
		return s.queries.UpsertUser(ctx, params)
	}
	return user, err
}

func (s *Service) emailRefusal(email string, verified bool) *loginFailure {
	if failure := emailFailure(email); failure != nil {
		return failure
	}
	if s.cfg.AllowAnyEmailDomain || emailAllowed(email, verified, s.cfg.AllowedEmailDomains) {
		return nil
	}
	reason := "domain_not_allowed"
	if !verified {
		reason = "email_unverified"
	}
	return &loginFailure{code: "email_not_allowed", reason: reason, attrs: []any{slog.String("email_domain", emailDomain(email))}}
}

func emailFailure(email string) *loginFailure {
	switch {
	case email == "":
		return &loginFailure{code: "email_missing"}
	case len(email) > maxEmailBytes:
		return &loginFailure{code: "email_not_allowed", reason: "email_too_long"}
	case strings.ContainsFunc(email, unicode.IsControl):
		return &loginFailure{code: "email_not_allowed", reason: "email_invalid"}
	}
	return nil
}

func truncateRunes(s string, limit int) string {
	if r := []rune(s); len(r) > limit {
		return string(r[:limit])
	}
	return s
}

func (s *Service) refuse(c *gin.Context, f *loginFailure) {
	audit.Conclude(c.Request.Context(), f.result())
	if f.err != nil && clientgone.Handle(c, s.logger, "sign-in", f.err) {
		return
	}
	attrs := []any{slog.String("error_code", f.code)}
	if f.reason != "" {
		attrs = append(attrs, slog.String("reason", f.reason))
	}
	if f.err != nil {
		attrs = append(attrs, logattrs.Safe(f.err)...)
		if code := f.providerErrorCode(); code != "" {
			attrs = append(attrs, slog.String("oauth_error", code))
		}
	}
	attrs = append(attrs, f.attrs...)
	msg := "sign-in failed"
	if f.code == "email_not_allowed" || f.code == "account_deactivated" {
		msg = "sign-in refused"
	}
	s.logger.WarnContext(c.Request.Context(), msg, attrs...)
	c.Redirect(http.StatusFound, loginErrorPath(f.code))
}

func (s *Service) logout(c *gin.Context) {
	if err := s.csrf.Check(c.Request); err != nil {
		rejectCrossOrigin(c)
		return
	}
	if cookie, err := c.Request.Cookie(s.sessionCookie); err == nil && cookie.Value != "" {
		if row, ok, err := s.sessionRow(c.Request); err == nil && ok {
			s.sessionPrincipalOf(c.Request.Context(), row)
		}
		if err := s.queries.DeleteSession(c.Request.Context(), opaquetoken.Hash(cookie.Value)); err != nil {
			if clientgone.Handle(c, s.logger, "delete session", err) {
				return
			}
			s.logger.ErrorContext(c.Request.Context(), "delete session", logattrs.Failure("session_delete_failed", err)...)
			abortError(c, http.StatusInternalServerError, "internal", "internal error")
			return
		}
	}
	s.ClearSessionCookie(c)
	c.Status(http.StatusNoContent)
}

func (s *Service) session(c *gin.Context) {
	p, ok, err := s.sessionPrincipal(c)
	if err != nil {
		if clientgone.Handle(c, s.logger, "resolve session", err) {
			return
		}
		s.logger.ErrorContext(c.Request.Context(), "resolve session", logattrs.Failure("session_lookup_failed", err)...)
		abortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if !ok {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "principal": p})
}

func (s *Service) me(c *gin.Context) {
	p, _ := FromContext(c.Request.Context())
	c.JSON(http.StatusOK, p)
}

func (s *Service) setCookie(c *gin.Context, name, value string, maxAge int) {
	//nolint:gosec // Secure is off only when the configured public URL is plain http (local development)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Service) ClearSessionCookie(c *gin.Context) {
	s.clearCookie(c, s.sessionCookie)
}

func (s *Service) clearCookie(c *gin.Context, name string) {
	s.setCookie(c, name, "", -1)
}

func loginErrorPath(code string) string {
	return "/login?error=" + url.QueryEscape(code)
}

func safeReturnTo(v string) string {
	if !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") ||
		strings.Contains(v, `\`) || len(v) > maxReturnToSize || slices.ContainsFunc([]byte(v), isControlByte) {
		return "/"
	}
	if u, err := url.Parse(v); err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	return v
}

func isControlByte(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func emailDomain(email string) string {
	i := strings.LastIndex(email, "@")
	if i < 0 {
		return ""
	}
	domain := email[i+1:]
	if slices.ContainsFunc([]byte(domain), func(b byte) bool { return b >= utf8.RuneSelf }) {
		return ""
	}
	return strings.ToLower(domain)
}

func emailAllowed(email string, verified bool, domains []string) bool {
	if !verified {
		return false
	}
	domain := emailDomain(email)
	return domain != "" && slices.Contains(domains, domain)
}
