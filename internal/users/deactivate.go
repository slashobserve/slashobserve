package users

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

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
		done, err = auth.Deactivate(ctx, q, caller, id)
		return err
	})
	switch {
	case errors.Is(err, auth.ErrNoUser):
		httpapi.AbortError(c, http.StatusNotFound, "not_found", "user not found")
		return
	case errors.Is(err, auth.ErrDeactivateSelf):
		httpapi.AbortError(c, http.StatusConflict, "cannot_deactivate_self", "you cannot deactivate yourself; ask another Admin to do it")
		return
	case errors.Is(err, auth.ErrLastAdmin):
		httpapi.AbortError(c, http.StatusConflict, "last_admin", "this is the last active Admin; make someone else an Admin first")
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
	err := auth.Reactivate(ctx, h.db.Queries(), id)
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
	h.logger.InfoContext(ctx, "reactivated a person", slog.String("actor_id", httpapi.Caller(c).ID), slog.String("user_id", id.String()))
	c.JSON(http.StatusOK, person)
}
