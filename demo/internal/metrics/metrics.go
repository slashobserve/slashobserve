package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Recorder struct {
	service  string
	pod      string
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func New(service, pod string) *Recorder {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests handled by business routes.",
	}, []string{"service", "route", "status", "pod"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration of business routes.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "route", "pod"})
	registry.MustRegister(
		requests,
		duration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return &Recorder{service: service, pod: pod, registry: registry, requests: requests, duration: duration}
}

func (r *Recorder) Observe(route string, status int, elapsed time.Duration) {
	r.requests.WithLabelValues(r.service, route, strconv.Itoa(status), r.pod).Inc()
	r.duration.WithLabelValues(r.service, route, r.pod).Observe(elapsed.Seconds())
}

func (r *Recorder) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}
