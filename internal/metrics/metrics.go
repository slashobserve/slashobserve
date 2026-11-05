package metrics

import (
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/slashobserve/slashobserve/internal/llm"
)

const unmatchedRoute = "unmatched"

var (
	modelCallBuckets  = []float64{0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300, 600}
	proxyQueryBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
	knownMethods      = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true}
)

type Registry struct {
	registry *prometheus.Registry

	httpRequests     *prometheus.HistogramVec
	agentRuns        *prometheus.CounterVec
	modelCalls       *prometheus.HistogramVec
	modelTokens      *prometheus.CounterVec
	proxyQueries     *prometheus.HistogramVec
	proxyCache       *prometheus.CounterVec
	schedulerJobs    *prometheus.CounterVec
	schedulerRunning *prometheus.GaugeVec
}

func New() *Registry {
	r := &Registry{
		registry: prometheus.NewRegistry(),
		httpRequests: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "slashobserve_http_request_duration_seconds",
			Help:    "Time to answer an HTTP request, by route template, method and status.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method", "status"}),
		agentRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "slashobserve_agent_runs_total",
			Help: "Agent runs that ended, by kind (create, revise or suggest) and outcome (succeeded or the failure code).",
		}, []string{"kind", "outcome"}),
		modelCalls: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "slashobserve_model_call_duration_seconds",
			Help:    "Time of one model call including its retries, by provider, model and outcome.",
			Buckets: modelCallBuckets,
		}, []string{"provider", "model", "outcome"}),
		modelTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "slashobserve_model_tokens_total",
			Help: "Tokens the model provider reported, by direction (input, output, cache_read or cache_write).",
		}, []string{"direction"}),
		proxyQueries: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "slashobserve_proxy_query_duration_seconds",
			Help:    "Time of one datasource call from the query proxy, by datasource type and outcome (ok or the error code).",
			Buckets: proxyQueryBuckets,
		}, []string{"datasource_type", "outcome"}),
		proxyCache: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "slashobserve_proxy_cache_total",
			Help: "Panel data lookups in the query proxy's result cache, by result (hit or miss).",
		}, []string{"result"}),
		schedulerJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "slashobserve_scheduler_jobs_total",
			Help: "Scheduled job runs that ended, by job and outcome.",
		}, []string{"job", "outcome"}),
		schedulerRunning: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "slashobserve_scheduler_job_running",
			Help: "Runs of a scheduled job in flight on this server.",
		}, []string{"job"}),
	}
	r.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		r.httpRequests, r.agentRuns, r.modelCalls, r.modelTokens, r.proxyQueries, r.proxyCache, r.schedulerJobs, r.schedulerRunning,
	)
	return r
}

type HTTP struct{ requests *prometheus.HistogramVec }

func (r *Registry) HTTP() HTTP { return HTTP{requests: r.httpRequests} }

func (h HTTP) ObserveRequest(route, method string, status int, took time.Duration) {
	if route == "" {
		route = unmatchedRoute
	}
	if !knownMethods[method] {
		method = "other"
	}
	h.requests.WithLabelValues(route, method, strconv.Itoa(status)).Observe(took.Seconds())
}

type AgentRuns struct{ runs *prometheus.CounterVec }

func (r *Registry) AgentRuns() AgentRuns { return AgentRuns{runs: r.agentRuns} }

func (a AgentRuns) AgentRun(kind, outcome string) {
	a.runs.WithLabelValues(kind, outcome).Inc()
}

type ModelCalls struct {
	calls  *prometheus.HistogramVec
	tokens *prometheus.CounterVec
}

func (r *Registry) ModelCalls() ModelCalls {
	return ModelCalls{calls: r.modelCalls, tokens: r.modelTokens}
}

func (m ModelCalls) ModelCall(provider, model, outcome string, took time.Duration, usage llm.Usage) {
	m.calls.WithLabelValues(provider, model, outcome).Observe(took.Seconds())
	for direction, n := range map[string]int64{
		"input":       usage.InputTokens,
		"output":      usage.OutputTokens,
		"cache_read":  usage.CacheReadTokens,
		"cache_write": usage.CacheWriteTokens,
	} {
		if n > 0 {
			m.tokens.WithLabelValues(direction).Add(float64(n))
		}
	}
}

type Proxy struct {
	queries *prometheus.HistogramVec
	cache   *prometheus.CounterVec
}

func (r *Registry) Proxy() Proxy { return Proxy{queries: r.proxyQueries, cache: r.proxyCache} }

func (p Proxy) ProxyQuery(datasourceType, outcome string, took time.Duration) {
	p.queries.WithLabelValues(datasourceType, outcome).Observe(took.Seconds())
}

func (p Proxy) ProxyCache(hit bool) {
	result := "miss"
	if hit {
		result = "hit"
	}
	p.cache.WithLabelValues(result).Inc()
}

type Scheduler interface {
	JobStarted(job string)
	JobFinished(job, outcome string)
}

type schedulerMetrics struct {
	jobs    *prometheus.CounterVec
	running *prometheus.GaugeVec
}

func (r *Registry) Scheduler() Scheduler {
	return schedulerMetrics{jobs: r.schedulerJobs, running: r.schedulerRunning}
}

func (s schedulerMetrics) JobStarted(job string) {
	s.running.WithLabelValues(job).Inc()
}

func (s schedulerMetrics) JobFinished(job, outcome string) {
	s.running.WithLabelValues(job).Dec()
	s.jobs.WithLabelValues(job, outcome).Inc()
}

func (r *Registry) Pool(stat func() *pgxpool.Stat) {
	r.registry.MustRegister(poolCollector{stat: stat})
}

var (
	poolConnections = prometheus.NewDesc("slashobserve_db_pool_connections", "Database pool connections by state (acquired, idle or constructing).", []string{"state"}, nil)
	poolMax         = prometheus.NewDesc("slashobserve_db_pool_max_connections", "The most connections the database pool opens.", nil, nil)
	poolAcquires    = prometheus.NewDesc("slashobserve_db_pool_acquires_total", "Connections taken from the database pool.", nil, nil)
	poolEmpty       = prometheus.NewDesc("slashobserve_db_pool_empty_acquires_total", "Acquires that waited because the database pool had no idle connection.", nil, nil)
	poolWait        = prometheus.NewDesc("slashobserve_db_pool_acquire_wait_seconds_total", "Time spent waiting to acquire database pool connections.", nil, nil)
)

type poolCollector struct {
	stat func() *pgxpool.Stat
}

func (poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{poolConnections, poolMax, poolAcquires, poolEmpty, poolWait} {
		ch <- d
	}
}

func (c poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.stat()
	ch <- prometheus.MustNewConstMetric(poolConnections, prometheus.GaugeValue, float64(s.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(poolConnections, prometheus.GaugeValue, float64(s.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(poolConnections, prometheus.GaugeValue, float64(s.ConstructingConns()), "constructing")
	ch <- prometheus.MustNewConstMetric(poolMax, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(poolAcquires, prometheus.CounterValue, float64(s.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolEmpty, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolWait, prometheus.CounterValue, s.AcquireDuration().Seconds())
}
