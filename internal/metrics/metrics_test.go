package metrics_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/slashobserve/slashobserve/internal/agent"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/metrics"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/server"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func scrape(t *testing.T, r *metrics.Registry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape status %d", rec.Code)
	}
	return rec.Body.String()
}

func newPoolStat(t *testing.T) func() *pgxpool.Stat {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://nobody@127.0.0.1:1/none?pool_max_conns=7")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool.Stat
}

func TestScrapeServesEverySeries(t *testing.T) {
	r := metrics.New()
	r.HTTP().ObserveRequest("/api/v1/dashboards/:id", "GET", 200, 20*time.Millisecond)
	r.AgentRuns().AgentRun("create", "succeeded")
	r.ModelCalls().ModelCall("anthropic", "claude-haiku-5-5", "ok", 3*time.Second, llm.Usage{InputTokens: 1200, OutputTokens: 300, CacheReadTokens: 900, CacheWriteTokens: 50})
	r.Proxy().ProxyQuery("prometheus", "ok", 40*time.Millisecond)
	r.Proxy().ProxyCache(true)
	r.Scheduler().JobStarted("annotations")
	r.Pool(newPoolStat(t))

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_http_request_duration_seconds_bucket{method="GET",route="/api/v1/dashboards/:id",status="200",le="0.025"} 1`,
		`slashobserve_agent_runs_total{kind="create",outcome="succeeded"} 1`,
		`slashobserve_model_call_duration_seconds_count{model="claude-haiku-5-5",outcome="ok",provider="anthropic"} 1`,
		`slashobserve_model_tokens_total{direction="input"} 1200`,
		`slashobserve_model_tokens_total{direction="output"} 300`,
		`slashobserve_model_tokens_total{direction="cache_read"} 900`,
		`slashobserve_model_tokens_total{direction="cache_write"} 50`,
		`slashobserve_proxy_query_duration_seconds_count{datasource_type="prometheus",outcome="ok"} 1`,
		`slashobserve_proxy_cache_total{result="hit"} 1`,
		`slashobserve_scheduler_job_running{job="annotations"} 1`,
		`slashobserve_db_pool_connections{state="acquired"} 0`,
		`slashobserve_db_pool_connections{state="idle"} 0`,
		`slashobserve_db_pool_connections{state="constructing"} 0`,
		`slashobserve_db_pool_max_connections 7`,
		`slashobserve_db_pool_acquires_total 0`,
		`slashobserve_db_pool_empty_acquires_total 0`,
		`slashobserve_db_pool_acquire_wait_seconds_total 0`,
		`go_goroutines `,
		`process_cpu_seconds_total `,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

func TestSeriesWithoutObservationsAreAbsent(t *testing.T) {
	body := scrape(t, metrics.New())
	if strings.Contains(body, "slashobserve_") {
		t.Fatalf("a fresh registry serves slashobserve series:\n%s", body)
	}
}

func TestHTTPLabelsStayBounded(t *testing.T) {
	r := metrics.New()
	h := r.HTTP()
	h.ObserveRequest("", "GET", 404, time.Millisecond)
	h.ObserveRequest("/api/v1/folders", "BREW", 404, time.Millisecond)
	h.ObserveRequest("/mcp", "POST", 200, time.Millisecond)

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_http_request_duration_seconds_count{method="GET",route="unmatched",status="404"} 1`,
		`slashobserve_http_request_duration_seconds_count{method="other",route="/api/v1/folders",status="404"} 1`,
		`slashobserve_http_request_duration_seconds_count{method="POST",route="/mcp",status="200"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
	if strings.Contains(body, "BREW") {
		t.Error("an unknown method became a label value")
	}
}

func TestModelCallSkipsZeroTokenDirectionsAndAddsUp(t *testing.T) {
	r := metrics.New()
	calls := r.ModelCalls()
	calls.ModelCall("openai", "m", "rate_limited", time.Second, llm.Usage{})
	calls.ModelCall("openai", "m", "ok", time.Second, llm.Usage{InputTokens: 10, OutputTokens: 5})
	calls.ModelCall("openai", "m", "ok", time.Second, llm.Usage{InputTokens: 7})

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_model_tokens_total{direction="input"} 17`,
		`slashobserve_model_tokens_total{direction="output"} 5`,
		`slashobserve_model_call_duration_seconds_count{model="m",outcome="rate_limited",provider="openai"} 1`,
		`slashobserve_model_call_duration_seconds_count{model="m",outcome="ok",provider="openai"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
	if strings.Contains(body, `direction="cache_read"`) {
		t.Error("a direction with no tokens was recorded")
	}
}

func TestProxyCacheCountsHitsAndMisses(t *testing.T) {
	r := metrics.New()
	p := r.Proxy()
	p.ProxyCache(false)
	p.ProxyCache(false)
	p.ProxyCache(true)

	body := scrape(t, r)
	for _, want := range []string{`slashobserve_proxy_cache_total{result="miss"} 2`, `slashobserve_proxy_cache_total{result="hit"} 1`} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

func TestSchedulerCountsRunsAndTracksTheOnesInFlight(t *testing.T) {
	r := metrics.New()
	s := r.Scheduler()
	s.JobStarted("cleanup")
	s.JobStarted("cleanup")
	s.JobFinished("cleanup", "succeeded")
	s.JobStarted("digest")
	s.JobFinished("digest", "failed")

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_scheduler_job_running{job="cleanup"} 1`,
		`slashobserve_scheduler_job_running{job="digest"} 0`,
		`slashobserve_scheduler_jobs_total{job="cleanup",outcome="succeeded"} 1`,
		`slashobserve_scheduler_jobs_total{job="digest",outcome="failed"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

type fakeSession struct {
	turn llm.Turn
	err  error
}

func (s fakeSession) Next(context.Context, []llm.ToolResult) (llm.Turn, error) { return s.turn, s.err }
func (fakeSession) Compact(llm.Compaction) error                               { return nil }

type fakeProvider struct{ session fakeSession }

func (fakeProvider) Name() string  { return "anthropic" }
func (fakeProvider) Model() string { return "claude-haiku-5-5" }
func (p fakeProvider) Start(context.Context, llm.Request) (llm.Session, error) {
	return p.session, nil
}

func nextOnce(t *testing.T, p llm.Provider) {
	t.Helper()
	session, err := p.Start(t.Context(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = session.Next(t.Context(), nil)
}

func TestMeteredProviderRecordsEveryModelCallWithItsOutcome(t *testing.T) {
	r := metrics.New()
	nextOnce(t, llm.WithMetrics(fakeProvider{fakeSession{turn: llm.Turn{Usage: llm.Usage{InputTokens: 40, OutputTokens: 4}}}}, r.ModelCalls()))
	nextOnce(t, llm.WithMetrics(fakeProvider{fakeSession{err: fmt.Errorf("wrapped: %w", &llm.CallError{Failure: llm.RateLimited})}}, r.ModelCalls()))
	nextOnce(t, llm.WithMetrics(fakeProvider{fakeSession{err: context.Canceled}}, r.ModelCalls()))
	nextOnce(t, llm.WithMetrics(fakeProvider{fakeSession{err: context.DeadlineExceeded}}, r.ModelCalls()))
	nextOnce(t, llm.WithMetrics(fakeProvider{fakeSession{err: errors.New("boom")}}, r.ModelCalls()))

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_model_call_duration_seconds_count{model="claude-haiku-5-5",outcome="ok",provider="anthropic"} 1`,
		`slashobserve_model_call_duration_seconds_count{model="claude-haiku-5-5",outcome="rate_limited",provider="anthropic"} 1`,
		`slashobserve_model_call_duration_seconds_count{model="claude-haiku-5-5",outcome="canceled",provider="anthropic"} 1`,
		`slashobserve_model_call_duration_seconds_count{model="claude-haiku-5-5",outcome="call_timeout",provider="anthropic"} 1`,
		`slashobserve_model_call_duration_seconds_count{model="claude-haiku-5-5",outcome="error",provider="anthropic"} 1`,
		`slashobserve_model_tokens_total{direction="input"} 40`,
		`slashobserve_model_tokens_total{direction="output"} 4`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

type fakeReviser struct {
	result revisions.Result
	err    error
}

func (f fakeReviser) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return f.result, f.err
}

