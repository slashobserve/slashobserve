package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource/provision"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/events/stream"
	"github.com/slashobserve/slashobserve/internal/llm/providers"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/queryproxy/proxyhttp"
	"github.com/slashobserve/slashobserve/internal/server"
	"github.com/slashobserve/slashobserve/internal/server/mcp"
)

var (
	version = "dev"
	commit  = "unknown"
)

const (
	connectTimeout    = 30 * time.Second
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 60 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 15 * time.Second
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.LookupEnv))
	}
	if err := run(); err != nil {
		slog.Error("server exiting", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)
	logger.Info("starting slashobserve-server", slog.String("version", version), slog.String("commit", commit))

	registry, err := provision.Build(logger, cfg.Datasources)
	if err != nil {
		return fmt.Errorf("configure datasources: %w", err)
	}
	defer func() { _ = registry.Close() }()
	if cfg.DatasourcesFile != "" && len(cfg.Datasources) == 0 {
		logger.Warn("datasources file lists no datasources", slog.String("path", cfg.DatasourcesFile))
	}
	logger.Info("datasources configured", slog.Int("count", len(cfg.Datasources)))

	llmProvider, err := providers.New(cfg.LLM, logger)
	if err != nil {
		return fmt.Errorf("configure llm provider: %w", err)
	}
	if llmProvider != nil {
		logger.Info("llm provider configured", slog.String("provider", llmProvider.Name()), slog.String("model", llmProvider.Model()))
	} else {
		logger.Info("no llm provider configured; the agent is off")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Restore default signal handling so a second signal force-exits during shutdown.
	context.AfterFunc(ctx, stop)

	connectCtx, cancelConnect := context.WithTimeout(ctx, connectTimeout)
	database, err := db.Open(connectCtx, cfg.DatabaseURL, logger)
	cancelConnect()
	if err != nil {
		return err
	}
	defer database.Close()

	if err := database.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	instance, err := database.Queries().GetInstance(ctx)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	logger.Info("database ready", slog.String("instance_id", instance.ID.String()))

	authService, err := auth.New(auth.Config{
		PublicURL:           cfg.PublicURL,
		IssuerURL:           cfg.OIDCIssuerURL,
		InternalURL:         cfg.OIDCInternalURL,
		ClientID:            cfg.OIDCClientID,
		ClientSecret:        cfg.OIDCClientSecret,
		SessionTTL:          cfg.SessionTTL,
		AllowedEmailDomains: cfg.AllowedEmailDomains,
		AllowAnyEmailDomain: cfg.AllowAnyEmailDomain,
	}, database.Queries(), logger)
	if err != nil {
		return fmt.Errorf("create auth service: %w", err)
	}
	logger.Info("oidc login configured", slog.String("oidc_issuer", cfg.OIDCIssuerURL))
	if cfg.AllowAnyEmailDomain {
		logger.Warn("all identity-provider accounts may sign in")
	}

	runner := queryproxy.NewRunner(registry, logger)
	dashboardsService := dashboards.New(database, runner, logger)
	proxyRunner := queryproxy.NewRunner(registry, logger, queryproxy.WithValueCache())
	proxyHandler := proxyhttp.NewHandler(proxyRunner, dashboardsService, registry, logger)

	toolRegistry := tools.New(tools.Deps{
		Datasources: registry,
		Runner:      runner,
		Dashboards:  dashboardsService,
		PublicURL:   cfg.PublicURL,
		Logger:      logger,
	})

	broker := events.NewBroker(database.Listen, logger)
	go broker.Run(ctx)

	handler, err := server.New(server.Options{
		Logger:      logger,
		Auth:        authService,
		Datasources: registry,
		API:         []server.Registrar{dashboardsService, stream.NewHandler(broker, dashboardsService, logger), dashboards.NewComments(database, dashboardsService), proxyHandler},
		Mounts: []server.Mount{{
			Path:    "/mcp",
			Handler: mcp.NewHandler(toolRegistry, authService, mcp.Options{PublicURL: cfg.PublicURL, Version: version, Logger: logger}),
		}},
		ContractCheck: cfg.APIContractCheck,
		Readiness: []server.Check{
			{Name: "database", Run: database.Ping},
			{Name: "migrations", Run: database.CheckMigrations},
		},
	})
	if err != nil {
		return fmt.Errorf("build http handler: %w", err)
	}
	return serve(ctx, logger, cfg.HTTPAddr, handler, broker.Close)
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == config.LogFormatText {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

func serve(ctx context.Context, logger *slog.Logger, addr string, handler http.Handler, onShutdown ...func()) error {
	// WriteTimeout is deliberately unset: a write deadline would cut SSE streams.
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	for _, f := range onShutdown {
		srv.RegisterOnShutdown(f)
	}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down", slog.Duration("timeout", shutdownTimeout))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
