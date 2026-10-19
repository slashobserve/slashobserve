package proxyhttp

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestNamesRequestReportsTheSameMessageForTheSameBadQuery(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		allowed []string
		want    string
	}{
		{"several unknown parameters", "zeta=1&alpha=2&mid=3&" + strings.Repeat("x", 500) + "=4", metricsParams, "unknown query parameter"},
		{"several repeated parameters", "search=a&search=b&limit=1&limit=2", metricsParams, "limit must be given once"},
		{"unknown sorts before repeated", "search=a&search=b&zzz=1", metricsParams, "search must be given once"},
		{"repeated sorts before unknown", "aaa=1&search=a&search=b", metricsParams, "unknown query parameter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 50 {
				_, message := namesRequest(tt.raw, tt.allowed)
				if message != tt.want {
					t.Fatalf("message = %q, want %q", message, tt.want)
				}
			}
		})
	}
}

func TestNamesRouteAnswersBadParametersWithTheSharedWording(t *testing.T) {
	hn := newHarness(t)
	for path, want := range map[string]string{
		"/api/v1/datasources/prom/metrics?bogus=1&other=2":     "unknown query parameter",
		"/api/v1/datasources/prom/metrics?search=a&search=b":   "search must be given once",
		"/api/v1/datasources/prom/labels?table=t&table=u":      "table must be given once",
		"/api/v1/datasources/prom/labels/service/values?x=%ff": "unknown query parameter",
	} {
		rec := hn.get(path, "")
		expectError(t, rec, http.StatusBadRequest, "invalid_request")
		if got := decodeError(t, rec).Error; got != want {
			t.Fatalf("%s: error = %q, want %q", path, got, want)
		}
	}
}

func TestNamesRoutesAnswer429WhenEveryUpstreamSlotStaysHeld(t *testing.T) {
	hn := newHarness(t, queryproxy.WithUpstreamWait(30*time.Millisecond))
	holdUpstreamSlots(t, hn)

	for _, path := range []string{
		"/api/v1/datasources/prom/metrics",
		"/api/v1/datasources/prom/labels",
		"/api/v1/datasources/prom/labels/service/values",
	} {
		start := time.Now()
		rec := hn.get(path, "")
		expectError(t, rec, http.StatusTooManyRequests, "too_many_requests")
		if got := rec.Header().Get("Retry-After"); got != "1" {
			t.Fatalf("%s: Retry-After = %q, want 1", path, got)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("%s: answered after %v", path, elapsed)
		}
	}
	if held := hn.heldSlots(); held != 0 {
		t.Fatalf("%d principal slots leaked", held)
	}
}
