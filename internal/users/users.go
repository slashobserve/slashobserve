package users

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	defaultLimit = 100
	maxLimit     = 500
)

type personJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	CreatedAt      string `json:"created_at"`
	LastLoginAt    string `json:"last_login_at"`
	LastSeenAt     string `json:"last_seen_at"`
	DashboardCount int    `json:"dashboard_count"`
	KeyCount       int    `json:"key_count"`
	SessionCount   int    `json:"session_count"`
}

type listJSON struct {
	Users      []personJSON `json:"users"`
	NextCursor *string      `json:"next_cursor"`
}

type Handler struct {
	db     *db.DB
	auth   *auth.Service
	logger *slog.Logger
}

func NewHandler(database *db.DB, authService *auth.Service, logger *slog.Logger) *Handler {
	return &Handler{db: database, auth: authService, logger: logger}
}

func (h *Handler) Register(api *gin.RouterGroup) {
	g := api.Group("/users", auth.RequireUser)
	g.GET("", h.list)
	g.DELETE("/:id/sessions", h.signOut)
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
	params := dbsqlc.ListUsersParams{RowLimit: httpapi.Int32(limit + 1)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	ctx := c.Request.Context()
	rows, err := h.db.Queries().ListUsers(ctx, params)
	if err != nil {
		h.logger.ErrorContext(ctx, "list users", logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
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
		body.Users = append(body.Users, personJSON{
			ID:             r.ID.String(),
			Name:           r.Name,
			Email:          r.Email,
			CreatedAt:      httpapi.FormatTime(r.CreatedAt.Time),
			LastLoginAt:    httpapi.FormatTime(r.LastLoginAt.Time),
			LastSeenAt:     httpapi.FormatTime(r.LastSeenAt.Time),
			DashboardCount: int(r.DashboardCount),
			KeyCount:       int(r.KeyCount),
			SessionCount:   int(r.SessionCount),
		})
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) signOut(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	fail := func(action string, err error) {
		h.logger.ErrorContext(ctx, action, logattrs.Failure("internal", err, slog.String("actor_id", caller.ID), slog.String("user_id", id.String()))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
	exists, err := h.db.Queries().UserExists(ctx, id)
	if err != nil {
		fail("check user exists", err)
		return
	}
	if !exists {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return
	}
	n, err := h.db.Queries().DeleteUserSessions(ctx, id)
	if err != nil {
		fail("delete user sessions", err)
		return
	}
	h.logger.InfoContext(ctx, "signed a person out everywhere", slog.String("actor_id", caller.ID), slog.String("user_id", id.String()), slog.Int64("sessions", n))
	if id.String() == caller.ID {
		h.auth.ClearSessionCookie(c)
	}
	c.Status(http.StatusNoContent)
}
