package users

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

func (h *Handler) deactivate(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	var done auth.Deactivation
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		if done, err = auth.Deactivate(ctx, q, caller, id); err != nil {
			return err
		}
		return recordDeactivated(ctx, q, caller, id, done)
	})
	switch {
	case errors.Is(err, auth.ErrNoUser):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return
	case errors.Is(err, auth.ErrDeactivateSelf):
		h.refuse(c, id, "cannot_deactivate_self", "you cannot deactivate yourself; ask another Admin to do it")
		return
	case errors.Is(err, auth.ErrLastAdmin):
		h.refuse(c, id, "last_admin", "this is the last active Admin; make someone else an Admin first")
		return
	case err != nil:
		h.fail(c, "deactivate a person", err, slog.String("user_id", id.String()))
		return
	}
	person, ok := h.person(c, id)
	if !ok {
		return
	}
	h.logger.InfoContext(ctx, "deactivated a person", slog.String("actor_id", caller.ID), slog.String("user_id", id.String()), slog.Int64("sessions", done.Sessions), slog.Int64("grants", done.Grants))
	c.JSON(http.StatusOK, person)
}

func (h *Handler) reactivate(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		done, err := auth.Reactivate(ctx, q, id)
		if err != nil {
			return err
		}
		return recordReactivated(ctx, q, caller, id, done)
	})
	switch {
	case errors.Is(err, auth.ErrNoUser):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return
	case err != nil:
		h.fail(c, "reactivate a person", err, slog.String("user_id", id.String()))
		return
	}
	person, ok := h.person(c, id)
	if !ok {
		return
	}
	h.logger.InfoContext(ctx, "reactivated a person", slog.String("actor_id", caller.ID), slog.String("user_id", id.String()))
	c.JSON(http.StatusOK, person)
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpapi.UUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller := httpapi.Caller(c)
	active := false
	err := h.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		name, err := q.RemoveDeactivatedUser(ctx, id)
		if err == nil {
			return recordRemoved(ctx, q, caller, id, name)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		exists, err := q.UserExists(ctx, id)
		if err != nil {
			return err
		}
		if !exists {
			return auth.ErrNoUser
		}
		active = true
		return nil
	})
	switch {
	case errors.Is(err, auth.ErrNoUser):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return
	case err != nil:
		h.fail(c, "remove a person", err, slog.String("user_id", id.String()))
		return
	case active:
		h.refuse(c, id, "not_deactivated", "deactivate this person first; only a deactivated person can be removed")
		return
	}
	h.logger.InfoContext(ctx, "removed a person", slog.String("actor_id", caller.ID), slog.String("user_id", id.String()))
	c.Status(http.StatusNoContent)
}
