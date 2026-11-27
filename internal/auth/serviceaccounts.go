package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/presence"
)

const (
	maxBodyBytes         = 64 << 10
	maxDescriptionLength = 256
	maxKeyNameLength     = 64
	maxKeyLifetime       = 366 * 24 * time.Hour
	displayPrefixLength  = 12
	uniqueViolation      = "23505"

	InvalidRoleMessage = "role must be admin, editor, commenter or viewer"
	provisionedMessage = "this service account comes from the service-accounts file; change its role, takesRuns or key in the file and restart the server"
)

var (
	errProvisioned = errors.New("service account is provisioned")
	errKeyNotFound = errors.New("key not found")
)

var serviceAccountName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

const ServiceAccountNameMessage = "name must be lowercase letters, digits and hyphens, start with a letter or digit, and be at most 63 characters"

func ValidServiceAccountName(name string) bool {
	return serviceAccountName.MatchString(name)
}

type serviceAccountJSON struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	CreatedBy   *string `json:"created_by"`
	CreatedAt   string  `json:"created_at"`
	Role        Role    `json:"role"`
	Provisioned bool    `json:"provisioned"`
	TakesRuns   bool    `json:"takes_runs"`
	Presence    string  `json:"presence"`
	LastSeenAt  *string `json:"last_seen_at"`
}

type keyJSON struct {
	ID               string  `json:"id"`
	ServiceAccountID string  `json:"service_account_id"`
	Name             string  `json:"name"`
	DisplayPrefix    string  `json:"display_prefix"`
	CreatedBy        *string `json:"created_by"`
	CreatedAt        string  `json:"created_at"`
	ExpiresAt        *string `json:"expires_at"`
	LastUsedAt       *string `json:"last_used_at"`
	RevokedAt        *string `json:"revoked_at"`
	State            string  `json:"state"`
}

func (s *Service) mountServiceAccounts(authenticated Routes) {
	accounts := authenticated.Group("/service-accounts")
	accounts.POST("", RoleAdmin, RequireUser, s.createServiceAccount)
	accounts.GET("", RoleAdmin, RequireUser, s.listServiceAccounts)
	accounts.PATCH("/:id", RoleAdmin, RequireUser, s.updateServiceAccount)
	accounts.DELETE("/:id", RoleAdmin, RequireUser, s.deleteServiceAccount)
	accounts.POST("/:id/keys", RoleAdmin, RequireUser, s.createKey)
	accounts.GET("/:id/keys", RoleAdmin, RequireUser, s.listKeys)
	accounts.DELETE("/:id/keys/:keyId", RoleAdmin, RequireUser, s.revokeKey)
}

func RequireUser(c *gin.Context) {
	if p, ok := FromContext(c.Request.Context()); !ok || p.Kind != KindUser {
		abortError(c, http.StatusForbidden, "forbidden", "service accounts cannot use this endpoint")
		return
	}
	c.Next()
}

func (s *Service) createServiceAccount(c *gin.Context) {
	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Role        *string `json:"role"`
	}
	if !decodeBody(c, &req) {
		return
	}
	role := RoleEditor
	if req.Role != nil {
		parsed, ok := parseRoleField(req.Role)
		if !ok {
			abortError(c, http.StatusBadRequest, "invalid_request", InvalidRoleMessage)
			return
		}
		role = parsed
	}
	if !ValidServiceAccountName(req.Name) {
		abortError(c, http.StatusBadRequest, "invalid_request", ServiceAccountNameMessage)
		return
	}
	if utf8.RuneCountInString(req.Description) > maxDescriptionLength || hasControlChars(req.Description) {
		abortError(c, http.StatusBadRequest, "invalid_request", "description must be at most 256 characters without control characters")
		return
	}
	ctx := c.Request.Context()
	caller, _ := FromContext(ctx)
	var account dbsqlc.ServiceAccount
	err := s.inTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		account, err = q.CreateServiceAccount(ctx, dbsqlc.CreateServiceAccountParams{
			Name:        req.Name,
			Description: req.Description,
			CreatedBy:   callerID(c),
			Role:        string(role),
		})
		if err != nil {
			return err
		}
		return s.changes.ServiceAccountCreated(ctx, q, caller, account)
	})
	if isUniqueViolation(err) {
		abortError(c, http.StatusConflict, "conflict", "a service account with this name already exists")
		return
	}
	if err != nil {
		s.internalError(c, "create service account", err)
		return
	}
	c.JSON(http.StatusCreated, newServiceAccountJSON(account, dbsqlc.ListServiceAccountPresenceRow{}))
}

