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

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource/provision"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/events/stream"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/llm/providers"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/news"
	"github.com/slashobserve/slashobserve/internal/prompts"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/queryproxy/proxyhttp"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/server"
	"github.com/slashobserve/slashobserve/internal/server/mcp"
	"github.com/slashobserve/slashobserve/internal/users"
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
		slog.Error("server exiting", exitAttrs(err)...)
		os.Exit(1)
	}
}

// exitError is a fatal startup or serving failure. A driver, library or DSN error can echo hostnames, user names or
// URLs, so its text is logged only when the stage says it is a message the server or the standard library wrote from
// the operator's own settings; every other cause is reduced to its type and SQLSTATE.
type exitError struct {
	code   string
	detail string
	attrs  []any
	cause  error
}

func (e *exitError) Error() string { return e.code }
func (e *exitError) Unwrap() error { return e.cause }

func withheld(code string, cause error) error {
	return &exitError{code: code, cause: cause}
}

func described(code string, cause error) error {
	return &exitError{code: code, detail: cause.Error(), cause: cause}
}

// The host and port come from the parsed setting, never from the error text, which also names the user and database.
func databaseExit(databaseURL string, err error) error {
	var parseErr *pgconn.ParseConfigError
	if errors.As(err, &parseErr) {
		return &exitError{code: "invalid_database_url", detail: "SLASHOBSERVE_DATABASE_URL is not a valid PostgreSQL connection string", cause: err}
	}
	exit := &exitError{code: "database_unavailable", cause: err}
	if cfg, parseErr := pgconn.ParseConfig(databaseURL); parseErr == nil {
		exit.attrs = []any{slog.String("db_host", cfg.Host), slog.Int("db_port", int(cfg.Port))}
	}
	return exit
}

func datasourcesExit(err error) error {
	exit := &exitError{code: "datasources_failed", cause: err}
	var buildErr *provision.BuildError
	if errors.As(err, &buildErr) {
		exit.attrs = []any{slog.String("datasource_uid", buildErr.UID)}
		exit.detail = buildErr.Message
	}
	return exit
}

func migrationExit(err error) error {
	exit := &exitError{code: "migration_failed", cause: err}
	if version, ok := db.FailedMigrationVersion(err); ok {
		exit.attrs = []any{slog.Int64("migration_version", version)}
	}
	return exit
}

func exitAttrs(err error) []any {
	var exit *exitError
	if !errors.As(err, &exit) {
		return logattrs.Failure("internal", err)
	}
	if exit.detail != "" {
		return append([]any{slog.String("error_code", exit.code), slog.String("detail", exit.detail)}, exit.attrs...)
	}
	return logattrs.Failure(exit.code, exit.cause, exit.attrs...)
}

func rolePolicyOf(cfg config.Config) (auth.RolePolicy, error) {
	policy := auth.RolePolicy{AdminEmails: cfg.AdminEmails}
	for _, entry := range cfg.OIDCGroupRoles {
		role, ok := auth.ParseRole(entry.Role)
		if !ok {
			return auth.RolePolicy{}, fmt.Errorf("SLASHOBSERVE_OIDC_GROUP_ROLES: unknown role %q", entry.Role)
		}
		policy.GroupRoles = append(policy.GroupRoles, auth.GroupRole{Group: entry.Group, Role: role})
	}
	return policy, nil
}

func newAuthService(ctx context.Context, cfg config.Config, queries *dbsqlc.Queries, logger *slog.Logger) (*auth.Service, error) {
	rolePolicy, err := rolePolicyOf(cfg)
	if err != nil {
		return nil, described("invalid_config", err)
	}
	service, err := auth.New(auth.Config{
		PublicURL:           cfg.PublicURL,
		IssuerURL:           cfg.OIDCIssuerURL,
		InternalURL:         cfg.OIDCInternalURL,
		ClientID:            cfg.OIDCClientID,
		ClientSecret:        cfg.OIDCClientSecret,
		SessionTTL:          cfg.SessionTTL,
		AllowedEmailDomains: cfg.AllowedEmailDomains,
		AllowAnyEmailDomain: cfg.AllowAnyEmailDomain,
		ProviderName:        cfg.AuthProviderName,
		RolePolicy:          rolePolicy,
	}, queries, logger)
	if err != nil {
		return nil, withheld("auth_setup_failed", err)
	}
	logger.Info("oidc login configured", slog.String("oidc_issuer", cfg.OIDCIssuerURL))
	if err := service.ReconcileRoles(ctx); err != nil {
		return nil, withheld("role_reconcile_failed", err)
	}
	if cfg.AllowAnyEmailDomain {
		logger.Warn("all identity-provider accounts may sign in")
	}
	return service, nil
}

