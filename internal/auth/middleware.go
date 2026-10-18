package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	keyPrefix        = "sok_"
	keyLength        = 47
	keyTouchInterval = 5 * time.Minute
	keyTouchTimeout  = 2 * time.Second
)

func (s *Service) require(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h := c.GetHeader("Authorization"); h != "" {
		p, ok, err := s.bearerPrincipal(c.Request.Context(), h)
		if err != nil {
			s.logger.ErrorContext(c.Request.Context(), "resolve bearer key", logattrs.Failure("key_lookup_failed", err)...)
			abortError(c, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		if !ok {
			abortError(c, http.StatusUnauthorized, "unauthenticated", "invalid credentials")
			return
		}
		c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), p))
		c.Next()
		return
	}
	p, ok, err := s.sessionPrincipal(c)
	if err != nil {
		s.logger.ErrorContext(c.Request.Context(), "resolve session", logattrs.Failure("session_lookup_failed", err)...)
		abortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if !ok {
		abortError(c, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	if !isSafeMethod(c.Request.Method) {
		if err := s.csrf.Check(c.Request); err != nil {
			abortError(c, http.StatusForbidden, "csrf_rejected", "cross-origin request rejected")
			return
		}
	}
	c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), p))
	c.Next()
}

func (s *Service) sessionPrincipal(c *gin.Context) (Principal, bool, error) {
	cookie, err := c.Request.Cookie(s.sessionCookie)
	if err != nil || cookie.Value == "" {
		return Principal{}, false, nil
	}
	row, err := s.queries.GetSessionUser(c.Request.Context(), hashToken(cookie.Value))
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, false, nil
	}
	if err != nil {
		return Principal{}, false, err
	}
	return Principal{Kind: KindUser, ID: row.ID.String(), Email: row.Email, Name: row.Name}, true, nil
}

func (s *Service) bearerPrincipal(ctx context.Context, header string) (Principal, bool, error) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || len(token) != keyLength || !strings.HasPrefix(token, keyPrefix) {
		return Principal{}, false, nil
	}
	row, err := s.queries.GetKeyPrincipal(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, false, nil
	}
	if err != nil {
		return Principal{}, false, err
	}
	if !row.LastUsedAt.Valid || time.Since(row.LastUsedAt.Time) > keyTouchInterval {
		tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), keyTouchTimeout)
		err = s.queries.TouchServiceAccountKey(tctx, row.KeyID)
		cancel()
		if err != nil {
			s.logger.WarnContext(ctx, "update key last used", logattrs.Failure("key_touch_failed", err)...)
		}
	}
	return Principal{Kind: KindServiceAccount, ID: row.ServiceAccountID.String(), Name: row.Name}, true, nil
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}
