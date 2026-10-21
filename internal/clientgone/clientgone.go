package clientgone

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

const Status = 499

func Is(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) && ctx.Err() != nil
}

func Abort(c *gin.Context, logger *slog.Logger, action string) {
	logger.DebugContext(c.Request.Context(), action+": client went away")
	c.AbortWithStatus(Status)
}

func Handle(c *gin.Context, logger *slog.Logger, action string, err error) bool {
	if !Is(c.Request.Context(), err) {
		return false
	}
	Abort(c, logger, action)
	return true
}

func WriteHeader(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string) {
	logger.DebugContext(r.Context(), action+": client went away")
	w.WriteHeader(Status)
	_, _ = w.Write(nil)
}
