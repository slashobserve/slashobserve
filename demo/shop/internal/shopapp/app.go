package shopapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/slashobserve/slashobserve/demo/shop/internal/fault"
	"github.com/slashobserve/slashobserve/demo/shop/internal/metrics"
	"github.com/slashobserve/slashobserve/demo/shop/internal/prng"
)

const (
	ServiceFrontend = "frontend"
	ServiceCheckout = "checkout"
	ServicePayments = "payments"

	defaultAddr     = ":8080"
	upstreamTimeout = 3 * time.Second
)

type Config struct {
	Service      string
	Pod          string
	Addr         string
	CheckoutURLs []string
	PaymentsURL  string
}

func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	get := func(key string) string {
		value, _ := lookup(key)
		return strings.TrimSpace(value)
	}
	cfg := Config{
		Service:     get("SHOP_SERVICE"),
		Pod:         get("SHOP_POD"),
		Addr:        get("SHOP_ADDR"),
		PaymentsURL: strings.TrimRight(get("SHOP_PAYMENTS_URL"), "/"),
	}
	if cfg.Addr == "" {
		cfg.Addr = defaultAddr
	}
	for _, raw := range strings.Split(get("SHOP_CHECKOUT_URLS"), ",") {
		if raw = strings.TrimRight(strings.TrimSpace(raw), "/"); raw != "" {
			cfg.CheckoutURLs = append(cfg.CheckoutURLs, raw)
		}
	}

	switch cfg.Service {
	case ServiceFrontend:
		if len(cfg.CheckoutURLs) == 0 {
			return Config{}, errors.New("SHOP_CHECKOUT_URLS is required for the frontend service")
		}
	case ServiceCheckout:
		if cfg.PaymentsURL == "" {
			return Config{}, errors.New("SHOP_PAYMENTS_URL is required for the checkout service")
		}
	case ServicePayments:
	default:
		return Config{}, fmt.Errorf("SHOP_SERVICE must be %s, %s or %s, got %q",
			ServiceFrontend, ServiceCheckout, ServicePayments, cfg.Service)
	}
	if cfg.Pod == "" {
		return Config{}, errors.New("SHOP_POD is required")
	}
	return cfg, nil
}

type app struct {
	cfg       Config
	log       *slog.Logger
	rng       *prng.Source
	faults    fault.State
	recorder  *metrics.Recorder
	client    *http.Client
	rotation  atomic.Uint64
	alternate atomic.Uint64
	orderSeq  atomic.Int64
	mux       *http.ServeMux
}

func newApp(cfg Config, logger *slog.Logger) http.Handler {
	a := &app{
		cfg:      cfg,
		log:      logger,
		rng:      prng.ForPod(cfg.Pod),
		recorder: metrics.New(cfg.Service, cfg.Pod),
		client: &http.Client{
			Timeout:   upstreamTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		mux: http.NewServeMux(),
	}
	a.orderSeq.Store(1000)

	a.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	a.mux.Handle("GET /metrics", a.recorder.Handler())
	a.mux.HandleFunc("GET /fault", a.getFault)
	a.mux.HandleFunc("POST /fault", a.setFault)
	a.mux.HandleFunc("DELETE /fault", a.resetFault)

	switch cfg.Service {
	case ServiceFrontend:
		a.business("GET /api/products", a.listProducts)
		a.business("GET /api/products/{id}", a.getProduct)
		a.business("POST /api/checkout", a.frontendCheckout)
		a.business("GET /api/orders/{id}", a.frontendOrder)
	case ServiceCheckout:
		a.business("POST /checkout", a.checkout)
		a.business("GET /orders/{id}", a.getOrder)
	case ServicePayments:
		a.business("POST /charge", a.charge)
	}
	return a.mux
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
