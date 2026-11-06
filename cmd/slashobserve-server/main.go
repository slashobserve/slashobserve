package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actions"
	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/assets"
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
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/providers"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/metrics"
	"github.com/slashobserve/slashobserve/internal/news"
	"github.com/slashobserve/slashobserve/internal/prompts"
	dashprovision "github.com/slashobserve/slashobserve/internal/provision"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/queryproxy/proxyhttp"
	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/scheduler"
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

func versionLine() string { return version + " (" + commit + ")" }

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(os.LookupEnv))
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(versionLine())
		return
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
	var upgradeErr *db.SpecUpgradeError
	if errors.As(err, &upgradeErr) {
		exit.attrs = append(exit.attrs, slog.String("dashboard_id", upgradeErr.DashboardID), slog.Int("dashboard_version", upgradeErr.Version))
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
		Version:             version,
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

	serverMetrics := metrics.New()

	llmProvider, err := providers.New(cfg.LLM, logger)
	if err != nil {
		return described("llm_provider_failed", err)
	}
	if llmProvider != nil {
		llmProvider = llm.WithMetrics(llmProvider, serverMetrics.ModelCalls())
		logger.Info("llm provider configured", slog.String("provider", llmProvider.Name()), slog.String("model", llmProvider.Model()))
	} else {
		logger.Info("no llm provider configured; the agent is off")
	}
	warnOptInLogging(logger, cfg)

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
	serverMetrics.Pool(database.Stat)

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

	runner := queryproxy.NewRunner(registry, logger, queryproxy.WithMetrics(serverMetrics.Proxy()))
	dashboardsService := dashboards.New(database, runner, folderAccess, logger)
	proxyRunner := queryproxy.NewRunner(registry, logger, queryproxy.WithValueCache(), queryproxy.WithMetrics(serverMetrics.Proxy()))
	proxyHandler := proxyhttp.NewHandler(proxyRunner, dashboardsService, dashboardsService, folderAccess, registry, logger)

	importsService := imports.New(database, dashboardsService, folderAccess, registry, logger)
	annotationsService := annotations.New(database, folderAccess)
	annotationsHandler := annotations.NewHandler(annotationsService, logger)
	findingsService := findings.New(database, folderAccess, annotationsService, dashboardsService, logger)
	findingURL := func(id string) string { return links.Finding(cfg.PublicURL, id) }
	toolComments := dashboards.NewComments(database, dashboardsService, nil)
	toolRegistry := tools.New(tools.Deps{
		Datasources: registry,
		Runner:      runner,
		Dashboards:  dashboardsService,
		Comments:    toolComments,
		Imports:     importsService,
		Folders:     folderAccess,
		Findings:    findingsService,
		PublicURL:   cfg.PublicURL,
		Logger:      logger,
	})

	var reviser revisions.Reviser
	observeOpts := revisions.ObserveOptions{
		DashboardURL: func(id, title string) string { return links.Dashboard(cfg.PublicURL, id, title) },
		FindingURL:   findingURL,
		Finding:      findingOfRequest(findingsService, findingURL),
	}
	if llmProvider != nil {
		reviser = agent.NewReviser(agent.New(agent.Config{
			Provider:    llmProvider,
			Tools:       toolRegistry,
			Dashboards:  dashboardsService,
			Builds:      dashboardsService,
			Datasources: registry,
			Imports:     importsService,
			Logger:      logger,

			SessionRenewBytes: cfg.Agent.SessionRenewBytes,
			LogToolInputs:     cfg.Agent.LogToolInputs,
		}), revisions.NewSteps(database, logger))
		reviser = agent.WithMetrics(reviser, serverMetrics.AgentRuns())
		observeOpts.Provider, observeOpts.Model = llmProvider.Name(), llmProvider.Model()
		observeOpts.Effort = reportedEffort(cfg.LLM)
	}
	revisionsService := revisions.NewService(database, reviser, logger, revisions.WithPanels(dashboardsService), revisions.WithSpecs(dashboardsService), revisions.WithBuilds(buildStore{dashboardsService}), revisions.WithPrincipals(authService), revisions.WithAccess(folderAccess))

	broker := events.NewBroker(database.Listen, logger)
	go broker.Run(ctx)

	stopWorkers := startWorkers(ctx, revisionsService, database.Listen, logger, revisions.WorkerOptions{RunTimeout: cfg.Agent.RunTimeout})
	defer stopWorkers()
	const healthJob = "dashboard_health"
	healthService := health.New(health.Deps{DB: database, Runner: runner, Access: folderAccess, Revisions: revisionsService, PublicURL: cfg.PublicURL, Logger: logger, Job: healthJob})
	jobs := scheduler.New(logger, database, serverMetrics.Scheduler(), instanceID())
	jobs.Register(healthJob, time.Hour, healthService.Run)
	stopScheduler := startScheduler(ctx, jobs, logger)
	defer stopScheduler()
	healthCtx, stopHealthRuns := context.WithCancel(ctx)
	healthHandler := health.NewHandler(healthCtx, healthService, jobs)
	defer func() {
		stopHealthRuns()
		healthHandler.Wait()
	}()

	provisioner := dashprovision.New(logger, dashboardsService, folderAccess, database, cfg.ProvisionDir)
	provisionCtx, stopProvisioning := context.WithCancel(ctx)
	provisionDone := make(chan struct{})
	go func() {
		defer close(provisionDone)
		provisioner.Watch(provisionCtx)
	}()
	defer func() {
		stopProvisioning()
		<-provisionDone
	}()

	outbound := allowlist.New(database, allowlist.Lock{Locked: cfg.OutboundDomainsLocked, Domains: cfg.OutboundDomains}, logger)
	egressClient := egress.New(outbound, egress.Options{UserAgent: "SlashObserve/" + version, Logger: logger})
	assetsClient := egress.New(outbound, egress.Options{UserAgent: "SlashObserve/" + version, Logger: logger, CacheTTL: time.Hour, CacheBytes: 64 << 20})

	grafanaCtx, stopGrafana := context.WithCancel(ctx)
	grafanaService := grafanaconn.New(grafanaCtx, grafanaconn.Deps{
		DB: database, Egress: egressClient, Domains: outbound, Imports: importsService, Access: folderAccess, SecretKey: cfg.SecretKey, Logger: logger,
	})
	defer grafanaService.Wait()
	defer stopGrafana()

	handler, err := server.New(server.Options{
		Logger:          logger,
		Auth:            authService,
		Datasources:     registry,
		DatasourceUsage: dashboardsService,
		API: []server.Registrar{
			dashboardsService,
			folders.NewHandler(database, folderAccess, logger),
			imports.NewHandler(importsService, folderAccess),
			grafanaconn.NewHandler(grafanaService),
			repairs.NewHandler(repairs.New(repairs.Deps{DB: database, Access: folderAccess, Revisions: revisionsService, PublicURL: cfg.PublicURL, Logger: logger})),
			stream.NewHandler(broker, dashboardsService, authService, folderAccess, folderAccess.RequireDashboard(folders.NeedView), logger),
			dashboards.NewComments(database, dashboardsService, revisionsService),
			revisionsService,
			revisions.NewObserveHandler(revisionsService, observeOpts),
			proxyHandler,
			findings.NewHandler(findingsService, findingURL, logger),
			proxyhttp.NewFindingData(proxyHandler, findingsService),
			users.NewHandler(database, authService, folderAccess, logger),
			outbound,
			prompts.NewHandler(database, logger),
			news.NewHandler(egressClient, logger),
			actions.NewHandler(dashboardsService, egressClient, folderAccess.RequireDashboard(folders.NeedEdit), logger),
			assets.NewHandler(assetsClient, logger),
			annotationsHandler,
			dashprovision.NewHandler(provisioner, logger),
			dashprovision.NewApplyHandler(dashprovision.NewApplier(database, dashboardsService, folderAccess), logger),
			healthHandler,
		},
		Mounts: append([]server.Mount{{
			Path:    "/mcp",
			Handler: mcp.NewHandler(toolRegistry, authService, mcp.Options{PublicURL: cfg.PublicURL, Version: version, Logger: logger}),
		}}, metricsMounts(cfg.Metrics, serverMetrics)...),
		Metrics:       serverMetrics.HTTP(),
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

func findingOfRequest(service *findings.Service, findingURL func(id string) string) func(context.Context, pgtype.UUID) (json.RawMessage, bool, error) {
	return func(ctx context.Context, requestID pgtype.UUID) (json.RawMessage, bool, error) {
		f, found, err := service.ForRequest(ctx, requestID)
		if err != nil || !found {
			return nil, found, err
		}
		raw, err := json.Marshal(findings.JSONOf(f, findingURL(f.ID)))
		return raw, err == nil, err
	}
}

type buildStore struct {
	service *dashboards.Service
}

func (b buildStore) ViewBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (revisions.BuildView, error) {
	build, err := b.service.ViewBuild(ctx, by, requestID)
	if err != nil {
		return revisions.BuildView{}, revisionsBuildError(err)
	}
	raw, err := json.Marshal(build.Spec)
	if err != nil {
		return revisions.BuildView{}, fmt.Errorf("marshal build spec: %w", err)
	}
	progress := build.Progress()
	view := revisions.BuildView{
		RequestID:     build.RequestID.String(),
		Saves:         build.Saves,
		PlannedPanels: progress.PlannedPanels,
		Panels:        progress.Panels,
		Spec:          raw,
		UpdatedAt:     httpapi.FormatTime(build.UpdatedAt),
	}
	if build.DashboardID.Valid {
		view.DashboardID, view.BaseVersion = build.DashboardID.String(), build.BaseVersion
	}
	return view, nil
}

func (b buildStore) SaveStoppedBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (revisions.SavedBuild, error) {
	dash, ver, replayed, err := b.service.SaveStoppedBuild(ctx, by, requestID)
	if err != nil {
		return revisions.SavedBuild{}, revisionsBuildError(err)
	}
	return revisions.SavedBuild{DashboardID: dash.ID, Version: ver.Number, Replayed: replayed}, nil
}

func revisionsBuildError(err error) error {
	var conflict *dashboards.ConflictError
	switch {
	case errors.As(err, &conflict):
		return &revisions.BuildConflictError{Latest: conflict.Latest}
	case errors.Is(err, dashboards.ErrBuildNotFound):
		return revisions.ErrBuildNotFound
	case errors.Is(err, dashboards.ErrBuildEmpty):
		return revisions.ErrBuildEmpty
	case errors.Is(err, dashboards.ErrBuildNotSavable):
		return revisions.ErrBuildNotSavable
	case errors.Is(err, dashboards.ErrNotFound):
		return revisions.ErrDashboardNotFound
	}
	return err
}

func metricsMounts(enabled bool, registry *metrics.Registry) []server.Mount {
	if !enabled {
		return nil
	}
	return []server.Mount{{Path: "/metrics", Handler: registry.Handler()}}
}

func warnOptInLogging(logger *slog.Logger, cfg config.Config) {
	if cfg.LLM.LogProviderErrors {
		logger.Warn("provider error messages are logged; a provider can echo prompt text in them")
	}
	if cfg.Agent.LogToolInputs {
		logger.Warn("agent tool inputs are logged by hash, size, panel ids and issue paths; meant for evals stacks")
	}
}

func reportedEffort(llm config.LLM) string {
	if llm.Provider == config.LLMProviderAnthropic {
		return cmp.Or(llm.Effort, "medium")
	}

	return ""
}

func startWorkers(ctx context.Context, service *revisions.Service, listen events.Listener, logger *slog.Logger, opts revisions.WorkerOptions) func() {
	if !service.Available() {
		return func() {}
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		revisions.NewWorker(service, listen, logger, opts).Run(workerCtx)
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

func startScheduler(ctx context.Context, jobs *scheduler.Scheduler, logger *slog.Logger) func() {
	schedulerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		jobs.Run(schedulerCtx)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			logger.Warn("scheduled jobs did not stop in time")
		}
	}
}

func instanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "server"
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	return host + "-" + hex.EncodeToString(suffix)
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