func TestMeteredReviserRecordsTheKindAndOutcomeOfEveryRun(t *testing.T) {
	r := metrics.New()
	runs := []struct {
		reviser revisions.Reviser
		req     revisions.Request
	}{
		{fakeReviser{result: revisions.Result{Version: 1}}, revisions.Request{Source: revisions.SourceObserve}},
		{fakeReviser{result: revisions.Result{Version: 2}}, revisions.Request{Source: revisions.SourceComment, DashboardID: "d"}},
		{fakeReviser{result: revisions.Result{FailureCode: revisions.FailureRateLimited}, err: errors.New("failed")}, revisions.Request{Source: revisions.SourcePrompt, DashboardID: "d"}},
		{fakeReviser{err: fmt.Errorf("wrapped: %w", revisions.ErrLeaseLost)}, revisions.Request{Source: revisions.SourceSuggest, DashboardID: "d"}},
		{fakeReviser{err: errors.New("boom")}, revisions.Request{Source: revisions.SourceObserve}},
	}
	for _, run := range runs {
		_, _ = agent.WithMetrics(run.reviser, r.AgentRuns()).Revise(t.Context(), run.req)
	}

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_agent_runs_total{kind="create",outcome="succeeded"} 1`,
		`slashobserve_agent_runs_total{kind="revise",outcome="succeeded"} 1`,
		`slashobserve_agent_runs_total{kind="revise",outcome="rate_limited"} 1`,
		`slashobserve_agent_runs_total{kind="suggest",outcome="lease_lost"} 1`,
		`slashobserve_agent_runs_total{kind="create",outcome="internal"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

type fakeDatasource struct{ err error }

func (fakeDatasource) Type() string                      { return "prometheus" }
func (fakeDatasource) CheckHealth(context.Context) error { return nil }
func (fakeDatasource) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}
func (fakeDatasource) Lint(context.Context, string) ([]datasource.Diagnostic, error) { return nil, nil }
func (fakeDatasource) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, nil
}
func (f fakeDatasource) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}, nil
}
func (f fakeDatasource) QueryInstant(ctx context.Context, q datasource.Query, r datasource.TimeRange) ([]datasource.Frame, error) {
	return f.QueryRange(ctx, q, r, time.Minute)
}
func (fakeDatasource) Close() error { return nil }

