package auth

import (
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

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxBodyBytes         = 64 << 10
	maxDescriptionLength = 256
	maxKeyNameLength     = 64
	maxKeyLifetime       = 366 * 24 * time.Hour
	displayPrefixLength  = 12
	uniqueViolation      = "23505"
)

var serviceAccountName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type serviceAccountJSON struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	CreatedBy   *string `json:"created_by"`
	CreatedAt   string  `json:"created_at"`
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
}

func (s *Service) mountServiceAccounts(authenticated *gin.RouterGroup) {
	accounts := authenticated.Group("/service-accounts", RequireUser)
	accounts.POST("", s.createServiceAccount)
	accounts.GET("", s.listServiceAccounts)
	accounts.POST("/:id/keys", s.createKey)
	accounts.GET("/:id/keys", s.listKeys)
	accounts.DELETE("/:id/keys/:keyId", s.revokeKey)
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
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeBody(c, &req) {
		return
	}
	if !serviceAccountName.MatchString(req.Name) {
		abortError(c, http.StatusBadRequest, "invalid_request", "name must match ^[a-z0-9][a-z0-9-]{0,62}$")
		return
	}
	if utf8.RuneCountInString(req.Description) > maxDescriptionLength || hasControlChars(req.Description) {
		abortError(c, http.StatusBadRequest, "invalid_request", "description must be at most 256 characters without control characters")
		return
	}
	account, err := s.queries.CreateServiceAccount(c.Request.Context(), dbsqlc.CreateServiceAccountParams{
		Name:        req.Name,
		Description: req.Description,
		CreatedBy:   callerID(c),
	})
	if isUniqueViolation(err) {
		abortError(c, http.StatusConflict, "conflict", "a service account with this name already exists")
		return
	}
	if err != nil {
		s.internalError(c, "create service account", err)
		return
	}
	c.JSON(http.StatusCreated, newServiceAccountJSON(account))
}

func (s *Service) listServiceAccounts(c *gin.Context) {
	accounts, err := s.queries.ListServiceAccounts(c.Request.Context())
	if err != nil {
		s.internalError(c, "list service accounts", err)
		return
	}
	items := make([]serviceAccountJSON, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, newServiceAccountJSON(a))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Service) createKey(c *gin.Context) {
	accountID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !decodeBody(c, &req) {
		return
	}
	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxKeyNameLength || hasControlChars(req.Name) {
		abortError(c, http.StatusBadRequest, "invalid_request", "name must be 1 to 64 characters without control characters")
		return
	}
	var expiresAt pgtype.Timestamptz
	if req.ExpiresAt != nil {
		now := time.Now()
		if !req.ExpiresAt.After(now) || req.ExpiresAt.After(now.Add(maxKeyLifetime)) {
			abortError(c, http.StatusBadRequest, "invalid_request", "expires_at must be in the future and within 366 days")
			return
		}
		expiresAt = pgtype.Timestamptz{Time: *req.ExpiresAt, Valid: true}
	}
	if !s.serviceAccountExists(c, accountID) {
		return
	}
	token, hash, err := newToken(keyPrefix)
	if err != nil {
		s.internalError(c, "generate key", err)
		return
	}
	row, err := s.queries.CreateServiceAccountKey(c.Request.Context(), dbsqlc.CreateServiceAccountKeyParams{
		ServiceAccountID: accountID,
		Name:             req.Name,
		DisplayPrefix:    token[:displayPrefixLength],
		TokenHash:        hash,
		CreatedBy:        callerID(c),
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		s.internalError(c, "create key", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": newKeyJSON(row), "token": token})
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
	items := make([]keyJSON, 0, len(rows))
	for _, r := range rows {
		items = append(items, newKeyJSON(dbsqlc.CreateServiceAccountKeyRow(r)))
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
	n, err := s.queries.RevokeServiceAccountKey(c.Request.Context(), dbsqlc.RevokeServiceAccountKeyParams{
		ID:               keyID,
		ServiceAccountID: accountID,
	})
	if err != nil {
		s.internalError(c, "revoke key", err)
		return
	}
	if n == 0 {
		abortError(c, http.StatusNotFound, "not_found", "key not found")
		return
	}
	c.Status(http.StatusNoContent)
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

func newServiceAccountJSON(a dbsqlc.ServiceAccount) serviceAccountJSON {
	return serviceAccountJSON{
		ID:          a.ID.String(),
		Name:        a.Name,
		Description: a.Description,
		CreatedBy:   uuidString(a.CreatedBy),
		CreatedAt:   formatTime(a.CreatedAt.Time),
	}
}

func newKeyJSON(k dbsqlc.CreateServiceAccountKeyRow) keyJSON {
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
