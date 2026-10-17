package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

type Check struct {
	Name string
	Run  func(context.Context) error
}

type Auth interface {
	Mount(*gin.RouterGroup) *gin.RouterGroup
}

type Registrar interface {
	Register(*gin.RouterGroup)
}

type Options struct {
	Logger      *slog.Logger
	Readiness   []Check
	Auth        Auth
	Datasources *datasource.Registry
	API         []Registrar

	ContractCheck bool
}

func New(opts Options) (http.Handler, error) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.ForwardedByClientIP = false
	engine.Use(logRequests(opts.Logger), recoverPanics(opts.Logger), securityHeaders)
	registerHealthRoutes(engine, opts.Logger, opts.Readiness)
	apiGroup := engine.Group(apiPrefix)
	var checked *contract
	if opts.ContractCheck {
		var err error
		if checked, err = newContract(context.Background(), opts.Logger); err != nil {
			return nil, err
		}
		apiGroup.Use(checked.middleware)
		opts.Logger.Warn("api contract check enabled")
	}
	authenticated := opts.Auth.Mount(apiGroup)
	registerDatasourceRoutes(authenticated, opts.Datasources)
	for _, r := range opts.API {
		r.Register(authenticated)
	}
	registerWebRoutes(engine)
	if checked != nil {
		if err := checked.checkRoutes(engine); err != nil {
			return nil, err
		}
	}
	return engine, nil
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
