package shopapp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashobserve/slashobserve/demo/shop/internal/fault"
)

const (
	minFaultLatency = 500 * time.Millisecond
	maxFaultLatency = 1500 * time.Millisecond

	statusClientClosedRequest = 499
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (a *app) business(pattern string, handler http.HandlerFunc) {
	method, route, _ := strings.Cut(pattern, " ")
	chain := a.instrument(method, route, a.injectFaults(handler))
	a.mux.Handle(pattern, otelhttp.NewHandler(chain, pattern))
}

func (a *app) instrument(method, route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			if r.Context().Err() != nil {
				sw.WriteHeader(statusClientClosedRequest)
				trace.SpanFromContext(r.Context()).AddEvent("client canceled")
			} else {
				sw.status = http.StatusOK
			}
		}
		elapsed := time.Since(start)
		a.recorder.Observe(route, sw.status, elapsed)
		a.log.InfoContext(r.Context(), "request handled",
			"http.request.method", method,
			"http.route", route,
			"http.response.status_code", sw.status,
			"duration_ms", elapsed.Milliseconds(),
		)
	})
}

func (a *app) injectFaults(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.rng.Percent(a.faults.Pct(fault.Latency)) {
			if !sleep(r.Context(), a.rng.Between(minFaultLatency, maxFaultLatency)) {
				return
			}
		}
		if a.rng.Percent(a.faults.Pct(fault.Errors)) {
			a.log.ErrorContext(r.Context(), "database connection refused")
			writeError(w, http.StatusInternalServerError, "database connection refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *app) work(ctx context.Context, name string, lo, hi time.Duration) (context.Context, trace.Span, bool) {
	ctx, span := otel.Tracer("shop").Start(ctx, name)
	ok := sleep(ctx, a.rng.Between(lo, hi))
	return ctx, span, ok
}

func failSpan(span trace.Span, message string) {
	span.SetStatus(codes.Error, message)
}