func run() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return described("invalid_config", err)
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)
	logger.Info("starting slashobserve-server", slog.String("version", version), slog.String("commit", commit))

	registry, err := provision.Build(logger, cfg.Datasources)
	if err != nil {
		return datasourcesExit(err)
	}
	defer func() { _ = registry.Close() }()
	if cfg.DatasourcesFile != "" && len(cfg.Datasources) == 0 {
		logger.Warn("datasources file lists no datasources", slog.String("path", cfg.DatasourcesFile))
	}
	logger.Info("datasources configured", slog.Int("count", len(cfg.Datasources)))

	llmProvider, err := providers.New(cfg.LLM, logger)
	if err != nil {
		return described("llm_provider_failed", err)
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
		return databaseExit(cfg.DatabaseURL, err)
	}
	defer database.Close()

	if err := database.Migrate(ctx); err != nil {
		return migrationExit(err)
	}
	instance, err := database.Queries().GetInstance(ctx)
	if err != nil {
		return withheld("instance_lookup_failed", err)
	}
	logger.Info("database ready", slog.String("instance_id", instance.ID.String()))

	authService, err := newAuthService(ctx, cfg, database.Queries(), logger)
	if err != nil {
		return err
	}

	folderAccess := folders.NewAccess(database)

	runner := queryproxy.NewRunner(registry, logger)
	dashboardsService := dashboards.New(database, runner, folderAccess, logger)
	proxyRunner := queryproxy.NewRunner(registry, logger, queryproxy.WithValueCache())
	proxyHandler := proxyhttp.NewHandler(proxyRunner, dashboardsService, registry, logger)

	importsService := imports.New(database, dashboardsService, registry, logger)
	toolComments := dashboards.NewComments(database, dashboardsService, nil)
	toolRegistry := tools.New(tools.Deps{
		Datasources: registry,
		Runner:      runner,
		Dashboards:  dashboardsService,
		Comments:    toolComments,
		Imports:     importsService,
		PublicURL:   cfg.PublicURL,
		Logger:      logger,
	})

	var reviser revisions.Reviser
	observeOpts := revisions.ObserveOptions{
		DashboardURL: func(id string) string { return tools.DashboardURL(cfg.PublicURL, id) },
	}
	if llmProvider != nil {
		reviser = agent.NewReviser(agent.New(agent.Config{
			Provider:    llmProvider,
			Tools:       toolRegistry,
			Dashboards:  dashboardsService,
			Datasources: registry,
			Imports:     importsService,
			Logger:      logger,
		}), revisions.NewSteps(database, logger))
		observeOpts.Provider, observeOpts.Model = llmProvider.Name(), llmProvider.Model()
		observeOpts.Effort = reportedEffort(cfg.LLM)
	}
	revisionsService := revisions.NewService(database, reviser, logger, revisions.WithPanels(dashboardsService), revisions.WithSpecs(dashboardsService), revisions.WithPrincipals(authService))

	broker := events.NewBroker(database.Listen, logger)
	go broker.Run(ctx)

	stopWorkers := startWorkers(ctx, revisionsService, database.Listen, logger)
	defer stopWorkers()

	outbound := allowlist.New(database, allowlist.Lock{Locked: cfg.OutboundDomainsLocked, Domains: cfg.OutboundDomains}, logger)
	egressClient := egress.New(outbound, egress.Options{UserAgent: "SlashObserve/" + version, Logger: logger})

	handler, err := server.New(server.Options{
		Logger:          logger,
		Auth:            authService,
		Datasources:     registry,
		DatasourceUsage: dashboardsService,
		API: []server.Registrar{
			dashboardsService,
			folders.NewHandler(database, folderAccess, logger),
			imports.NewHandler(importsService),
			stream.NewHandler(broker, dashboardsService, authService, logger),
			dashboards.NewComments(database, dashboardsService, revisionsService),
			revisionsService,
			revisions.NewObserveHandler(revisionsService, observeOpts),
			proxyHandler,
			users.NewHandler(database, authService, logger),
			outbound,
			prompts.NewHandler(database, logger),
			news.NewHandler(egressClient, logger),
		},
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
		return described("http_setup_failed", err)
	}
	if err := serve(ctx, logger, cfg.HTTPAddr, handler, broker.Close); err != nil {
		return described("http_server_failed", err)
	}
	return nil
}

func reportedEffort(llm config.LLM) string {
	if llm.Provider == config.LLMProviderAnthropic {
		return llm.Effort
	}

	return ""
}

func startWorkers(ctx context.Context, service *revisions.Service, listen events.Listener, logger *slog.Logger) func() {
	if !service.Available() {
		return func() {}
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		revisions.NewWorker(service, listen, logger, revisions.WorkerOptions{}).Run(workerCtx)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			logger.Warn("agent workers did not stop in time")
		}
	}
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
