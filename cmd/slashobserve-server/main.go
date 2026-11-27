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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/accessrequests"
	"github.com/slashobserve/slashobserve/internal/actions"
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/agent/triage"
	"github.com/slashobserve/slashobserve/internal/agentsview"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/assets"
	"github.com/slashobserve/slashobserve/internal/audit"
	auditexport "github.com/slashobserve/slashobserve/internal/audit/export"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/auditfolder"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
	"github.com/slashobserve/slashobserve/internal/datasource/provision"
	"github.com/slashobserve/slashobserve/internal/datasourceadmin"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/discoveryhttp"
	"github.com/slashobserve/slashobserve/internal/draftbudget"
	"github.com/slashobserve/slashobserve/internal/drafthttp"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/events/stream"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/fleet"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/homedash"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/inboundmcp"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/livetext"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/providers"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/mcpauth"
	"github.com/slashobserve/slashobserve/internal/metrics"
	"github.com/slashobserve/slashobserve/internal/news"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/placement"
	"github.com/slashobserve/slashobserve/internal/prompts"
	"github.com/slashobserve/slashobserve/internal/proposals"
	dashprovision "github.com/slashobserve/slashobserve/internal/provision"
	"github.com/slashobserve/slashobserve/internal/public"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/queryproxy/proxyhttp"
	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/runsession"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
	"github.com/slashobserve/slashobserve/internal/scheduler"
	"github.com/slashobserve/slashobserve/internal/sealed"
	"github.com/slashobserve/slashobserve/internal/search"
	"github.com/slashobserve/slashobserve/internal/server"
	"github.com/slashobserve/slashobserve/internal/server/mcp"
	"github.com/slashobserve/slashobserve/internal/stars"
	"github.com/slashobserve/slashobserve/internal/tailhttp"
	"github.com/slashobserve/slashobserve/internal/threads"
	"github.com/slashobserve/slashobserve/internal/users"
	"github.com/slashobserve/slashobserve/internal/viewers"
	"github.com/slashobserve/slashobserve/internal/webhooks"
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
	requestsCutAfter  = shutdownTimeout - 5*time.Second
	drainPeriod       = 5 * time.Second
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
func databaseExit(cfg config.Config, err error) error {
	direct := errors.Is(err, db.ErrDirectURL)
	var parseErr *pgconn.ParseConfigError
	switch {
	case errors.Is(err, db.ErrDirectURLMismatch):
		return &exitError{code: "database_direct_url_mismatch", detail: "SLASHOBSERVE_DATABASE_DIRECT_URL reaches a different database than SLASHOBSERVE_DATABASE_URL", cause: err}
	case direct && errors.As(err, &parseErr):
		return &exitError{code: "invalid_database_direct_url", detail: "SLASHOBSERVE_DATABASE_DIRECT_URL is not a valid PostgreSQL connection string", cause: err}
	case errors.As(err, &parseErr):
		return &exitError{code: "invalid_database_url", detail: "SLASHOBSERVE_DATABASE_URL is not a valid PostgreSQL connection string", cause: err}
	}
	databaseURL := cfg.DatabaseURL
	if direct {
		databaseURL = cfg.DatabaseDirectURL
	}
	exit := &exitError{code: "database_unavailable", cause: err}
	if connCfg, parseErr := pgconn.ParseConfig(databaseURL); parseErr == nil {
		exit.attrs = []any{slog.String("db_host", connCfg.Host), slog.Int("db_port", int(connCfg.Port))}
	}
	if direct {
		exit.attrs = append(exit.attrs, slog.String("connection", "direct"))
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

func fileAdminsOnly(sources []config.Datasource) []string {
	var uids []string
	for _, ds := range sources {
		if ds.Audience == datasource.AudienceAdmins {
			uids = append(uids, ds.UID)
		}
	}
	return uids
}

func warnExposedAppDatasources(ctx context.Context, q *dbsqlc.Queries, sources []config.Datasource, logger *slog.Logger) {
	exposed, err := placement.ExposedAppDatasources(ctx, q, fileAdminsOnly(sources))
	if err != nil {
		logger.Warn("admins only datasources not checked", logattrs.Failure("datasource_audience_check_failed", err)...)
		return
	}
	for _, e := range exposed {
		logger.Warn("admins only datasource read outside admins only folders",
			slog.String("datasource_uid", e.Datasource), slog.Int("shared_items", e.Shared), slog.Int("personal_items", e.Personal))
	}
}

func checkDatabase(ctx context.Context, q *dbsqlc.Queries, sources []config.Datasource, logger *slog.Logger) error {
	if err := checkFileAudiences(ctx, q, sources); err != nil {
		return err
	}
	warnExposedAppDatasources(ctx, q, sources, logger)
	instance, err := q.GetInstance(ctx)
	if err != nil {
		return withheld("instance_lookup_failed", err)
	}
	logger.Info("database ready", slog.String("instance_id", instance.ID.String()))
	return nil
}

func checkFileAudiences(ctx context.Context, q *dbsqlc.Queries, sources []config.Datasource) error {
	err := placement.CheckFileAudiences(ctx, q, fileAdminsOnly(sources))
	var misplaced *placement.MisplacedError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &misplaced):
		return &exitError{code: "datasource_audience_refused", detail: misplaced.Error(), cause: err,
			attrs: []any{slog.String("datasource_uid", misplaced.Datasource), slog.Int("count", misplaced.Shared+misplaced.Personal)}}
	}
	return withheld("datasource_audience_check_failed", err)
}

