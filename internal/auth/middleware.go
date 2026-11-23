package auth

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

const (
	keyPrefix        = "sok_"
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
			RejectCredential(c.Request.Context(), h)
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
	p := s.sessionPrincipalOf(c.Request.Context(), row)
	if !isSafeMethod(c.Request.Method) {
		if err := s.csrf.Check(c.Request); err != nil {
			rejectCrossOrigin(c)
			return
		}
	}
	s.touchSeen(c.Request.Context(), row)
	c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), p))
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

func (s *Service) RequestPrincipal(r *http.Request) (Principal, bool, error) {
	if h := r.Header.Get("Authorization"); h != "" {
		return s.bearerPrincipal(r.Context(), h)
	}
	row, ok, err := s.sessionRow(r)
	if err != nil || !ok {
		return Principal{}, false, err
	}
	return s.sessionPrincipalOf(r.Context(), row), true, nil
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
	if err != nil || cookie.Value == "" || !s.signInOn() {
		return dbsqlc.GetSessionUserRow{}, false, nil
	}
	row, err := s.queries.GetSessionUser(r.Context(), opaquetoken.Hash(cookie.Value))
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
	p := Principal{Kind: KindUser, ID: row.ID.String(), Email: row.Email, Name: row.Name, Role: s.storedRole(ctx, "user_id", row.ID.String(), row.Role), Groups: row.Groups}
	authenticated(ctx, p, audit.AuthSession, row.SessionID)
	return p
}

func authenticated(ctx context.Context, p Principal, method audit.AuthMethod, credential pgtype.UUID) {
	Authenticated(ctx, p, method, credential.String())
}

func Authenticated(ctx context.Context, p Principal, method audit.AuthMethod, credentialID string) {
	audit.Update(ctx, func(s *audit.Scope) {
		s.Actor = AuditActor(p)
		s.Auth = audit.Auth{Method: method, CredentialID: credentialID}
	})
}

func rejectCrossOrigin(c *gin.Context) {
	audit.Conclude(c.Request.Context(), audit.Result{Action: "auth.csrf_rejected", Outcome: audit.OutcomeDenied, Reason: "csrf_rejected"})
	abortError(c, http.StatusForbidden, "csrf_rejected", "cross-origin request rejected")
}

var credentialKinds = map[string]audit.AuthMethod{
	keyPrefix: audit.AuthAPIKey,
	"sot_":    audit.AuthAPIKey,
	"sob_":    audit.AuthAPIKey,
	"soa_":    audit.AuthOAuth,
	"sor_":    audit.AuthOAuth,
}

func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	return token, found && strings.EqualFold(scheme, "Bearer")
}

func RejectCredential(ctx context.Context, header string) {
	audit.Update(ctx, func(s *audit.Scope) { s.Auth = audit.Auth{Method: CredentialKind(header)} })
	audit.Conclude(ctx, rejectedCredential(header))
}

func rejectedCredential(header string) audit.Result {
	rejected := audit.Result{Action: "auth.credential_rejected", Outcome: audit.OutcomeDenied, Reason: "malformed_credential"}
	token, ok := bearerToken(header)
	if !ok {
		return rejected
	}
	for prefix := range credentialKinds {
		if opaquetoken.Shaped(token, prefix) {
			rejected.Reason = "unknown_credential"
			rejected.Target = audit.Target{Kind: "credential", Name: token[:displayPrefixLength]}
		}
	}
	return rejected
}

func CredentialKind(header string) audit.AuthMethod {
	if token, ok := bearerToken(header); ok {
		for prefix, kind := range credentialKinds {
			if strings.HasPrefix(token, prefix) {
				return kind
			}
		}
	}
	return audit.AuthNone
}

func AuditActor(p Principal) audit.Actor {
	return audit.Actor{Kind: audit.ActorKind(p.Kind), ID: auditID(p.ID), Name: cmp.Or(p.Name, p.Email), Role: string(p.Role)}
}

func auditID(id string) uuid.NullUUID {
	parsed, err := uuid.Parse(id)
	return uuid.NullUUID{UUID: parsed, Valid: err == nil}
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
	if !found || !strings.EqualFold(scheme, "Bearer") || !opaquetoken.Shaped(token, keyPrefix) {
		return dbsqlc.GetKeyPrincipalRow{}, false, nil
	}
	row, err := s.queries.GetKeyPrincipal(ctx, opaquetoken.Hash(token))
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
	p := Principal{Kind: KindServiceAccount, ID: row.ServiceAccountID.String(), Name: row.Name, Role: s.storedRole(ctx, "service_account_id", row.ServiceAccountID.String(), row.Role)}
	authenticated(ctx, p, audit.AuthAPIKey, row.KeyID)
	return p, true, nil
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}
