package auth

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

const (
	loginStateTTL   = 10 * time.Minute
	maxReturnToSize = 2048

	maxDisplayNameRunes = 256
	maxEmailBytes       = 254
)

type loginFailure struct {
	code   string
	reason string
	err    error
	attrs  []any
}

func (s *Service) login(c *gin.Context) {
	ctx := c.Request.Context()
	provider, err := s.provider(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "oidc discovery failed", slog.Any("error", err))
		c.Redirect(http.StatusFound, loginErrorPath("unavailable"))
		return
	}

	returnTo := safeReturnTo(c.Query("return_to"))
	state, stateHash, nonce, verifier, err := newLoginSecrets()
	if err != nil {
		s.logger.ErrorContext(ctx, "generate login state", slog.Any("error", err))
		c.Redirect(http.StatusFound, loginErrorPath("internal"))
		return
	}
	if err := s.queries.DeleteExpiredLoginStates(ctx); err != nil {
		s.logger.WarnContext(ctx, "delete expired login states", slog.Any("error", err))
	}
	err = s.queries.CreateLoginState(ctx, dbsqlc.CreateLoginStateParams{
		StateHash:    stateHash,
		Nonce:        nonce,
		CodeVerifier: verifier,
		ReturnTo:     returnTo,
		ExpiresAt:    pgtype.Timestamptz{Time: time.Now().Add(loginStateTTL), Valid: true},
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "store login state", slog.Any("error", err))
		c.Redirect(http.StatusFound, loginErrorPath("internal"))
		return
	}

	s.setCookie(c, s.loginCookie, state, int(loginStateTTL.Seconds()))
	authURL := s.oauthConfig(provider).AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	c.Redirect(http.StatusFound, authURL)
}

func newLoginSecrets() (state string, stateHash []byte, nonce, verifier string, err error) {
	state, stateHash, err = newToken("")
	if err != nil {
		return "", nil, "", "", err
	}
	nonce, _, err = newToken("")
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
	stored, err := s.queries.ConsumeLoginState(ctx, hashToken(state))
	if errors.Is(err, pgx.ErrNoRows) {
		s.refuse(c, &loginFailure{code: "state_mismatch"})
		return
	}
	if err != nil {
		s.refuse(c, &loginFailure{code: "internal", err: err})
		return
	}
	if c.Query("error") != "" {
		s.refuse(c, &loginFailure{code: "access_denied"})
		return
	}

	token, failure := s.completeLogin(c, stored)
	if failure != nil {
		s.refuse(c, failure)
		return
	}
	if old, err := c.Request.Cookie(s.sessionCookie); err == nil && old.Value != "" {
		if err := s.queries.DeleteSession(ctx, hashToken(old.Value)); err != nil {
			s.logger.WarnContext(ctx, "delete previous session", slog.Any("error", err))
		}
	}
	s.setCookie(c, s.sessionCookie, token, int(s.cfg.SessionTTL.Seconds()))
	c.Redirect(http.StatusFound, stored.ReturnTo)
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
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", &loginFailure{code: "invalid_token", err: err}
	}
	if failure := emailFailure(claims.Email); failure != nil {
		return "", failure
	}
	if !s.cfg.AllowAnyEmailDomain && !emailAllowed(claims.Email, claims.EmailVerified, s.cfg.AllowedEmailDomains) {
		reason := "domain_not_allowed"
		if !claims.EmailVerified {
			reason = "email_unverified"
		}
		return "", &loginFailure{code: "email_not_allowed", reason: reason, attrs: []any{slog.String("email_domain", emailDomain(claims.Email))}}
	}

	user, err := s.queries.UpsertUser(ctx, dbsqlc.UpsertUserParams{
		OidcIssuer:  idToken.Issuer,
		OidcSubject: idToken.Subject,
		Email:       claims.Email,
		Name:        truncateRunes(strings.ReplaceAll(claims.Name, "\x00", ""), maxDisplayNameRunes),
	})
	if err != nil {
		return "", &loginFailure{code: "internal", err: err}
	}
	token, hash, err := newToken("")
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
		s.logger.WarnContext(ctx, "delete expired sessions", slog.Any("error", err))
	}
	return token, nil
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
	attrs := []any{slog.String("code", f.code)}
	if f.reason != "" {
		attrs = append(attrs, slog.String("reason", f.reason))
	}
	if f.err != nil {
		attrs = append(attrs, slog.Any("error", f.err))
	}
	attrs = append(attrs, f.attrs...)
	msg := "sign-in failed"
	if f.code == "email_not_allowed" {
		msg = "sign-in refused"
	}
	s.logger.WarnContext(c.Request.Context(), msg, attrs...)
	c.Redirect(http.StatusFound, loginErrorPath(f.code))
}

func (s *Service) logout(c *gin.Context) {
	if err := s.csrf.Check(c.Request); err != nil {
		abortError(c, http.StatusForbidden, "csrf_rejected", "cross-origin request rejected")
		return
	}
	if cookie, err := c.Request.Cookie(s.sessionCookie); err == nil && cookie.Value != "" {
		if err := s.queries.DeleteSession(c.Request.Context(), hashToken(cookie.Value)); err != nil {
			s.logger.ErrorContext(c.Request.Context(), "delete session", slog.Any("error", err))
			abortError(c, http.StatusInternalServerError, "internal", "internal error")
			return
		}
	}
	s.clearCookie(c, s.sessionCookie)
	c.Status(http.StatusNoContent)
}

func (s *Service) session(c *gin.Context) {
	p, ok, err := s.sessionPrincipal(c)
	if err != nil {
		s.logger.ErrorContext(c.Request.Context(), "resolve session", slog.Any("error", err))
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