func (s *Service) applyServiceAccountUpdate(ctx context.Context, caller Principal, id pgtype.UUID, role *Role, takesRuns *bool) (account dbsqlc.ServiceAccount, err error) {
	err = s.inTx(ctx, func(q *dbsqlc.Queries) error {
		before, err := q.GetServiceAccount(ctx, id)
		if err != nil {
			return err
		}
		if before.Provisioned {
			return errProvisioned
		}
		params := dbsqlc.UpdateServiceAccountParams{ID: id}
		if role != nil {
			params.Role = pgtype.Text{String: string(*role), Valid: true}
		}
		if takesRuns != nil {
			params.TakesRuns = pgtype.Bool{Bool: *takesRuns, Valid: true}
		}
		if account, err = q.UpdateServiceAccount(ctx, params); err != nil {
			return err
		}
		return s.changes.ServiceAccountUpdated(ctx, q, caller, before, account)
	})
	return account, err
}

func (s *Service) updateServiceAccount(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Role      *string `json:"role"`
		TakesRuns *bool   `json:"takes_runs"`
	}
	if !decodeBody(c, &req) {
		return
	}
	if req.Role == nil && req.TakesRuns == nil {
		abortError(c, http.StatusBadRequest, "invalid_request", "send role, takes_runs or both")
		return
	}
	var role *Role
	if req.Role != nil {
		parsed, ok := ParseRole(*req.Role)
		if !ok {
			abortError(c, http.StatusBadRequest, "invalid_request", InvalidRoleMessage)
			return
		}
		role = &parsed
	}
	ctx := c.Request.Context()
	caller, _ := FromContext(ctx)
	account, err := s.applyServiceAccountUpdate(ctx, caller, id, role, req.TakesRuns)
	if errors.Is(err, errProvisioned) {
		abortError(c, http.StatusConflict, "provisioned", provisionedMessage)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		abortError(c, http.StatusNotFound, "not_found", "service account not found")
		return
	}
	if err != nil {
		s.internalError(c, "update service account", err)
		return
	}
	target := []any{slog.String("actor_id", caller.ID), slog.String("target_kind", string(KindServiceAccount)), slog.String("target_id", account.ID.String())}
	if role != nil {
		s.logger.InfoContext(ctx, "role changed", append(target, slog.String("role", account.Role))...)
	}
	s.logger.InfoContext(ctx, "service account changed", append(target, slog.String("role", account.Role), slog.Bool("takes_runs", account.TakesRuns))...)
	rows, err := s.queries.ListServiceAccountPresence(ctx, []pgtype.UUID{account.ID})
	if err != nil {
		s.internalError(c, "read service account presence", err)
		return
	}
	c.JSON(http.StatusOK, newServiceAccountJSON(account, presenceByAccount(rows)[account.ID]))
}

