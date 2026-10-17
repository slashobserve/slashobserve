package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Check struct {
	Name string
	Run  func(context.Context) error
}

type Options struct {
	Logger    *slog.Logger
	Readiness []Check
}

func New(opts Options) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.ForwardedByClientIP = false
	engine.Use(logRequests(opts.Logger), recoverPanics(opts.Logger), securityHeaders)
	registerHealthRoutes(engine, opts.Logger, opts.Readiness)
	registerWebRoutes(engine)
	return engine
}

func recoverPanics(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		logger.ErrorContext(c.Request.Context(), "panic serving request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Any("panic", recovered))
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

func logRequests(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		level := slog.LevelInfo
		if p := c.Request.URL.Path; p == "/healthz" || p == "/readyz" {
			level = slog.LevelDebug
		}
		logger.LogAttrs(c.Request.Context(), level, "http request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int("bytes", c.Writer.Size()),
			slog.Duration("duration", time.Since(start)))
	}
}

func securityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Header("Content-Security-Policy", "frame-ancestors 'none'")
	c.Header("X-Frame-Options", "DENY")
	c.Next()
}
