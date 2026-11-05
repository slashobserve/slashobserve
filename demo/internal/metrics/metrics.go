package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	OrderOK     = "ok"
	OrderFailed = "failed"
)

var orderProcessingBuckets = []float64{0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 3, 5}

type Recorder struct {
	service         string
	pod             string
	registry        *prometheus.Registry
	requests        *prometheus.CounterVec
	duration        *prometheus.HistogramVec
	ordersProcessed *prometheus.CounterVec
	orderProcessing *prometheus.HistogramVec
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
	ordersProcessed := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "shop_orders_processed_total",
		Help: "Orders taken off the orders queue, by result.",
	}, []string{"service", "pod", "result"})
	orderProcessing := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "shop_order_processing_seconds",
		Help:    "Time a queue worker spends processing one order.",
		Buckets: orderProcessingBuckets,
	}, []string{"service", "pod"})
	registry.MustRegister(
		requests,
		duration,
		ordersProcessed,
		orderProcessing,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return &Recorder{
		service:         service,
		pod:             pod,
		registry:        registry,
		requests:        requests,
		duration:        duration,
		ordersProcessed: ordersProcessed,
		orderProcessing: orderProcessing,
	}
}

func (r *Recorder) Observe(route string, status int, elapsed time.Duration) {
	r.requests.WithLabelValues(r.service, route, strconv.Itoa(status), r.pod).Inc()
	r.duration.WithLabelValues(r.service, route, r.pod).Observe(elapsed.Seconds())
}

func (r *Recorder) WatchOrdersQueue(depth func() int, oldestAge func() time.Duration) {
	labels := r.podLabels()
	r.registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "shop_orders_queue_depth",
			Help:        "Orders waiting in the orders queue.",
			ConstLabels: labels,
		}, func() float64 { return float64(depth()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "shop_orders_queue_oldest_age_seconds",
			Help:        "Age of the oldest order waiting in the orders queue, 0 when it is empty.",
			ConstLabels: labels,
		}, func() float64 { return oldestAge().Seconds() }),
	)
	for _, result := range []string{OrderOK, OrderFailed} {
		r.ordersProcessed.WithLabelValues(r.service, r.pod, result)
	}
}

func (r *Recorder) OrderProcessed(result string, elapsed time.Duration) {
	r.ordersProcessed.WithLabelValues(r.service, r.pod, result).Inc()
	r.orderProcessing.WithLabelValues(r.service, r.pod).Observe(elapsed.Seconds())
}

func (r *Recorder) OrderDropped() {
	r.ordersProcessed.WithLabelValues(r.service, r.pod, OrderFailed).Inc()
}

func (r *Recorder) podLabels() prometheus.Labels {
	return prometheus.Labels{"service": r.service, "pod": r.pod}
}

func (r *Recorder) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}
