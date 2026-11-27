package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/audit"
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

type PublicRegistrar interface {
	RegisterPublic(*gin.RouterGroup)
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
	Phase           func() Phase
	PublicURL       *url.URL
	Auth            Auth
	Datasources     *datasource.Registry
	DatasourceUsage DatasourceUsage
	API             []Registrar
	Public          []PublicRegistrar
	Mounts          []Mount
	Redirects       Redirects
	Metrics         RequestMetrics
	Audit           *audit.Recorder
	TrustedProxies  []netip.Prefix

	ContractCheck bool
}

func New(opts Options) (http.Handler, error) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	if err := trustProxies(engine, opts.TrustedProxies); err != nil {
		return nil, err
	}
	if opts.Metrics != nil {
		engine.Use(observeRequests(opts.Metrics))
	}
	engine.Use(logRequests(opts.Logger), recoverPanics(opts.Logger), securityHeaders(servedOverHTTPS(opts.PublicURL)))
	registerHealthRoutes(engine, opts.Logger, opts.Readiness, opts.Phase)
	recordRequests, err := auditMiddleware(opts.Audit)
	if err != nil {
		return nil, err
	}
	apiGroup := engine.Group(apiPrefix, recordRequests...)
	documented, err := documentedQueryParams()
	if err != nil {
		return nil, fmt.Errorf("read documented query parameters: %w", err)
	}
	apiGroup.Use(warnUnknownQuery(documented))
	var checked *contract
	if opts.ContractCheck {
		if checked, err = newContract(context.Background(), opts.Logger); err != nil {
			return nil, err
		}
		apiGroup.Use(checked.middleware)
		opts.Logger.Warn("api contract check enabled")
	}
	public := apiGroup.Group(publicPrefix, noStore)
	for _, r := range opts.Public {
		r.RegisterPublic(public)
	}
	authenticated := opts.Auth.Mount(apiGroup)
	authenticated.Use(origin.Middleware, principalAccess)
	registerDatasourceRoutes(authenticated, opts.Datasources, opts.DatasourceUsage, opts.Logger)
	for _, r := range opts.API {
		r.Register(authenticated)
	}
	for _, m := range opts.Mounts {
		engine.Any(m.Path, append(recordRequests, gin.WrapH(m.Handler))...)
	}
	registerWebRoutes(engine, opts.Redirects)
	if checked != nil {
		if err := checked.checkRoutes(engine); err != nil {
			return nil, err
		}
	}
	return compressReads(engine)
}

func principalAccess(c *gin.Context) {
	principal, _ := auth.FromContext(c.Request.Context())
	c.Request = c.Request.WithContext(datasource.WithAccess(c.Request.Context(), datasource.Access{By: principal}))
	c.Next()
}

func auditMiddleware(r *audit.Recorder) ([]gin.HandlerFunc, error) {
	if r == nil {
		return nil, nil
	}
	mw, err := r.Middleware()
	if err != nil {
		return nil, err
	}
	return []gin.HandlerFunc{mw}, nil
}

func recoverPanics(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			err, _ := recovered.(error)
			switch {
			case errors.Is(err, http.ErrAbortHandler):
				panic(recovered)
			case errors.Is(err, syscall.EPIPE), errors.Is(err, syscall.ECONNRESET):
				c.Abort()
				return
			}
			logger.ErrorContext(c.Request.Context(), "panic serving request",
				slog.String("method", c.Request.Method),
				slog.String("path", loggedPath(c.Request.URL.Path)),
				slog.String("panic_type", fmt.Sprintf("%T", recovered)))
			c.AbortWithStatus(http.StatusInternalServerError)
		}()
		c.Next()
	}
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
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", loggedPath(c.Request.URL.Path)),
			slog.Int("status", c.Writer.Status()),
			slog.Int("bytes", c.Writer.Size()),
			slog.Duration("duration", time.Since(start)),
		}
		logger.LogAttrs(c.Request.Context(), level, "http request", append(attrs, requestAttrs(c)...)...)
	}
}

func requestAttrs(c *gin.Context) []slog.Attr {
	scope, ok := audit.ScopeOf(c.Request.Context())
	if !ok {
		return []slog.Attr{slog.String("route", c.FullPath())}
	}
	attrs := []slog.Attr{slog.String("request_id", scope.RequestID.String()), slog.String("route", scope.Route)}
	if scope.Actor.ID.Valid {
		attrs = append(attrs, slog.String("principal", string(scope.Actor.Kind)+":"+scope.Actor.ID.UUID.String()))
	}
	return attrs
}

const publicPrefix = "/public"

var tokenPathPrefixes = []string{apiPrefix + publicPrefix + "/", "/p/", "/rca/"}

func loggedPath(urlPath string) string {
	for _, prefix := range tokenPathPrefixes {
		rest, ok := strings.CutPrefix(urlPath, prefix)
		if !ok || rest == "" {
			continue
		}
		if _, tail, more := strings.Cut(rest, "/"); more {
			return prefix + ":token/" + tail
		}
		return prefix + ":token"
	}
	return urlPath
}

func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Next()
}

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; media-src 'none'; object-src 'none'; frame-src 'none'; worker-src 'self'; base-uri 'none'; frame-ancestors 'none'"

const strictTransportSecurity = "max-age=31536000"

func servedOverHTTPS(publicURL *url.URL) bool {
	return publicURL != nil && publicURL.Scheme == "https"
}

func securityHeaders(overHTTPS bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", contentSecurityPolicy)
		c.Header("X-Frame-Options", "DENY")
		if overHTTPS {
			c.Header("Strict-Transport-Security", strictTransportSecurity)
		}
		c.Next()
	}
}
