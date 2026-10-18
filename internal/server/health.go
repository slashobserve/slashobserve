package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const readinessTimeout = 2 * time.Second

type healthResponse struct {
	Status string `json:"status"`
}

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func registerHealthRoutes(engine *gin.Engine, logger *slog.Logger, checks []Check) {
	healthz := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, healthResponse{Status: "ok"})
	}
	readyz := func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
		defer cancel()
		resp := readinessResponse{Status: "ok", Checks: make(map[string]string, len(checks))}
		status := http.StatusOK
		for _, check := range checks {
			if err := check.Run(ctx); err != nil {
				logger.WarnContext(ctx, "readiness check failed", logattrs.Failure("readiness_check_failed", err, slog.String("check", check.Name))...)
				resp.Checks[check.Name] = "error"
				resp.Status = "unavailable"
				status = http.StatusServiceUnavailable
				continue
			}
			resp.Checks[check.Name] = "ok"
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(status, resp)
	}
	engine.GET("/healthz", healthz)
	engine.HEAD("/healthz", healthz)
	engine.GET("/readyz", readyz)
	engine.HEAD("/readyz", readyz)
}
