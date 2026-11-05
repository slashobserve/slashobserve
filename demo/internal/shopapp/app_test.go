package shopapp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func startApp(t *testing.T, cfg Config) http.Handler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if cfg.QueueWorkers == 0 {
		cfg.QueueWorkers = defaultQueueWorkers
	}
	return newApp(ctx, cfg, slog.New(slog.DiscardHandler))
}

func startCheckout(t *testing.T) http.Handler {
	t.Helper()
	payments := httptest.NewServer(startApp(t, Config{Service: ServicePayments, Pod: "payments-0"}))
	t.Cleanup(payments.Close)
	return startApp(t, Config{Service: ServiceCheckout, Pod: "checkout-0", PaymentsURL: payments.URL})
}

func do(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func faultReport(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var report map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("fault response %q: %v", rec.Body.String(), err)
	}
	return report
}

func metricValue(t *testing.T, h http.Handler, name string, labels ...string) float64 {
	t.Helper()
	body, err := io.ReadAll(do(t, h, http.MethodGet, "/metrics", "").Body)
	if err != nil {
		t.Fatal(err)
	}
	total, found := 0.0, false
	for line := range strings.SplitSeq(string(body), "\n") {
		series, raw, ok := strings.Cut(line, " ")
		if !ok || (series != name && !strings.HasPrefix(series, name+"{")) {
			continue
		}
		if !hasLabels(series, labels) {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			t.Fatalf("metric line %q: %v", line, err)
		}
		total += value
		found = true
	}
	if !found {
		t.Fatalf("no %s series with labels %v in /metrics", name, labels)
	}
	return total
}

func hasLabels(series string, labels []string) bool {
	for _, label := range labels {
		if !strings.Contains(series, label) {
			return false
		}
	}
	return true
}

func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestQueueWorkersFromEnv(t *testing.T) {
	cases := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"", defaultQueueWorkers, false},
		{"1", 1, false},
		{"100", 100, false},
		{"0", 0, true},
		{"101", 0, true},
		{"four", 0, true},
	}
	for _, tc := range cases {
		env := map[string]string{"SHOP_SERVICE": ServicePayments, "SHOP_POD": "payments-0", "SHOP_QUEUE_WORKERS": tc.raw}
		cfg, err := ConfigFromEnv(func(key string) (string, bool) { v, ok := env[key]; return v, ok })
		if tc.wantErr {
			if err == nil {
				t.Errorf("SHOP_QUEUE_WORKERS=%q: want an error, got %d workers", tc.raw, cfg.QueueWorkers)
			}
			continue
		}
		if err != nil || cfg.QueueWorkers != tc.want {
			t.Errorf("SHOP_QUEUE_WORKERS=%q: got %d, %v; want %d", tc.raw, cfg.QueueWorkers, err, tc.want)
		}
	}
}

func TestQueueFaultIsSetReportedAndReset(t *testing.T) {
	h := startCheckout(t)

	if rec := do(t, h, http.MethodPost, "/fault?type=queue&pct=40", ""); rec.Code != http.StatusOK || faultReport(t, rec)["queue_pct"] != 40.0 {
		t.Fatalf("POST queue fault: %d %s", rec.Code, rec.Body)
	}
	got := faultReport(t, do(t, h, http.MethodGet, "/fault", ""))
	if got["queue_pct"] != 40.0 || got["errors_pct"] != 0.0 || got["latency_pct"] != 0.0 {
		t.Fatalf("GET /fault after the queue fault = %v", got)
	}
	if got := faultReport(t, do(t, h, http.MethodDelete, "/fault", "")); got["queue_pct"] != 0.0 {
		t.Fatalf("DELETE /fault left queue_pct = %v", got["queue_pct"])
	}
}

func TestQueueFaultBacksUpOrdersUntilReset(t *testing.T) {
	h := startCheckout(t)
	do(t, h, http.MethodPost, "/fault?type=queue&pct=100", "")

	created := 0
	for range 4 {
		rec := do(t, h, http.MethodPost, "/checkout", `{"items":[{"sku":"p1","qty":1}]}`)
		if rec.Code == http.StatusCreated {
			created++
		}
	}
	if created == 0 {
		t.Fatal("no checkout created an order")
	}
	depth := `pod="checkout-0"`
	if got := metricValue(t, h, "shop_orders_queue_depth", depth); got != float64(created) {
		t.Fatalf("queue depth under the queue fault = %v, want %d", got, created)
	}
	if got := metricValue(t, h, "shop_orders_queue_oldest_age_seconds", depth); got <= 0 {
		t.Fatalf("oldest age under the queue fault = %v, want above 0", got)
	}

	do(t, h, http.MethodDelete, "/fault", "")
	eventually(t, "the orders queue to drain", func() bool {
		return metricValue(t, h, "shop_orders_queue_depth") == 0 &&
			metricValue(t, h, "shop_orders_processed_total") == float64(created)
	})
	if got := metricValue(t, h, "shop_orders_queue_oldest_age_seconds"); got != 0 {
		t.Fatalf("oldest age after the queue drained = %v, want 0", got)
	}
	if got := metricValue(t, h, "shop_order_processing_seconds_count", `service="checkout"`); got != float64(created) {
		t.Fatalf("order processing observations = %v, want %d", got, created)
	}
}
