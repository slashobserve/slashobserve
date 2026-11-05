package shopapp

import (
	"net/http"
	"sync"
	"testing"
)

const checkoutCart = `{"items":[{"sku":"p1","qty":1}]}`

func TestProductCacheHitsAbout85PercentAndMissesQueryTheDatabase(t *testing.T) {
	h := startApp(t, Config{Service: ServiceFrontend, Pod: "frontend-0", CheckoutURLs: []string{"http://127.0.0.1:1"}})

	const lookups = 1000
	slots := make(chan struct{}, 100)
	var wg sync.WaitGroup
	for range lookups {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			if rec := do(t, h, http.MethodGet, "/api/products/3", ""); rec.Code != http.StatusOK {
				t.Errorf("GET /api/products/3 = %d", rec.Code)
			}
		})
	}
	wg.Wait()

	hits := metricValue(t, h, "shop_product_cache_requests_total", `result="hit"`, `pod="frontend-0"`)
	misses := metricValue(t, h, "shop_product_cache_requests_total", `result="miss"`, `pod="frontend-0"`)
	if hits+misses != lookups {
		t.Fatalf("cache lookups = %v hits + %v misses, want %d", hits, misses, lookups)
	}
	if ratio := hits / lookups; ratio < 0.80 || ratio > 0.90 {
		t.Fatalf("cache hit ratio = %.3f, want 0.85 within 0.05", ratio)
	}
	if got := metricValue(t, h, "shop_db_query_duration_seconds_count", `query="get_product"`); got != misses {
		t.Fatalf("get_product queries = %v, want one per miss (%v)", got, misses)
	}
}

func TestPoolGaugesReportTheInventoryPool(t *testing.T) {
	cases := []struct {
		service Config
		exposes bool
	}{
		{Config{Service: ServiceFrontend, Pod: "frontend-0", CheckoutURLs: []string{"http://127.0.0.1:1"}}, true},
		{Config{Service: ServiceCheckout, Pod: "checkout-0", PaymentsURL: "http://127.0.0.1:1"}, true},
		{Config{Service: ServicePayments, Pod: "payments-0"}, false},
	}
	for _, tc := range cases {
		h := startApp(t, tc.service)
		body := do(t, h, http.MethodGet, "/metrics", "").Body.String()
		if !tc.exposes {
			for _, name := range []string{"shop_db_pool_in_use", "shop_db_pool_max"} {
				if hasLabels(body, []string{name}) {
					t.Errorf("%s exposes %s", tc.service.Pod, name)
				}
			}
			continue
		}
		pod := `pod="` + tc.service.Pod + `"`
		if got := metricValue(t, h, "shop_db_pool_max", pod, `service="`+tc.service.Service+`"`); got != dbPoolSize {
			t.Errorf("%s shop_db_pool_max = %v, want %d", tc.service.Pod, got, dbPoolSize)
		}
		if got := metricValue(t, h, "shop_db_pool_in_use", pod); got != 0 {
			t.Errorf("%s idle shop_db_pool_in_use = %v, want 0", tc.service.Pod, got)
		}
	}
}

func TestSlowDBFaultSlowsQueriesAndHoldsPoolSlotsUntilReset(t *testing.T) {
	h := startCheckout(t)
	rec := do(t, h, http.MethodPost, "/fault?type=slow_db&pct=100", "")
	if rec.Code != http.StatusOK || faultReport(t, rec)["slow_db_pct"] != 100.0 {
		t.Fatalf("POST slow_db fault: %d %s", rec.Code, rec.Body)
	}

	const checkouts = 3
	var wg sync.WaitGroup
	for range checkouts {
		wg.Go(func() { do(t, h, http.MethodPost, "/checkout", checkoutCart) })
	}
	eventually(t, "a slow query to hold a pool slot", func() bool {
		return metricValue(t, h, "shop_db_pool_in_use") >= 1
	})
	wg.Wait()

	reserve := `query="reserve_stock"`
	fast := `le="0.25"`
	if got := metricValue(t, h, "shop_db_pool_in_use"); got != 0 {
		t.Fatalf("pool in use after the checkouts returned = %v, want 0", got)
	}
	if got := metricValue(t, h, "shop_db_query_duration_seconds_count", reserve); got != checkouts {
		t.Fatalf("reserve_stock queries = %v, want %d", got, checkouts)
	}
	if got := metricValue(t, h, "shop_db_query_duration_seconds_bucket", reserve, fast); got != 0 {
		t.Fatalf("reserve_stock queries under 250 ms with slow_db at 100%% = %v, want 0", got)
	}

	if got := faultReport(t, do(t, h, http.MethodDelete, "/fault", "")); got["slow_db_pct"] != 0.0 {
		t.Fatalf("DELETE /fault left slow_db_pct = %v", got["slow_db_pct"])
	}
	do(t, h, http.MethodPost, "/checkout", checkoutCart)
	if got := metricValue(t, h, "shop_db_query_duration_seconds_bucket", reserve, fast); got != 1 {
		t.Fatalf("reserve_stock queries under 250 ms after the reset = %v, want 1", got)
	}
}
