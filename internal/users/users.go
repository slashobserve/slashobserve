package users

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	defaultLimit = 100
	maxLimit     = 500

	maxRoleBodyBytes = 1 << 10
)

type personJSON struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Email          string             `json:"email"`
	CreatedAt      string             `json:"created_at"`
	LastLoginAt    string             `json:"last_login_at"`
	LastSeenAt     string             `json:"last_seen_at"`
	DashboardCount int                `json:"dashboard_count"`
	KeyCount       int                `json:"key_count"`
	SessionCount   int                `json:"session_count"`
	Role           string             `json:"role"`
	RoleSource     string             `json:"role_source"`
	RoleGroup      *string            `json:"role_group"`
	RoleSetBy      *string            `json:"role_set_by"`
	RoleLocked     bool               `json:"role_locked"`
	DeactivatedAt  *string            `json:"deactivated_at"`
	DeactivatedBy  *string            `json:"deactivated_by"`
	PersonalItems  *personalItemsJSON `json:"personal_items"`
	State          State              `json:"state"`
	AllowedActions []Action           `json:"allowed_actions"`
}

type refusalJSON struct {
	Error          string   `json:"error"`
	Code           string   `json:"code"`
	State          State    `json:"state"`
	AllowedActions []Action `json:"allowed_actions"`
}

type personalItemsJSON struct {
	Dashboards    int `json:"dashboards"`
	Observations  int `json:"observations"`
	SavedSearches int `json:"saved_searches"`
}

type listJSON struct {
	Users      []personJSON `json:"users"`
	NextCursor *string      `json:"next_cursor"`
}

type Handler struct {
	db     *db.DB
	auth   *auth.Service
	access *folders.Access
	logger *slog.Logger
}

func NewHandler(database *db.DB, authService *auth.Service, access *folders.Access, logger *slog.Logger) *Handler {
	return &Handler{db: database, auth: authService, access: access, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	g := api.Group("/users")
	g.GET("", auth.RoleAdmin, h.list)
	g.PATCH("/:id", auth.RoleAdmin, auth.RequireUser, h.update)
	g.DELETE("/:id/sessions", auth.RoleAdmin, auth.RequireUser, h.signOut)
	g.POST("/:id/deactivate", auth.RoleAdmin, auth.RequireUser, h.deactivate)
	g.POST("/:id/reactivate", auth.RoleAdmin, auth.RequireUser, h.reactivate)
	g.DELETE("/:id", auth.RoleAdmin, auth.RequireUser, h.remove)
	h.registerInvites(api)
	h.registerTeams(api)
}

func (h *Handler) list(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	after, ok := httpapi.DecodeTimeCursor(c.Query("cursor"))
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "invalid cursor")
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	levels, err := h.access.Levels(ctx, caller)
	if err != nil {
		h.fail(c, "list folder levels", err)
		return
	}
	params := dbsqlc.ListUsersParams{RowLimit: httpapi.Int32(limit + 1), Folders: levels.Visible()}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := h.db.Queries().ListUsers(ctx, params)
	if err != nil {
		if clientgone.Handle(c, h.logger, "list users", err) {
			return
		}
		h.logger.ErrorContext(ctx, "list users", logattrs.Failure("internal", err, slog.String("actor_id", caller.ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	body := listJSON{Users: []personJSON{}}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next := httpapi.EncodeTimeCursor(last.CreatedAt.Time, last.ID.String())
		body.NextCursor = &next
	}
	for _, r := range rows {
		body.Users = append(body.Users, newPersonJSON(r, caller))
	}
	c.JSON(http.StatusOK, body)
}

func newPersonJSON(r dbsqlc.ListUsersRow, caller auth.Principal) personJSON {
	source := auth.RoleSource(r.RoleSource)
	f := factsOf(caller, r.ID, source, r.DeactivatedAt)
	p := personJSON{
		State:          f.state,
		AllowedActions: allowedActions(f),
		ID:             r.ID.String(),
		Name:           r.Name,
		Email:          r.Email,
		CreatedAt:      httpapi.FormatTime(r.CreatedAt.Time),
		LastLoginAt:    httpapi.FormatTime(r.LastLoginAt.Time),
		LastSeenAt:     httpapi.FormatTime(r.LastSeenAt.Time),
		DashboardCount: int(r.DashboardCount),
		KeyCount:       int(r.KeyCount),
		SessionCount:   int(r.SessionCount),
		Role:           r.Role,
		RoleSource:     r.RoleSource,
		RoleLocked:     source.Locked(),
	}
	if r.RoleGroup.Valid {
		p.RoleGroup = &r.RoleGroup.String
	}
	if source == auth.SourceManual && r.ManualRoleByName.Valid {
		p.RoleSetBy = &r.ManualRoleByName.String
	}
	if r.DeactivatedAt.Valid {
		at := httpapi.FormatTime(r.DeactivatedAt.Time)
		p.DeactivatedAt = &at
		if r.DeactivatedByName.Valid {
			p.DeactivatedBy = &r.DeactivatedByName.String
		}
		p.PersonalItems = &personalItemsJSON{
			Dashboards:    int(r.PersonalDashboardCount),
			Observations:  int(r.PersonalObservationCount),
			SavedSearches: int(r.PersonalSavedSearchCount),
		}
	}
	return p
}

func parseRolePatch(raw json.RawMessage) (*auth.Role, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	if string(raw) == "null" {
		return nil, true
	}
	var name string
	if err := json.Unmarshal(raw, &name); err != nil {
		return nil, false
	}
	role, ok := auth.ParseRole(name)
	if !ok {
		return nil, false
	}
	return &role, true
}

func lockedMessage(source auth.RoleSource) string {
	origin := "an identity provider group"
	switch source {
	case auth.SourceAdminEmails:
		origin = "SLASHOBSERVE_ADMIN_EMAILS"
	case auth.SourceFirstUser:
		origin = "being the first person to sign in"
	}
	return "this role comes from " + origin + " and cannot be changed here"
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Role json.RawMessage `json:"role"`
	}
	if !httpapi.DecodeBody(c, &req, maxRoleBodyBytes) {
		return
	}
	role, ok := parseRolePatch(req.Role)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", auth.InvalidRoleMessage+" or null")
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		return h.auth.SetManualRole(ctx, q, caller, id, role)
	})
	var locked *auth.RoleLockedError
	switch {
	case errors.Is(err, auth.ErrNoUser):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return
	case errors.As(err, &locked):
		h.refuse(c, id, "role_locked", lockedMessage(locked.Source))
		return
	case errors.Is(err, auth.ErrLastAdmin):
		h.refuse(c, id, "last_admin", "at least one admin must remain")
		return
	case err != nil:
		h.fail(c, "set manual role", err, slog.String("user_id", id.String()))
		return
	}
	person, ok := h.person(c, id)
	if !ok {
		return
	}
	h.logger.InfoContext(ctx, "role changed", slog.String("actor_id", caller.ID), slog.String("target_kind", "user"), slog.String("target_id", id.String()), slog.String("role", person.Role))
	c.JSON(http.StatusOK, person)
}