func serviceAccountsExit(err error) error {
	exit := &exitError{code: "service_accounts_failed", cause: err}
	var refused *auth.ProvisionRefusedError
	if errors.As(err, &refused) {
		exit.detail = refused.Error()
	}
	return exit
}

func mcpSourcesExit(err error) error {
	var invalid *inboundmcp.InvalidError
	switch {
	case errors.As(err, &invalid), errors.Is(err, inboundmcp.ErrUIDTaken), errors.Is(err, inboundmcp.ErrNoSecretKey), errors.Is(err, inboundmcp.ErrTooMany):
		return described("mcp_sources_failed", err)
	}
	return withheld("mcp_sources_failed", err)
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

func reconcileStartup(ctx context.Context, database *db.DB, authService *auth.Service, cfg config.Config) error {
	if err := authService.ReconcileRoles(ctx); err != nil {
		return withheld("role_reconcile_failed", err)
	}
	if err := database.WithSessionLock(ctx, "service_accounts", func(ctx context.Context) error {
		return authService.EnsureProvisioned(ctx, fileAccounts(cfg.ServiceAccounts))
	}); err != nil {
		return serviceAccountsExit(err)
	}
	if err := database.WithSessionLock(ctx, "mcp_sources", func(ctx context.Context) error {
		return inboundmcp.New(inboundmcp.Deps{DB: database, Key: sealed.Key(cfg.SecretKey.Reveal())}).Provision(ctx, cfg.MCPSources)
	}); err != nil {
		return mcpSourcesExit(err)
	}
	return nil
}

func approveListedTools(ctx context.Context, service *inboundmcp.Service, cfg config.Config, logger *slog.Logger) {
	if err := service.ApproveListed(ctx, cfg.MCPSources); err != nil && ctx.Err() == nil {
		logger.ErrorContext(ctx, "MCP source approvals from the provisioning file failed", slog.String("failure", err.Error()))
	}
}

func fileAccounts(accounts []config.ServiceAccount) []auth.FileAccount {
	out := make([]auth.FileAccount, len(accounts))
	for i, a := range accounts {
		out[i] = auth.FileAccount{Name: a.Name, Role: a.Role, Description: a.Description, TakesRuns: a.TakesRuns, Key: a.Key}
	}
	return out
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
		ClientSecret:        cfg.OIDCClientSecret.Reveal(),
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
	if cfg.BrowserSignIn {
		logger.Info("oidc login configured", slog.String("oidc_issuer", cfg.OIDCIssuerURL))
	} else {
		logger.Warn("browser sign-in is off; only service-account keys can authenticate")
	}
	if cfg.AllowAnyEmailDomain {
		logger.Warn("all identity-provider accounts may sign in")
	}
	return service, nil
}

func logDatasources(cfg config.Config, logger *slog.Logger) {
	if cfg.DatasourcesFile != "" && len(cfg.Datasources) == 0 {
		logger.Warn("datasources file lists no datasources", slog.String("path", cfg.DatasourcesFile))
	}
	logger.Info("datasources configured", slog.Int("count", len(cfg.Datasources)))
}

func sweepBeforeInvestigations(on bool, runs *runsession.Service, reg *tools.Registry, datasources *datasource.Registry) {
	if on {
		runs.SweepWith(triage.Service{Tools: reg, Datasources: datasources, Limits: triage.DefaultLimits})
	}
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
	logDatasources(cfg, logger)

	serverMetrics := metrics.New()

	llmProvider, err := newLLMProvider(cfg, logger, serverMetrics)
	if err != nil {
		return err
	}
	sidecar, err := prepareAgentRuntime(cfg, llmProvider != nil, logger)
	if err != nil {
		return err
	}
	warnOptInLogging(logger, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Restore default signal handling so a second signal force-exits during shutdown.
	context.AfterFunc(ctx, stop)

	connectCtx, cancelConnect := context.WithTimeout(ctx, connectTimeout)
	database, err := db.Open(connectCtx, db.Config{URL: cfg.DatabaseURL, DirectURL: cfg.DatabaseDirectURL, MaxConnsFloor: poolFloor(cfg.Agent.Workers)}, logger)
	cancelConnect()
	if err != nil {
		return databaseExit(cfg, err)
	}
	defer database.Close()
	serverMetrics.Pool(database.Stat)

	appliedMigrations, err := database.Migrate(ctx)
	if err != nil {
		return migrationExit(err)
	}
	if err := checkDatabase(ctx, database.Queries(), cfg.Datasources, logger); err != nil {
		return err
	}

	serverID := instanceID()
	auditMetrics := serverMetrics.Audit()
	recorder := audit.New(audit.Config{
		DB: database, Server: serverID, Version: version, Metrics: auditMetrics, Retention: cfg.AuditRetention, Logger: logger,
	})
	audit.Install(recorder)
	if err := recorder.PartitionJob()(ctx); err != nil {
		logger.Warn("audit partitions not prepared", logattrs.Failure("audit_partitions_failed", err)...)
	}
	untilServed := context.WithoutCancel(ctx)
	stopAudit := startBackground(untilServed, "audit writer", recorder.Run, logger)
	defer stopAudit()
	recordMigrations(ctx, logger, appliedMigrations)
	membership := fleet.New(database, serverID, version, logger)
	serverMetrics.Fleet(membership.Live)
	stopFleet := startFleet(ctx, membership, logger)
	defer stopFleet()

	authService, err := newAuthService(ctx, cfg, database.Queries(), logger)
	if err != nil {
		return err
	}
	folderAccess := folders.NewAccess(database)
	accessRequests := accessrequests.New(database, folderAccess)
	authService.RecordChanges(database, accessrequests.ChangeLog{ChangeLog: activity.AuthChanges{}, Requests: accessRequests})
	if err := reconcileStartup(ctx, database, authService, cfg); err != nil {
		return err
	}

	slots := queryproxy.NewSlots(maxConcurrentQueries(cfg.Datasources), membership.Share)
	serverMetrics.DatasourceSlots(func() []metrics.SlotUsage { return slotUsage(slots) })
	auditReader := trail.NewReader(database, logger)
	if err := addAuditDatasource(registry, slots, auditReader, logger); err != nil {
		return described("audit_datasource_failed", err)
	}

	runner := queryproxy.NewRunner(registry, logger, queryproxy.WithSlots(slots), queryproxy.WithMetrics(serverMetrics.Proxy()))
	dashboardsService := dashboards.New(database, runner, folderAccess, logger)
	proposalsService := proposals.New(database, folderAccess, dashboardsService, logger)
	dashboardsService.SetProposer(proposalsService.Propose)
	proxyRunner := queryproxy.NewRunner(registry, logger, queryproxy.WithSlots(slots), queryproxy.WithValueCache(), queryproxy.WithMetrics(serverMetrics.Proxy()))
	proxyHandler := proxyhttp.NewHandler(proxyRunner, dashboardsService, dashboardsService, folderAccess, registry, logger)

	importsService := imports.New(database, dashboardsService, folderAccess, registry, logger)
	annotationsService := annotations.New(database, folderAccess, annotations.WithPanels(dashboardsService))
	annotationsHandler := annotations.NewHandler(annotationsService, logger)
	activityStore := activity.NewStore(database, folderAccess)
	searchService := search.New(database, folderAccess, cfg.PublicURL)
	savedSearchesService := savedsearches.New(database, folderAccess, registry, cfg.PublicURL, logger)
	observationsService := observations.New(database, folderAccess, proxyHandler, cfg.PublicURL, logger, observations.WithOutputs(dashboardsService, annotationsService), observations.WithPlacement(dashboardsService.Placement()))
	findingsService, err := findings.New(database, folderAccess, annotationsService, dashboardsService, observationsService, logger)
	if err != nil {
		return described("findings_setup_failed", err)
	}
	publicLinks := public.New(database, folderAccess, dashboardsService, registry, cfg.PublicURL, logger, public.WithRCAs(observationsService), public.WithVariableOptions(proxyHandler))
	publicLinksHandler := public.NewHandler(publicLinks, folderAccess, logger)
	const healthJob = "dashboard_health"
	viewersService := viewers.New(database)
	healthService := health.New(health.Deps{DB: database, Runner: runner, Access: folderAccess, PublicURL: cfg.PublicURL, Logger: logger, Job: healthJob})
	threadsService := threads.NewService(database, folderAccess, dashboardsService, annotationsService, observationsService, proposalsService)
	proposalsService.OnClosed(threadsService)
	threadsService.ShowAnswersWith(findingsService.ShowIn, findingsService.HideFrom)
	findingsService.ReadThreadsWith(func(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error {
		_, err := threadsService.ReadMessage(ctx, by, messageID)
		return err
	})
	findingURL := savedFindingURL(cfg.PublicURL)
	liveText := livetext.New(database, database.Listen, logger)
	baseObserveOpts := revisions.ObserveOptions{
		DashboardURL:       func(id, title string) string { return links.Dashboard(cfg.PublicURL, id, title) },
		ObservationURL:     func(id string) string { return links.Observation(cfg.PublicURL, id) },
		Finding:            findingOfRequest(findingsService, findingURL),
		SaveAnswer:         saveAnswerOf(findingsService),
		ShareAnswer:        shareAnswerOf(threadsService),
		LiveText:           liveText.Text,
		ShareTitle:         observationsService.RenameFromChat,
		ObservationVisible: observationsService.Visible,
	}
	revisionOpts := []revisions.Option{
		revisions.WithPanels(dashboardsService),
		revisions.WithSpecs(dashboardsService),
		revisions.WithBuilds(buildStore{dashboardsService}),
		revisions.WithObservations(runObservations{observationsService}),
		revisions.WithPrincipals(authService),
		revisions.WithAccess(folderAccess),
		revisions.WithThreads(threads.NewRuns(database)),
		revisions.WithMetrics(serverMetrics.AgentRequests()),
	}
	observeOpts, revisionOpts := withBuiltInAgent(llmProvider, cfg.LLM, baseObserveOpts, revisionOpts)
	revisionsService := revisions.NewService(database, logger, revisionOpts...)
	findingsService.RecordAnswerReadsWith(revisionsService.RecordAnswerRead)
	threadsService.ReadAnswersWith(revisionsService, threadAnswers{findings: findingsService, url: findingURL})
	revisionsService.ReadThreadsWith(func(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error {
		_, err := threadsService.ReadMessage(ctx, by, messageID)
		return err
	})
	asks := threads.NewAsks(threadsService, revisionsService, llmProvider != nil)
	claimsConfig := claims.Config{
		DB: database, Runs: revisionsService, Listen: database.Listen, PublicURL: cfg.PublicURL, InternalURL: sidecar.internalURL(),
		InstanceID: serverID, Model: sidecar.modelSettings(cfg), RunTimeout: cfg.Agent.RunTimeout, Logger: logger,
	}
	claimsService := claims.New(claimsConfig)
	runSessions := runsession.New(runsession.Config{
		DB: database, Runs: revisionsService, Claims: claimsService, Builds: dashboardsService, Dashboards: dashboardsService,
		Imports: importsService, Statuses: registry, Datasources: registry, Steps: claimsService,
		Prices: cfg.LLM.Prices, MaxRunCostUSD: cfg.Agent.MaxRunCostUSD, InvestigationCalls: cfg.Agent.InvestigationCalls, InvestigationLoop: cfg.Agent.InvestigationLoop, InvestigationCallsMax: cfg.Agent.InvestigationCallsMax, FindingGrounding: cfg.Agent.FindingGrounding, Logger: logger,
	})
	outbound := allowlist.New(database, allowlist.Lock{Locked: cfg.OutboundDomainsLocked, Domains: cfg.OutboundDomains}, logger)
	egressClient := egress.New(outbound, egress.Options{UserAgent: "SlashObserve/" + version, Logger: logger})
	mcpSourcesService := inboundmcp.New(inboundmcp.Deps{
		DB: database, Key: sealed.Key(cfg.SecretKey.Reveal()), Domains: outbound,
		Connector: inboundmcp.MCPConnector{Trip: egressClient.CallTripper(), PrivateTrip: egressClient.PrivateCallTripper(), Version: version}, Logger: logger,
	})
	var startHealthRun func(context.Context)
	toolRegistry := tools.New(tools.Deps{
		StartHealthRun: func(ctx context.Context) { startHealthRun(ctx) },
		Datasources:    registry,
		Runner:         runner,
		Dashboards:     dashboardsService,
		Threads:        threadsService,
		Asks:           asks,
		Proposals:      proposalsService,
		Imports:        importsService,
		Folders:        folderAccess,
		Findings:       findingsService,
		Observations:   observationsService,
		SavedSearches:  savedSearchesService,
		Health:         healthService,
		Annotations:    annotationsService,
		Activity:       activityStore,
		Audit:          auditReader,
		Runs:           revisions.NewRunDesk(revisionsService, observeOpts),
		Claims:         claimsService,
		RunSessions:    runSessions,
		LiveText:       liveText,
		Search:         searchService,
		Stars:          stars.New(database, folderAccess),
		AccessRequests: accessRequests,
		Viewers:        viewersService,
		Home:           homedash.New(database, folderAccess),
		Provider:       llmProvider,
		DraftBudget:    draftbudget.New(database),
		PublicURL:      cfg.PublicURL,
		MCPSources:     mcpSourcesService,
		Logger:         logger,
		LedgerTool:     cfg.Agent.InvestigationLoop,
	})
	sweepBeforeInvestigations(cfg.Agent.InvestigationBrief, runSessions, toolRegistry, registry)

	broker := events.NewBroker(database.Listen, logger)
	go broker.Run(ctx)
	stopClaims := startBackground(ctx, "claim listener", claimsService.Run, logger)
	defer stopClaims()
	stopLiveText := startBackground(ctx, "live text listener", liveText.Run, logger)
	defer stopLiveText()

	supervisor, stopAgentRuntime := sidecar.start(ctx, cfg, claimsService, logger)
	defer stopAgentRuntime()
	jobs := scheduler.New(logger, database, serverMetrics.Scheduler(), serverID)
	jobs.Register(healthJob, time.Hour, healthService.Run)
	jobs.Register("activity_prune", time.Hour, activity.PruneJob(database, cfg.ActivityRetention))
	jobs.Register("audit_partitions", 24*time.Hour, recorder.PartitionJob())
	stopAuditExport := registerAuditExport(ctx, jobs, cfg.AuditExport, auditReader, database, auditMetrics, logger)
	defer stopAuditExport()
	jobs.Register("webhook_deliveries_prune", time.Hour, webhooks.PruneJob(database))
	jobs.Register("oauth_prune", time.Hour, mcpauth.PruneJob(database))
	jobs.Register("run_tokens_prune", time.Hour, claims.PruneJob(database))
	jobs.Register("agent_requests_sweep", time.Minute, revisionsService.Sweep)
	stopScheduler := startScheduler(ctx, jobs, logger)
	defer stopScheduler()
	healthCtx, stopHealthRuns := context.WithCancel(ctx)
	healthHandler := health.NewHandler(healthCtx, healthService, health.NewRepairs(healthService, revisionsService), jobs)
	startHealthRun = healthHandler.StartRun
	defer func() {
		stopHealthRuns()
		healthHandler.Wait()
	}()

	stopAuditFolder := startInBackground(ctx, auditfolder.New(database, dashboardsService, folderAccess, dashprovision.Principal, logger).Start)
	defer stopAuditFolder()

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

	assetsClient := egress.New(outbound, egress.Options{UserAgent: "SlashObserve/" + version, Logger: logger, CacheTTL: time.Hour, CacheBytes: 64 << 20})

	datasourceAdmin := datasourceadmin.New(datasourceadmin.Deps{
		DB: database, Registry: registry, Slots: slots, Key: sealed.Key(cfg.SecretKey.Reveal()),
		Domains: outbound, OutboundLocked: cfg.OutboundDomainsLocked, Access: folderAccess, Logger: logger,
	})
	if err := datasourceAdmin.Load(ctx); err != nil {
		return withheld("datasources_load_failed", err)
	}
	stopDatasourceSync := startBackground(ctx, "datasource sync", datasourceAdmin.Run, logger)
	defer stopDatasourceSync()

	grafanaCtx, stopGrafana := context.WithCancel(ctx)
	grafanaService := grafanaconn.New(grafanaCtx, grafanaconn.Deps{
		DB: database, Egress: egressClient, Domains: outbound, Imports: importsService, Access: folderAccess,
		Principals: authService, InstanceID: serverID, SecretKey: []byte(cfg.SecretKey.Reveal()), Logger: logger,
	})
	grafanaService.Start()
	defer grafanaService.Wait()
	defer stopGrafana()

	stopApprovals := startBackground(ctx, "MCP source approvals", func(ctx context.Context) {
		approveListedTools(ctx, mcpSourcesService, cfg, logger)
	}, logger)
	defer stopApprovals()

	webhooksService := webhooks.NewService(webhooks.Deps{
		DB: database, Activity: activityStore, Principals: authService, Domains: outbound, Sender: egressClient, Key: sealed.Key(cfg.SecretKey.Reveal()), Logger: logger,
	})
	stopWebhooks := startBackground(ctx, "webhook worker", webhooks.NewWorker(webhooks.WorkerDeps{
		DB: database, Activity: activityStore, Principals: authService, Sender: egressClient, Key: sealed.Key(cfg.SecretKey.Reveal()),
		Followers: broker, InstanceID: serverID, Logger: logger,
	}).Run, logger)
	defer stopWebhooks()

	oauthServer := mcpauth.New(mcpauth.Config{PublicURL: cfg.PublicURL, Logger: logger, Share: membership.Share}, database, authService)
	handler, err := server.New(server.Options{
		Logger:          logger,
		TrustedProxies:  cfg.TrustedProxies,
		Draining:        ctx.Done(),
		PublicURL:       cfg.PublicURL,
		Auth:            authService,
		Datasources:     registry,
		DatasourceUsage: dashboardsService,
		API: []server.Registrar{
			dashboardsService,
			folders.NewHandler(database, folderAccess, logger).SettleSharingWith(accessRequests.SettleFolder).GuardOpeningWith(dashboardsService.Placement().OpeningGuard()),
			accessrequests.NewHandler(accessRequests, logger),
			proposals.NewHandler(proposalsService, folderAccess.RequireDashboard(folders.NeedView), logger),
			imports.NewHandler(importsService, folderAccess),
			grafanaconn.NewHandler(grafanaService),
			webhooks.NewHandler(webhooksService, logger),
			repairs.NewHandler(repairs.New(repairs.Deps{DB: database, Access: folderAccess, Revisions: revisionsService, PublicURL: cfg.PublicURL, Logger: logger})),
			stream.NewHandler(broker, dashboardsService, authService, folderAccess, folderAccess.RequireDashboard(folders.NeedView), logger),
			activity.NewHandler(activityStore, logger),
			trail.NewHandler(auditReader, authService, logger),
			agentsview.NewHandler(database, agentsview.Observe{Enabled: revisionsService.Available(), Provider: observeOpts.Provider, Model: observeOpts.Model}, logger),
			search.NewHandler(searchService, logger),
			stream.NewActivityHandler(broker, activityStore, authService, logger),
			stream.NewAuditHandler(broker, auditReader, authService, logger),
			threads.NewComments(threadsService, revisionsService),
			threads.NewThreads(threadsService, asks),
			revisionsService,
			revisions.NewObserveHandler(revisionsService, observeOpts),
			proxyHandler,
			findings.NewHandler(findingsService, findingURL, logger),
			observations.NewHandler(observationsService, logger, observations.WithPanelChecker(toolRegistry)),
			observations.NewRCAHandler(observationsService, logger),
			savedsearches.NewHandler(savedSearchesService, logger, savedsearches.WithSaveGuard(toolRegistry)),
			proxyhttp.NewFindingData(proxyHandler, findingsService),
			proxyhttp.NewObservationData(proxyHandler, observationsService),
			users.NewHandler(database, authService, folderAccess, logger),
			outbound,
			prompts.NewHandler(database, logger),
			news.NewHandler(egressClient, logger),
			actions.NewHandler(dashboardsService, egressClient, folderAccess.RequireDashboard(folders.NeedEdit), logger),
			assets.NewHandler(assetsClient, logger),
			annotationsHandler,
			dashprovision.NewHandler(provisioner, logger),
			stars.NewHandler(stars.New(database, folderAccess), folderAccess.RequireDashboard(folders.NeedView), logger),
			viewers.NewHandler(viewersService, folderAccess, logger),
			homedash.NewHandler(homedash.New(database, folderAccess), logger),
			publicLinksHandler,
			dashprovision.NewApplyHandler(dashprovision.NewApplier(database, dashboardsService, folderAccess), logger),
			healthHandler,
			drafthttp.NewHandler(toolRegistry, logger),
			discoveryhttp.NewHandler(toolRegistry, logger),
			datasourceadmin.NewHandler(datasourceAdmin, logger),
			inboundmcp.NewHandler(mcpSourcesService, logger),
			tailhttp.NewHandler(registry, proxyRunner, authService, logger),
			oauthServer,
		},
		Public: []server.PublicRegistrar{publicLinksHandler, proxyhttp.NewPublicData(proxyHandler, publicLinks)},
		Mounts: slices.Concat([]server.Mount{{
			Path: "/mcp",
			Handler: mcp.NewHandler(toolRegistry, mcpCredentials{keys: authService, runs: claimsService, oauth: oauthServer, supervisor: supervisor}, mcp.Options{
				PublicURL: cfg.PublicURL, ResourceMetadataURL: oauthServer.ResourceMetadataURL(), Version: version, Logger: logger, Runs: runSessions, PanelApp: mcp.BuiltPanel(),
			}),
		}}, oauthMounts(oauthServer), metricsMounts(cfg.Metrics, serverMetrics)),
		Redirects:     server.Redirects{GrafanaDashboard: grafanaService.DashboardRedirect(authService.RequestPrincipal)},
		Metrics:       serverMetrics.HTTP(),
		Audit:         recorder,
		ContractCheck: cfg.APIContractCheck,
		Readiness: []server.Check{
			{Name: "database", Run: database.Ping},
			{Name: "migrations", Run: database.CheckMigrations},
		},
	})
	if err != nil {
		return described("http_setup_failed", err)
	}
	return serveRecorded(ctx, logger, cfg.HTTPAddr, serverID, handler, broker.Close)
}

func savedFindingURL(publicURL *url.URL) func(findings.Finding) string {
	return func(f findings.Finding) string {
		if !f.Saved() {
			return ""
		}
		return links.Observation(publicURL, f.ObservationID)
	}
}

func findingOfRequest(service *findings.Service, findingURL func(f findings.Finding) string) func(context.Context, auth.Principal, pgtype.UUID) (json.RawMessage, bool, error) {
	return func(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (json.RawMessage, bool, error) {
		f, found, err := service.ForRequest(ctx, by, requestID)
		if err != nil || !found {
			return nil, found, err
		}
		raw, err := json.Marshal(findings.JSONOf(f, findingURL(f)))
		return raw, err == nil, err
	}
}

type threadAnswers struct {
	findings *findings.Service
	url      func(f findings.Finding) string
}

func (a threadAnswers) RunAnswer(ctx context.Context, by auth.Principal, run pgtype.UUID) (threads.Answer, bool, error) {
	f, found, err := a.findings.ForRequest(ctx, by, run)
	if err != nil || !found {
		return threads.Answer{}, false, err
	}
	answer, err := answerOf(run, f, by, a.url(f))
	if err != nil {
		return threads.Answer{}, false, err
	}
	return answer, true, nil
}

func answerOf(run pgtype.UUID, f findings.Finding, by auth.Principal, url string) (threads.Answer, error) {
	raw, err := json.Marshal(findings.JSONOf(f, url))
	if err != nil {
		return threads.Answer{}, err
	}
	return threads.Answer{
		RunID:           run.String(),
		FindingID:       f.ID,
		Title:           f.Title,
		Markdown:        f.Markdown,
		EvidencePanels:  len(f.Evidence),
		SuggestedChange: f.SuggestedChange,
		ObservationID:   f.ObservationID,
		CanSave:         f.SavableBy(by),
		Finding:         raw,
	}, nil
}

func saveAnswerOf(service *findings.Service) revisions.AnswerSaver {
	return func(ctx context.Context, by auth.Principal, requestID pgtype.UUID) (revisions.SavedAnswer, error) {
		f, replayed, err := service.SaveAsObservation(ctx, by, requestID)
		if err != nil {
			return revisions.SavedAnswer{}, saveAnswerError(err)
		}
		return revisions.SavedAnswer{ObservationID: f.ObservationID, Replayed: replayed}, nil
	}
}

func saveAnswerError(err error) error {
	switch {
	case errors.Is(err, findings.ErrNotFound):
		return revisions.ErrNoAnswer
	case errors.Is(err, findings.ErrRequesterOnly):
		return revisions.ErrNotRequester
	}
	return err
}

func shareAnswerOf(service *threads.Service) revisions.AnswerSharer {
	return func(ctx context.Context, by auth.Principal, requestID, threadID pgtype.UUID) (revisions.SharedAnswer, error) {
		shared, err := service.ShareRun(ctx, by, threads.ShareInput{RunID: requestID, ThreadID: threadID})
		if err != nil {
			return revisions.SharedAnswer{}, shareAnswerError(err)
		}
		return revisions.SharedAnswer{ThreadID: shared.Thread.ID, MessageID: shared.Message.ID, Replayed: shared.Replayed}, nil
	}
}

func shareAnswerError(err error) error {
	var invalid *threads.InvalidInputError
	switch {
	case errors.Is(err, threads.ErrRunNotFound):
		return revisions.ErrRequestNotFound
	case errors.Is(err, threads.ErrNoAnswer):
		return revisions.ErrNoAnswerToShare
	case errors.Is(err, threads.ErrNotShareable):
		return revisions.ErrNotShareable
	case errors.Is(err, threads.ErrNotFound), errors.As(err, &invalid) && invalid.Code == threads.CodeWrongThread:
		return revisions.ErrShareThread
	case errors.Is(err, threads.ErrTargetNotFound):
		return revisions.ErrRequestNotFound
	}
	return err
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

type runObservations struct {
	service *observations.Service
}

func (r runObservations) OpenForRun(ctx context.Context, q *dbsqlc.Queries, in revisions.RunObservationOpen) (pgtype.UUID, error) {
	id, err := r.service.OpenForRun(ctx, q, observations.RunOpen{Run: in.Run, By: in.By, ObservationID: in.ObservationID, Strict: in.Strict})
	return id, openObservationError(err)
}

func openObservationError(err error) error {
	if errors.Is(err, observations.ErrNotFound) {
		return revisions.ErrObservationNotFound
	}
	return err
}

func revisionsBuildError(err error) error {
	var conflict *dashboards.ConflictError
	switch {
	case errors.As(err, &conflict):
		return &revisions.BuildConflictError{Latest: conflict.Latest}
	case errors.Is(err, dashboards.ErrNoBuild):
		return revisions.ErrNoBuild
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

func (m mcpCredentials) Authenticate(ctx context.Context, header string) (mcp.Credential, bool, error) {
	if cred, ok, claimed := m.observeAgent(header); claimed {
		return cred, ok, nil
	}
	if p, ok, err := m.keys.BearerPrincipal(ctx, header); err != nil || ok {
		cred := mcp.Credential{Principal: p}
		if ok && p.Kind == auth.KindServiceAccount {
			cred.Claimer = &claims.Claimer{Kind: revisions.AssignServiceAccount, ID: p.ID}
		}
		return cred, ok, err
	}
	if run, ok, err := m.runs.Authenticate(ctx, header); err != nil || ok {
		return mcp.Credential{Principal: run.Request.RequestedBy, Run: &run.Request, Agent: run.Agent, Mode: run.Mode}, ok, err
	}
	access, ok, err := m.oauth.Authenticate(ctx, header)
	if err != nil || !ok {
		return mcp.Credential{}, ok, err
	}
	claimer := &claims.Claimer{Kind: revisions.AssignGrant, ID: access.GrantID, Person: access.Principal.ID, Client: access.Client}
	return mcp.Credential{Principal: access.Principal, OAuth: true, Client: access.Client, Claimer: claimer, Mode: access.Mode}, true, nil
}

func oauthMounts(oauthServer *mcpauth.Server) []server.Mount {
	paths := oauthServer.Paths()
	mounts := make([]server.Mount, len(paths))
	for i, path := range paths {
		mounts[i] = server.Mount{Path: path, Handler: oauthServer}
	}
	return mounts
}

func metricsMounts(enabled bool, registry *metrics.Registry) []server.Mount {
	if !enabled {
		return nil
	}
	return []server.Mount{{Path: "/metrics", Handler: registry.Handler()}}
}

func newLLMProvider(cfg config.Config, logger *slog.Logger, serverMetrics *metrics.Registry) (llm.Provider, error) {
	provider, err := providers.New(cfg.LLM, logger)
	if err != nil {
		return nil, described("llm_provider_failed", err)
	}
	if provider == nil {
		logger.Info("no llm provider configured; the agent is off")
		return nil, nil
	}
	provider = llm.WithMetrics(provider, serverMetrics.ModelCalls())
	logger.Info("llm provider configured", slog.String("provider", provider.Name()), slog.String("model", provider.Model()))
	if err := requirePricedModel(cfg, provider); err != nil {
		return nil, described("invalid_config", err)
	}
	return provider, nil
}

func requirePricedModel(cfg config.Config, provider llm.Provider) error {
	if provider == nil || cfg.Agent.MaxRunCostUSD <= 0 {
		return nil
	}
	if price, ok := cfg.LLM.Prices.Lookup(provider.Model()); !ok || !price.Complete() {
		return fmt.Errorf("SLASHOBSERVE_AGENT_RUN_MAX_COST_USD is set but model %q has no price for every token class; add it to SLASHOBSERVE_LLM_PRICES_FILE", provider.Model())
	}
	return nil
}

func warnOptInLogging(logger *slog.Logger, cfg config.Config) {
	if cfg.LLM.LogProviderErrors {
		logger.Warn("provider error messages are logged; a provider can echo prompt text in them")
	}
}

func withBuiltInAgent(provider llm.Provider, cfg config.LLM, observe revisions.ObserveOptions, opts []revisions.Option) (revisions.ObserveOptions, []revisions.Option) {
	if provider == nil {
		return observe, opts
	}
	observe.Provider, observe.Model = provider.Name(), provider.Model()
	observe.Effort = reportedEffort(cfg)
	return observe, append(opts, revisions.WithBuiltInAgent())
}

func reportedEffort(llm config.LLM) string {
	if llm.Provider == config.LLMProviderAnthropic {
		return cmp.Or(llm.Effort, "medium")
	}

	return ""
}

func poolFloor(workers int) int { return 2*workers + 8 }

func registerAuditExport(ctx context.Context, jobs *scheduler.Scheduler, export config.AuditExport, reader *trail.Reader, database *db.DB, m metrics.Audit, logger *slog.Logger) func() {
	if export.Endpoint == nil {
		return func() {}
	}
	cursor := auditexport.NewCursor(database.Queries())
	sender := auditexport.NewSender(export.Endpoint, export.Headers)
	exporter := auditexport.New(reader, cursor, sender.Send, m, logger)
	jobs.Register(auditexport.JobName, auditexport.JobEvery, exporter.Run)
	return startBackground(ctx, "audit export lag", auditexport.NewLagWatcher(cursor, m, logger).Run, logger)
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

func startBackground(ctx context.Context, name string, run func(context.Context), logger *slog.Logger) func() {
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(loopCtx)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			logger.Warn("background loop did not stop in time", slog.String("loop", name))
		}
	}
}

func maxConcurrentQueries(datasources []config.Datasource) map[string]int {
	totals := make(map[string]int, len(datasources))
	for _, ds := range datasources {
		totals[ds.UID] = ds.MaxConcurrentQueries
	}
	return totals
}

func startInBackground(ctx context.Context, work func(context.Context)) func() {
	workCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		work(workCtx)
	}()
	return func() {
		stop()
		<-done
	}
}

func addAuditDatasource(registry *datasource.Registry, slots *queryproxy.Slots, reader *trail.Reader, logger *slog.Logger) error {
	in, err := auditsource.Instance(reader, logger)
	if err != nil {
		return err
	}
	if err := registry.AddBuiltIn(in); err != nil {
		return fmt.Errorf("the audit datasource needs the uid %q, which the datasources file already uses: %w", auditsource.UID, err)
	}
	slots.SetTotal(auditsource.UID, auditsource.MaxConcurrentQueries)
	return nil
}

func slotUsage(slots *queryproxy.Slots) []metrics.SlotUsage {
	usage := slots.Usage()
	out := make([]metrics.SlotUsage, 0, len(usage))
	for _, u := range usage {
		if u.Datasource == auditsource.UID {
			continue
		}
		out = append(out, metrics.SlotUsage{Datasource: u.Datasource, InUse: u.InUse, Limit: u.Limit})
	}
	return out
}

func startFleet(ctx context.Context, membership *fleet.Membership, logger *slog.Logger) func() {
	fleetCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		membership.Run(fleetCtx)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			logger.Warn("server heartbeat did not stop in time")
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

func serve(ctx context.Context, logger *slog.Logger, listener net.Listener, handler http.Handler, onShutdown ...func()) error {
	requests, cutRequests := context.WithCancel(context.Background())
	defer cutRequests()
	// WriteTimeout is deliberately unset: a write deadline would cut SSE streams.
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		BaseContext:       func(net.Listener) context.Context { return requests },
	}
	for _, f := range onShutdown {
		srv.RegisterOnShutdown(f)
	}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", listener.Addr().String()))
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down", slog.Duration("timeout", shutdownTimeout))
	return shutdown(srv, cutRequests, drainPeriod, requestsCutAfter, shutdownTimeout)
}

func shutdown(srv *http.Server, cutRequests context.CancelFunc, drain, cutAfter, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cut := time.AfterFunc(cutAfter, cutRequests)
	defer cut.Stop()
	time.Sleep(drain)
	return srv.Shutdown(ctx)
}
