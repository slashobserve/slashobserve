package proxyhttp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
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

func TestNamesRequestParsing(t *testing.T) {
	odd := "\xff\xfe"
	nul := "a\x00b"
	tests := []struct {
		name    string
		raw     string
		allowed []string
		want    queryproxy.NamesRequest
		message string
	}{
		{name: "empty query", raw: "", allowed: labelsParams},
		{name: "every labels parameter", raw: "metric=m&table=t&search=s&limit=7", allowed: labelsParams, want: queryproxy.NamesRequest{Metric: "m", Table: "t", Search: "s", Limit: 7}},
		{name: "invalid UTF-8 in search is passed through", raw: "search=" + url.QueryEscape(odd), allowed: metricsParams, want: queryproxy.NamesRequest{Search: odd}},
		{name: "NUL in search is passed through", raw: "search=a%00b", allowed: metricsParams, want: queryproxy.NamesRequest{Search: nul}},
		{name: "invalid UTF-8 in metric is passed through", raw: "metric=%ff%fe", allowed: labelsParams, want: queryproxy.NamesRequest{Metric: odd}},
		{name: "NUL in metric is passed through", raw: "metric=a%00b", allowed: labelsParams, want: queryproxy.NamesRequest{Metric: nul}},
		{name: "invalid UTF-8 in table is passed through", raw: "table=%ff%fe", allowed: labelsParams, want: queryproxy.NamesRequest{Table: odd}},
		{name: "NUL in table is passed through", raw: "table=a%00b", allowed: labelsParams, want: queryproxy.NamesRequest{Table: nul}},
		{name: "metric is unknown for the metrics route", raw: "metric=m", allowed: metricsParams, message: "unknown query parameter"},
		{name: "table is unknown for the metrics route", raw: "table=t", allowed: metricsParams, message: "unknown query parameter"},
		{name: "label is a path parameter only", raw: "label=l", allowed: labelsParams, message: "unknown query parameter"},
		{name: "repeated search", raw: "search=a&search=b", allowed: metricsParams, message: "search must be given once"},
		{name: "repeated and empty", raw: "search=&search=", allowed: metricsParams, message: "search must be given once"},
		{name: "malformed query string", raw: "search=%zz", allowed: metricsParams, message: "the query string is malformed"},
		{name: "limit 500", raw: "limit=500", allowed: metricsParams, want: queryproxy.NamesRequest{Limit: 500}},
		{name: "limit 0", raw: "limit=0", allowed: metricsParams, message: "limit must be between 1 and 500"},
		{name: "limit -1", raw: "limit=-1", allowed: metricsParams, message: "limit must be between 1 and 500"},
		{name: "limit 501", raw: "limit=501", allowed: metricsParams, message: "limit must be between 1 and 500"},
		{name: "limit with a plus sign", raw: "limit=%2B5", allowed: metricsParams, message: "limit must be between 1 and 500"},
		{name: "limit with a leading zero", raw: "limit=05", allowed: metricsParams, message: "limit must be between 1 and 500"},
		{name: "limit that is not a number", raw: "limit=abc", allowed: metricsParams, message: "limit must be between 1 and 500"},
		{name: "empty limit", raw: "limit=", allowed: metricsParams, message: "limit must be between 1 and 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, message := namesRequest(tt.raw, tt.allowed)
			if message != tt.message {
				t.Fatalf("message = %q, want %q", message, tt.message)
			}
			if message == "" && got != tt.want {
				t.Fatalf("request = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNamesRoutesPassOddBytesToTheDatasourceAndAnswerEmptyLists(t *testing.T) {
	hn := newHarness(t)
	tests := []struct {
		name, path string
		check      func(t *testing.T, req datasource.DiscoverRequest)
	}{
		{"search with invalid UTF-8", "/api/v1/datasources/prom/metrics?search=%ff%fe", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Search, "\xff\xfe") }},
		{"search with NUL", "/api/v1/datasources/prom/metrics?search=a%00b", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Search, "a\x00b") }},
		{"metric with invalid UTF-8", "/api/v1/datasources/prom/labels?metric=%ff", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Metric, "\xff") }},
		{"metric with NUL", "/api/v1/datasources/prom/labels?metric=a%00b", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Metric, "a\x00b") }},
		{"table with invalid UTF-8", "/api/v1/datasources/prom/labels?table=%ff", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Table, "\xff") }},
		{"table with NUL", "/api/v1/datasources/prom/labels?table=a%00b", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Table, "a\x00b") }},
		{"label with invalid UTF-8", "/api/v1/datasources/prom/labels/%ff/values", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Label, "\xff") }},
		{"label with NUL", "/api/v1/datasources/prom/labels/a%00b/values", func(t *testing.T, r datasource.DiscoverRequest) { eq(t, r.Label, "a\x00b") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(hn.ds.discoverRequests())
			rec := hn.get(tt.path, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", rec.Body.String(), err)
			}
			for _, key := range []string{"metrics", "labels", "values"} {
				if raw, ok := body[key]; ok && string(raw) != "[]" {
					t.Fatalf("%s = %s, want []", key, raw)
				}
			}
			reqs := hn.ds.discoverRequests()
			if len(reqs) != before+1 {
				t.Fatalf("discover calls = %d, want %d", len(reqs), before+1)
			}
			tt.check(t, reqs[len(reqs)-1])
		})
	}
}

func eq(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
