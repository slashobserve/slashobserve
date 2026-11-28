package users

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const maxInviteBodyBytes = 1 << 10

type inviteJSON struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      auth.Role `json:"role"`
	InvitedBy string    `json:"invited_by"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	SignInURL string    `json:"sign_in_url"`
	Member    bool      `json:"member"`
}

type inviteListJSON struct {
	Invites    []inviteJSON `json:"invites"`
	NextCursor *string      `json:"next_cursor"`
}

func (h *Handler) registerInvites(api auth.Routes) {
	g := api.Group("/invites")
	g.GET("", auth.RoleAdmin, h.listInvites)
	g.POST("", auth.RoleAdmin, h.createInvite)
	g.DELETE("/:id", auth.RoleAdmin, h.deleteInvite)
}

func (h *Handler) newInviteJSON(id pgtype.UUID, email, role, by string, created, updated pgtype.Timestamptz, member bool) inviteJSON {
	return inviteJSON{
		ID:        id.String(),
		Email:     email,
		Role:      auth.Role(role),
		InvitedBy: by,
		CreatedAt: httpapi.FormatTime(created.Time),
		UpdatedAt: httpapi.FormatTime(updated.Time),
		SignInURL: h.auth.SignInURL(),
		Member:    member,
	}
}

func (h *Handler) listInvites(c *gin.Context) {
	limit, ok := httpapi.LimitParam(c, defaultLimit, maxLimit)
	if !ok {
		return
	}
	after, ok := httpapi.DecodeTimeCursor(c.Query("cursor"))
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "invalid cursor")
		return
	}
	params := dbsqlc.ListInvitesParams{RowLimit: httpapi.Int32(limit + 1)}
	if after != nil {
		params.CursorCreatedAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.CursorID = after.ID
	}
	rows, err := h.db.Queries().ListInvites(c.Request.Context(), params)
	if err != nil {
		h.fail(c, "list invites", err)
		return
	}
	body := inviteListJSON{Invites: []inviteJSON{}}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next := httpapi.EncodeTimeCursor(last.CreatedAt.Time, last.ID.String())
		body.NextCursor = &next
	}
	for _, r := range rows {
		body.Invites = append(body.Invites, h.newInviteJSON(r.ID, r.Email, r.Role, r.CreatedByName, r.CreatedAt, r.UpdatedAt, r.Member))
	}
	c.JSON(http.StatusOK, body)
}

func (h *Handler) createInvite(c *gin.Context) {
	var req struct {
		Email string  `json:"email"`
		Role  *string `json:"role"`
	}
	if !httpapi.DecodeBody(c, &req, maxInviteBodyBytes) {
		return
	}
	email := strings.TrimSpace(req.Email)
	if problem := auth.InviteEmailProblem(email); problem != "" {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", problem)
		return
	}
	if req.Role == nil {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", auth.InvalidRoleMessage)
		return
	}
	role, ok := auth.ParseRole(*req.Role)
	if !ok {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", auth.InvalidRoleMessage)
		return
	}
	if !h.auth.EmailMaySignIn(email) {
		httpapi.AbortError(c, http.StatusUnprocessableEntity, "email_domain_not_allowed", "people with this email cannot sign in; add its domain to SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS first")
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	callerID, err := db.ParseUUID(caller.ID)
	if err != nil {
		h.fail(c, "read caller id", err)
		return
	}
	var row dbsqlc.UpsertInviteRow
	member := false
	err = h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		if member, err = q.EmailHasSignedIn(ctx, email); err != nil || member {
			return err
		}
		previous, err := q.LockInviteRole(ctx, email)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err = q.UpsertInvite(ctx, dbsqlc.UpsertInviteParams{
			Email: email, Role: string(role), CreatedByKind: string(caller.Kind), CreatedByID: callerID, CreatedByName: caller.Name,
		})
		if err != nil {
			return err
		}
		return recordInvite(ctx, q, caller, row, auth.Role(previous))
	})
	if err != nil {
		h.fail(c, "create invite", err)
		return
	}
	if member {
		httpapi.AbortError(c, http.StatusConflict, "already_member", "someone with this email has already signed in; change their role on the People page")
		return
	}
	status := http.StatusOK
	if row.Inserted {
		status = http.StatusCreated
	}
	h.logger.InfoContext(ctx, "invite saved", slog.String("actor_id", caller.ID), slog.String("invite_id", row.ID.String()), slog.String("role", row.Role), slog.Bool("created", row.Inserted))
	c.JSON(status, h.newInviteJSON(row.ID, row.Email, row.Role, row.CreatedByName, row.CreatedAt, row.UpdatedAt, false))
}

func (h *Handler) deleteInvite(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		email, err := q.DeleteInvite(ctx, id)
		if err != nil {
			return err
		}
		return recordInviteWithdrawn(ctx, q, caller, id, email)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "invite not found; it was withdrawn or the person has signed in")
		return
	}
	if err != nil {
		h.fail(c, "delete invite", err, slog.String("invite_id", id.String()))
		return
	}
	h.logger.InfoContext(ctx, "invite withdrawn", slog.String("actor_id", caller.ID), slog.String("invite_id", id.String()))
	c.Status(http.StatusNoContent)
}
