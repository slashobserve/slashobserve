package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/origin"
)

type Check struct {
	Name string
	Run  func(context.Context) error
}

type Auth interface {
	Mount(*gin.RouterGroup) auth.Routes
}

type Registrar interface {
	Register(auth.Routes)
}

type Mount struct {
	Path    string
	Handler http.Handler
}

type RequestMetrics interface {
	ObserveRequest(route, method string, status int, took time.Duration)
}

type DatasourceUsage interface {
	DatasourceUsage(ctx context.Context, by auth.Principal) (map[string]int, error)
}

type Options struct {
	Logger          *slog.Logger
	Readiness       []Check
	Auth            Auth
	Datasources     *datasource.Registry
	DatasourceUsage DatasourceUsage
	API             []Registrar
	Mounts          []Mount
	Metrics         RequestMetrics

	ContractCheck bool
}

func New(opts Options) (http.Handler, error) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.ForwardedByClientIP = false
	if opts.Metrics != nil {
		engine.Use(observeRequests(opts.Metrics))
	}
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
	authenticated.Use(origin.Middleware)
	registerDatasourceRoutes(authenticated, opts.Datasources, opts.DatasourceUsage, opts.Logger)
	for _, r := range opts.API {
		r.Register(authenticated)
	}
	for _, m := range opts.Mounts {
		engine.Any(m.Path, gin.WrapH(m.Handler))
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
			slog.String("panic_type", fmt.Sprintf("%T", recovered)))
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

func observeRequests(m RequestMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		m.ObserveRequest(c.FullPath(), c.Request.Method, c.Writer.Status(), time.Since(start))
	}
}

func logRequests(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		level := slog.LevelInfo
		if p := c.Request.URL.Path; p == "/healthz" || p == "/readyz" || p == "/metrics" {
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

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; media-src 'none'; object-src 'none'; frame-src 'none'; worker-src 'self'; base-uri 'none'; frame-ancestors 'none'"

func securityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	c.Header("Content-Security-Policy", contentSecurityPolicy)
	c.Header("X-Frame-Options", "DENY")
	c.Next()
}
