package auth

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Service) deleteServiceAccount(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	if !s.changeableServiceAccount(c, id) {
		return
	}
	n, err := s.queries.DeleteServiceAccount(c.Request.Context(), id)
	if err != nil {
		s.internalError(c, "delete service account", err)
		return
	}
	if n == 0 {
		abortError(c, http.StatusNotFound, "not_found", "service account not found")
		return
	}
	s.logger.InfoContext(c.Request.Context(), "deleted a service account", slog.String("actor_id", callerID(c).String()), slog.String("service_account_id", id.String()))
	c.Status(http.StatusNoContent)
}