func (s *Service) listServiceAccounts(c *gin.Context) {
	accounts, err := s.queries.ListServiceAccounts(c.Request.Context())
	if err != nil {
		s.internalError(c, "list service accounts", err)
		return
	}
	ids := make([]pgtype.UUID, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	rows, err := s.queries.ListServiceAccountPresence(c.Request.Context(), ids)
	if err != nil {
		s.internalError(c, "list service account presence", err)
		return
	}
	seen := presenceByAccount(rows)
	items := make([]serviceAccountJSON, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, newServiceAccountJSON(a, seen[a.ID]))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Service) createKey(c *gin.Context) {
	accountID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	name, expiresAt, ok := decodeKeyRequest(c)
	if !ok {
		return
	}
	token, hash, err := opaquetoken.New(keyPrefix)
	if err != nil {
		s.internalError(c, "generate key", err)
		return
	}
	ctx := c.Request.Context()
	caller, _ := FromContext(ctx)
	var row dbsqlc.CreateServiceAccountKeyRow
	err = s.inTx(ctx, func(q *dbsqlc.Queries) error {
		account, err := q.GetServiceAccount(ctx, accountID)
		if err != nil {
			return err
		}
		if account.Provisioned {
			return errProvisioned
		}
		if row, err = q.CreateServiceAccountKey(ctx, dbsqlc.CreateServiceAccountKeyParams{
			ServiceAccountID: accountID,
			Name:             name,
			DisplayPrefix:    token[:displayPrefixLength],
			TokenHash:        hash,
			CreatedBy:        callerID(c),
			ExpiresAt:        expiresAt,
		}); err != nil {
			return err
		}
		return s.changes.KeyCreated(ctx, q, caller, account, row)
	})
	if errors.Is(err, errProvisioned) {
		abortError(c, http.StatusConflict, "provisioned", provisionedMessage)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		abortError(c, http.StatusNotFound, "not_found", "service account not found")
		return
	}
	if err != nil {
		s.internalError(c, "create key", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": newKeyJSON(row, time.Now()), "token": token})
}

func decodeKeyRequest(c *gin.Context) (string, pgtype.Timestamptz, bool) {
	var req struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !decodeBody(c, &req) {
		return "", pgtype.Timestamptz{}, false
	}
	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxKeyNameLength || hasControlChars(req.Name) {
		abortError(c, http.StatusBadRequest, "invalid_request", "name must be 1 to 64 characters without control characters")
		return "", pgtype.Timestamptz{}, false
	}
	if req.ExpiresAt == nil {
		return req.Name, pgtype.Timestamptz{}, true
	}
	now := time.Now()
	if !req.ExpiresAt.After(now) || req.ExpiresAt.After(now.Add(maxKeyLifetime)) {
		abortError(c, http.StatusBadRequest, "invalid_request", "expires_at must be in the future and within 366 days")
		return "", pgtype.Timestamptz{}, false
	}
	return req.Name, pgtype.Timestamptz{Time: *req.ExpiresAt, Valid: true}, true
}

func (s *Service) listKeys(c *gin.Context) {
	accountID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if !s.serviceAccountExists(c, accountID) {
		return
	}
	rows, err := s.queries.ListServiceAccountKeys(c.Request.Context(), accountID)
	if err != nil {
		s.internalError(c, "list keys", err)
		return
	}
	now := time.Now()
	items := make([]keyJSON, 0, len(rows))
	for _, r := range rows {
		items = append(items, newKeyJSON(dbsqlc.CreateServiceAccountKeyRow(r), now))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Service) revokeKey(c *gin.Context) {
	accountID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	keyID, ok := parseUUIDParam(c, "keyId")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller, _ := FromContext(ctx)
	err := s.inTx(ctx, func(q *dbsqlc.Queries) error {
		account, err := q.GetServiceAccount(ctx, accountID)
		if err != nil {
			return err
		}
		if account.Provisioned {
			return errProvisioned
		}
		revoked, err := q.RevokeServiceAccountKey(ctx, dbsqlc.RevokeServiceAccountKeyParams{ID: keyID, ServiceAccountID: accountID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errKeyNotFound
		}
		if err != nil || !revoked.NewlyRevoked {
			return err
		}
		return s.changes.KeyRevoked(ctx, q, caller, account, keyID, revoked.Name)
	})
	switch {
	case errors.Is(err, errProvisioned):
		abortError(c, http.StatusConflict, "provisioned", provisionedMessage)
	case errors.Is(err, errKeyNotFound):
		abortError(c, http.StatusNotFound, "not_found", "key not found")
	case errors.Is(err, pgx.ErrNoRows):
		abortError(c, http.StatusNotFound, "not_found", "service account not found")
	case err != nil:
		s.internalError(c, "revoke key", err)
	default:
		c.Status(http.StatusNoContent)
	}
}

func (s *Service) serviceAccountExists(c *gin.Context, id pgtype.UUID) bool {
	_, err := s.queries.GetServiceAccount(c.Request.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		abortError(c, http.StatusNotFound, "not_found", "service account not found")
		return false
	}
	if err != nil {
		s.internalError(c, "get service account", err)
		return false
	}
	return true
}

func (s *Service) internalError(c *gin.Context, action string, err error) {
	if clientgone.Handle(c, s.logger, action, err) {
		return
	}
	principal, _ := FromContext(c.Request.Context())
	s.logger.ErrorContext(c.Request.Context(), action, logattrs.Failure("internal", err,
		slog.String("principal_kind", string(principal.Kind)), slog.String("principal_id", principal.ID))...)
	abortError(c, http.StatusInternalServerError, "internal", "internal error")
}

func decodeBody(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		abortError(c, http.StatusBadRequest, "invalid_request", "request body must be valid JSON with known fields")
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		abortError(c, http.StatusBadRequest, "invalid_request", "request body must be a single JSON object")
		return false
	}
	return true
}

func parseRoleField(raw *string) (Role, bool) {
	if raw == nil {
		return "", false
	}
	return ParseRole(*raw)
}

func hasControlChars(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
}

func parseUUIDParam(c *gin.Context, name string) (pgtype.UUID, bool) {
	id, err := db.ParseUUID(c.Param(name))
	if err != nil {
		abortError(c, http.StatusBadRequest, "invalid_request", name+" must be a UUID")
		return pgtype.UUID{}, false
	}
	return id, true
}

func callerID(c *gin.Context) pgtype.UUID {
	var id pgtype.UUID
	if p, ok := FromContext(c.Request.Context()); ok {
		_ = id.Scan(p.ID)
	}
	return id
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func presenceByAccount(rows []dbsqlc.ListServiceAccountPresenceRow) map[pgtype.UUID]dbsqlc.ListServiceAccountPresenceRow {
	out := make(map[pgtype.UUID]dbsqlc.ListServiceAccountPresenceRow, len(rows))
	for _, r := range rows {
		out[r.ID] = r
	}
	return out
}

func newServiceAccountJSON(a dbsqlc.ServiceAccount, seen dbsqlc.ListServiceAccountPresenceRow) serviceAccountJSON {
	return serviceAccountJSON{
		ID:          a.ID.String(),
		Name:        a.Name,
		Description: a.Description,
		CreatedBy:   uuidString(a.CreatedBy),
		CreatedAt:   formatTime(a.CreatedAt.Time),
		Role:        Role(a.Role),
		Provisioned: a.Provisioned,
		TakesRuns:   a.TakesRuns,
		Presence:    string(presence.Of(seen.WorkerSeenAt.Time, seen.WorkerSeenAt.Valid, seen.WorkerWorking, seen.CheckedAt.Time)),
		LastSeenAt:  optionalTime(seen.WorkerSeenAt),
	}
}

func newKeyJSON(k dbsqlc.CreateServiceAccountKeyRow, now time.Time) keyJSON {
	return keyJSON{
		ID:               k.ID.String(),
		ServiceAccountID: k.ServiceAccountID.String(),
		Name:             k.Name,
		DisplayPrefix:    k.DisplayPrefix,
		CreatedBy:        uuidString(k.CreatedBy),
		CreatedAt:        formatTime(k.CreatedAt.Time),
		ExpiresAt:        optionalTime(k.ExpiresAt),
		LastUsedAt:       optionalTime(k.LastUsedAt),
		RevokedAt:        optionalTime(k.RevokedAt),
		State:            keyState(k.RevokedAt, k.ExpiresAt, now),
	}
}

func uuidString(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	s := id.String()
	return &s
}

func optionalTime(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := formatTime(t.Time)
	return &s
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
