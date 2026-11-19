package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func (s *Service) deleteServiceAccount(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	caller, _ := FromContext(ctx)
	err := s.inTx(ctx, func(q *dbsqlc.Queries) error {
		account, err := q.GetServiceAccount(ctx, id)
		if err != nil {
			return err
		}
		if account.Provisioned {
			return errProvisioned
		}
		n, err := q.DeleteServiceAccount(ctx, id)
		if err == nil && n == 0 {
			return pgx.ErrNoRows
		}
		if err != nil {
			return err
		}
		return s.changes.ServiceAccountDeleted(ctx, q, caller, account)
	})
	switch {
	case errors.Is(err, errProvisioned):
		abortError(c, http.StatusConflict, "provisioned", provisionedMessage)
	case errors.Is(err, pgx.ErrNoRows):
		abortError(c, http.StatusNotFound, "not_found", "service account not found")
	case err != nil:
		s.internalError(c, "delete service account", err)
	default:
		s.logger.InfoContext(ctx, "deleted a service account", slog.String("actor_id", callerID(c).String()), slog.String("service_account_id", id.String()))
		c.Status(http.StatusNoContent)
	}
}
