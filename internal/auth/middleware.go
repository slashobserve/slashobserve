package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (s *Service) require(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h := c.GetHeader("Authorization"); h != "" {
		abortError(c, http.StatusUnauthorized, "unauthenticated", "invalid credentials")
		return
	}
	p, ok, err := s.sessionPrincipal(c)
	if err != nil {
		s.logger.ErrorContext(c.Request.Context(), "resolve session", slog.Any("error", err))
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

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}
