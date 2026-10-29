package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	keyPrefix        = "sok_"
	keyLength        = 47
	keyTouchInterval = 5 * time.Minute
	keyTouchTimeout  = 2 * time.Second
	userSeenInterval = time.Minute
)

func (s *Service) require(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h := c.GetHeader("Authorization"); h != "" {
		p, ok, err := s.bearerPrincipal(c.Request.Context(), h)
		if err != nil {
			if clientgone.Handle(c, s.logger, "resolve bearer key", err) {
				return
			}
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
	row, ok, err := s.sessionRow(c.Request)
	if err != nil {
		if clientgone.Handle(c, s.logger, "resolve session", err) {
			return
		}
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
	s.touchSeen(c.Request.Context(), row)
	c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), s.sessionPrincipalOf(c.Request.Context(), row)))
	c.Next()
}

func (s *Service) Valid(r *http.Request) (bool, error) {
	if h := r.Header.Get("Authorization"); h != "" {
		_, ok, err := s.lookupKey(r.Context(), h)
		return ok, err
	}
	_, ok, err := s.sessionRow(r)
	return ok, err
}

func (s *Service) sessionPrincipal(c *gin.Context) (Principal, bool, error) {
	row, ok, err := s.sessionRow(c.Request)
	if err != nil || !ok {
		return Principal{}, false, err
	}
	s.touchSeen(c.Request.Context(), row)
	return s.sessionPrincipalOf(c.Request.Context(), row), true, nil
}

func (s *Service) sessionRow(r *http.Request) (dbsqlc.GetSessionUserRow, bool, error) {
	cookie, err := r.Cookie(s.sessionCookie)
	if err != nil || cookie.Value == "" {
		return dbsqlc.GetSessionUserRow{}, false, nil
	}
	row, err := s.queries.GetSessionUser(r.Context(), hashToken(cookie.Value))
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.GetSessionUserRow{}, false, nil
	}
	if err != nil {
		return dbsqlc.GetSessionUserRow{}, false, err
	}
	return row, true, nil
}

func (s *Service) touchSeen(ctx context.Context, row dbsqlc.GetSessionUserRow) {
	if time.Since(row.LastSeenAt.Time) <= userSeenInterval {
		return
	}
	tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), keyTouchTimeout)
	err := s.queries.TouchUserSeen(tctx, dbsqlc.TouchUserSeenParams{ID: row.ID, MinAge: pgtype.Interval{Microseconds: userSeenInterval.Microseconds(), Valid: true}})
	cancel()
	if err != nil {
		s.logger.WarnContext(ctx, "update user last seen", logattrs.Failure("user_touch_failed", err)...)
	}
}

func (s *Service) sessionPrincipalOf(ctx context.Context, row dbsqlc.GetSessionUserRow) Principal {
	return Principal{Kind: KindUser, ID: row.ID.String(), Email: row.Email, Name: row.Name, Role: s.storedRole(ctx, "user_id", row.ID.String(), row.Role), Groups: row.Groups}
}

func (s *Service) storedRole(ctx context.Context, idAttr, id, stored string) Role {
	role, ok := ParseRole(stored)
	if !ok {
		s.logger.WarnContext(ctx, "stored role is not a known role; using viewer", slog.String(idAttr, id))
		return RoleViewer
	}
	return role
}

func (s *Service) lookupKey(ctx context.Context, header string) (dbsqlc.GetKeyPrincipalRow, bool, error) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || len(token) != keyLength || !strings.HasPrefix(token, keyPrefix) {
		return dbsqlc.GetKeyPrincipalRow{}, false, nil
	}
	row, err := s.queries.GetKeyPrincipal(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.GetKeyPrincipalRow{}, false, nil
	}
	if err != nil {
		return dbsqlc.GetKeyPrincipalRow{}, false, err
	}
	return row, true, nil
}

func (s *Service) bearerPrincipal(ctx context.Context, header string) (Principal, bool, error) {
	row, ok, err := s.lookupKey(ctx, header)
	if err != nil || !ok {
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
	return Principal{Kind: KindServiceAccount, ID: row.ServiceAccountID.String(), Name: row.Name, Role: s.storedRole(ctx, "service_account_id", row.ServiceAccountID.String(), row.Role)}, true, nil
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}