func (h *Handler) person(c *gin.Context, id pgtype.UUID) (personJSON, bool) {
	ctx := c.Request.Context()
	levels, err := h.access.Levels(ctx, httpapi.Caller(c))
	if err != nil {
		h.fail(c, "list folder levels", err)
		return personJSON{}, false
	}
	row, err := h.db.Queries().GetPerson(ctx, dbsqlc.GetPersonParams{ID: id, Folders: levels.Visible()})
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return personJSON{}, false
	}
	if err != nil {
		h.fail(c, "get person", err, slog.String("user_id", id.String()))
		return personJSON{}, false
	}
	return newPersonJSON(dbsqlc.ListUsersRow(row), httpapi.Caller(c)), true
}

func (h *Handler) refuse(c *gin.Context, id pgtype.UUID, code, message string) {
	person, ok := h.person(c, id)
	if !ok {
		return
	}
	c.AbortWithStatusJSON(http.StatusConflict, refusalJSON{Error: message, Code: code, State: person.State, AllowedActions: person.AllowedActions})
}

func (h *Handler) fail(c *gin.Context, action string, err error, ids ...any) {
	if clientgone.Handle(c, h.logger, action, err) {
		return
	}
	attrs := append(logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID)), ids...)
	h.logger.ErrorContext(c.Request.Context(), action, attrs...)
	httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
}

type signedOutData struct {
	Name     string `json:"name"`
	Sessions int64  `json:"sessions"`
	Grants   int64  `json:"grants"`
}

func (h *Handler) signOut(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	fail := func(action string, err error) {
		if clientgone.Handle(c, h.logger, action, err) {
			return
		}
		h.logger.ErrorContext(ctx, action, logattrs.Failure("internal", err, slog.String("actor_id", caller.ID), slog.String("user_id", id.String()))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
	person, err := h.db.Queries().GetUserPrincipal(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		fail("read person", err)
		return
	}
	by, err := db.ParseUUID(caller.ID)
	if err != nil {
		fail("read caller id", err)
		return
	}
	var sessions, grants int64
	err = h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		if sessions, err = q.DeleteUserSessions(ctx, id); err != nil {
			return err
		}
		if grants, err = q.RevokeUserOAuthGrants(ctx, dbsqlc.RevokeUserOAuthGrantsParams{By: by, UserID: id}); err != nil {
			return err
		}
		if _, err = q.DeleteUnusedUserOAuthCodes(ctx, id); err != nil {
			return err
		}
		return activity.AppendAdmin(ctx, q, activity.By(ctx, caller), activity.TypeUserSignedOutEverywhere, id, signedOutData{Name: person.Name, Sessions: sessions, Grants: grants})
	})
	if err != nil {
		fail("sign a person out everywhere", err)
		return
	}
	h.logger.InfoContext(ctx, "signed a person out everywhere", slog.String("actor_id", caller.ID), slog.String("user_id", id.String()), slog.Int64("sessions", sessions), slog.Int64("grants", grants))
	if id.String() == caller.ID {
		h.auth.ClearSessionCookie(c)
	}
	c.Status(http.StatusNoContent)
}