func TestRunnerRecordsEveryUpstreamQueryAndCacheLookup(t *testing.T) {
	r := metrics.New()
	reg, err := datasource.NewRegistry(slog.New(slog.DiscardHandler),
		datasource.Instance{UID: "up", Datasource: fakeDatasource{}},
		datasource.Instance{UID: "down", Datasource: fakeDatasource{err: datasource.ErrUnavailable}},
	)
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(reg, slog.New(slog.DiscardHandler), queryproxy.WithMetrics(r.Proxy()))
	dashboard := &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "t",
		Time:        spec.TimeRange{From: "now-6h", To: "now"},
		Panels: []spec.Panel{
			{ID: "a", Type: spec.Timeseries, Title: "a", Datasource: "up", Queries: []spec.Query{{Query: "up"}}},
			{ID: "b", Type: spec.Timeseries, Title: "b", Datasource: "down", Queries: []spec.Query{{Query: "up"}}},
		},
	}
	now := time.Now()
	for _, id := range []string{"a", "b"} {
		_, _ = runner.RunPanel(t.Context(), queryproxy.Request{Dashboard: dashboard, PanelID: id, Range: datasource.TimeRange{From: now.Add(-time.Hour), To: now}})
	}
	runner.RecordCacheLookup(true)
	runner.RecordCacheLookup(false)

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_proxy_query_duration_seconds_count{datasource_type="prometheus",outcome="ok"} 1`,
		`slashobserve_proxy_query_duration_seconds_count{datasource_type="prometheus",outcome="datasource_unavailable"} 1`,
		`slashobserve_proxy_cache_total{result="hit"} 1`,
		`slashobserve_proxy_cache_total{result="miss"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

type openAuth struct{}

func (openAuth) Mount(g *gin.RouterGroup) auth.Routes { return auth.NewRoutes(g) }

func TestServerRecordsRequestsByRouteTemplate(t *testing.T) {
	r := metrics.New()
	var logs strings.Builder
	handler, err := server.New(server.Options{
		Logger:  slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})),
		Auth:    openAuth{},
		Mounts:  []server.Mount{{Path: "/metrics", Handler: r.Handler()}},
		Metrics: r.HTTP(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/healthz", "/healthz", "/api/v1/no-such-route"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics answered %d", rec.Code)
	}

	body := scrape(t, r)
	for _, want := range []string{
		`slashobserve_http_request_duration_seconds_count{method="GET",route="/healthz",status="200"} 2`,
		`slashobserve_http_request_duration_seconds_count{method="GET",route="unmatched",status="404"} 1`,
		`slashobserve_http_request_duration_seconds_count{method="GET",route="/metrics",status="200"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
	if strings.Contains(body, "no-such-route") {
		t.Error("an unmatched path became a label value")
	}
	if strings.Contains(logs.String(), "path=/metrics") {
		t.Errorf("a scrape was logged above debug:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "path=/api/v1/no-such-route") {
		t.Errorf("an API request was not logged:\n%s", logs.String())
	}
}
