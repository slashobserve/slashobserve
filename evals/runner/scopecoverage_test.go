package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func fakePrometheus(t *testing.T, series map[string][]string) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/series" {
			http.NotFound(w, r)
			return
		}
		scope := r.URL.Query().Get("match[]")
		asked = append(asked, scope)
		names, ok := series[scope]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"parse error"}`)
			return
		}
		data := make([]map[string]string, 0, len(names))
		for _, name := range names {
			data = append(data, map[string]string{"__name__": name, "job": "shop"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

func coverageDashboard(queries ...string) *spec.Dashboard {
	panels := make([]spec.Panel, len(queries))
	for i, q := range queries {
		panels[i] = spec.Panel{ID: fmt.Sprintf("p%d", i), Type: spec.Timeseries, Datasource: "prom", Queries: []spec.Query{{Query: q}}}
	}
	return &spec.Dashboard{Panels: panels}
}

func coverageObservation(queries ...string) observation {
	return observation{dashboard: coverageDashboard(queries...), datasourceTypes: map[string]string{"prom": string(spec.Prometheus)}}
}

var shopSeries = []string{
	"http_requests_total", "http_requests_total",
	"http_request_duration_seconds_bucket", "http_request_duration_seconds_sum", "http_request_duration_seconds_count",
	"up", "scrape_duration_seconds", "process_cpu_seconds_total", "go_goroutines", "promhttp_metric_handler_requests_total",
}

func TestCoverageReadsTheSeriesNamesInScope(t *testing.T) {
	srv, asked := fakePrometheus(t, map[string][]string{`{job="shop"}`: shopSeries})
	names, err := seriesNames(context.Background(), srv.Client(), srv.URL+"/", `{job="shop"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 9 || names[0] != "http_requests_total" {
		t.Fatalf("names = %q, want each name once", names)
	}
	if !slices.Equal(*asked, []string{`{job="shop"}`}) {
		t.Fatalf("asked = %q", *asked)
	}
	if _, err := seriesNames(context.Background(), srv.Client(), srv.URL, `{job=`); err == nil || !strings.Contains(err.Error(), "prometheus answered 400: parse error") {
		t.Fatalf("a rejected selector must name the error, got %v", err)
	}
}

func TestCoverageGroupsAHistogramAndDropsScrapeAndRuntimeFamilies(t *testing.T) {
	families := metricFamilies(`{job="shop"}`, slices.Compact(slices.Clone(shopSeries)))
	if got := slices.Sorted(maps.Keys(families)); !slices.Equal(got, []string{"http_request_duration_seconds", "http_requests_total"}) {
		t.Fatalf("families = %v", families)
	}

	named := metricFamilies(`{__name__=~"go_.*|up", job="shop"}`, []string{"go_goroutines", "up", "process_open_fds", "orders_count"})
	for _, want := range []string{"go_goroutines", "up", "orders_count"} {
		if _, ok := named[want]; !ok {
			t.Errorf("%s must count when the scope names it or it is no histogram part: %v", want, named)
		}
	}
	if _, ok := named["process_open_fds"]; ok {
		t.Errorf("process_ stays excluded when the scope does not name it: %v", named)
	}
}

func TestCoverageScoresTheShareOfFamiliesNamedAsWholeTokens(t *testing.T) {
	names := []string{"http_requests_total", "http_request_duration_seconds_bucket", "http_request_duration_seconds_count", "orders_total", "cart_items"}
	obs := coverageObservation(
		`sum by (route) (rate(http_requests_total[5m]))`,
		`histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))`,
		"sum(rate(orders_total_created[5m])) # orders_total",
	)
	m := measureCoverage(`{job="shop"}`, names, obs)
	if m.covered != 2 || m.families != 4 || !slices.Equal(m.uncovered, []string{"cart_items", "orders_total"}) {
		t.Fatalf("measure = %+v", m)
	}
	if s := m.score(); s == nil || *s != 0.5 {
		t.Fatalf("score = %v", s)
	}

	c := checkCoverage(Expect{Coverage: 0.5}, observation{dashboard: obs.dashboard, coverage: &m})
	if !c.Passed {
		t.Fatalf("0.5 meets coverage 0.5: %s", c.Detail)
	}
	c = checkCoverage(Expect{Coverage: 0.9}, observation{dashboard: obs.dashboard, coverage: &m})
	want := `2 of 4 metric families in {job="shop"} are queried (0.50, need 0.90); not queried: cart_items, orders_total`
	if c.Passed || c.Detail != want {
		t.Fatalf("check = %+v, want detail %q", c, want)
	}
}

func TestCoverageCountsANativeHistogramByItsBaseName(t *testing.T) {
	names := []string{"latency_seconds_bucket", "latency_seconds_sum", "latency_seconds_count"}
	m := measureCoverage(`{job="shop"}`, names, coverageObservation("histogram_quantile(0.9, rate(latency_seconds[5m]))"))
	if m.covered != 1 || m.families != 1 {
		t.Fatalf("measure = %+v", m)
	}
}

func TestCoverageListsAtMostTwentyUncoveredFamilies(t *testing.T) {
	var names []string
	for i := range 25 {
		names = append(names, fmt.Sprintf("family_%02d", i))
	}
	m := measureCoverage(`{job="wide"}`, names, coverageObservation("sum(family_00)"))
	c := checkCoverage(Expect{Coverage: 0.9}, observation{dashboard: coverageDashboard("x"), coverage: &m})
	if !strings.HasSuffix(c.Detail, "family_20 and 4 more") || strings.Contains(c.Detail, "family_21") {
		t.Fatalf("detail = %s", c.Detail)
	}
}

func TestCoverageFailsWithoutFamiliesOrSeries(t *testing.T) {
	m := measureCoverage(`{job="shop"}`, []string{"up", "go_goroutines"}, coverageObservation("up"))
	if m.score() != nil {
		t.Fatalf("an empty family set has no score: %+v", m)
	}
	cases := map[string]observation{
		"no metric family matches the scope {job=\"shop\"}": {dashboard: coverageDashboard("up"), coverage: &m},
		"the series in scope were not read":                 {dashboard: coverageDashboard("up")},
		"no valid saved spec":                               {},
	}
	for want, obs := range cases {
		if c := checkCoverage(Expect{Scope: `{job="shop"}`, Coverage: 0.9}, obs); c.Passed || c.Detail != want {
			t.Errorf("check = %+v, want detail %q", c, want)
		}
	}
}

func TestCoverageRunsForAScopedCreateAgainstThePrometheusFlag(t *testing.T) {
	srv, _ := fakePrometheus(t, map[string][]string{`{job="shop"}`: shopSeries})
	sc := Scenario{ID: "red", Kind: kindCreate, Expect: Expect{Scope: `{job="shop"}`, Coverage: 0.9, MinPanels: 1, MaxSeconds: 60}}
	obs := coverageObservation("sum(rate(http_requests_total[5m]))", "histogram_quantile(0.9, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))")
	obs.request, obs.panels = okRequest(), map[string]panelOutcome{}
	e := &execution{c: &client{http: srv.Client()}, sc: sc, o: options{prometheus: srv.URL}, obs: obs}
	e.measureCoverage(context.Background())
	res := finish(sc, Pricing{}, Result{}, e.obs)
	if res.Coverage == nil || *res.Coverage != 1 {
		t.Fatalf("coverage = %v", res.Coverage)
	}
	idx := slices.IndexFunc(res.Checks, func(c CheckResult) bool { return c.Name == coverageCheckName })
	if idx < 0 || !res.Checks[idx].Passed {
		t.Fatalf("checks = %+v", res.Checks)
	}

	e.o.prometheus = "http://127.0.0.1:1"
	e.measureCoverage(context.Background())
	if c := checkCoverage(sc.Expect, e.obs); c.Passed || !strings.HasPrefix(c.Detail, "read the series in scope: ") {
		t.Fatalf("an unreachable Prometheus fails the check: %+v", c)
	}

	if checks := createChecks(Expect{MinPanels: 1}, obs); slices.ContainsFunc(checks, func(c CheckResult) bool { return c.Name == coverageCheckName }) {
		t.Fatal("an unscoped scenario has no coverage check")
	}
}

func TestCoverageScenarioDefaultsAndValidation(t *testing.T) {
	const create = "id: scoped\nprompt: Show the shop.\ncategory: prometheus\nkind: create\nsuites: [live]\nexpect:\n  datasources: [prometheus]\n"
	if sc := loadOne(t, create+"  scope: '{job=\"shop\"}'\n"); sc.Expect.Coverage != defaultCoverage {
		t.Fatalf("coverage = %v, want the default %v", sc.Expect.Coverage, defaultCoverage)
	}
	if sc := loadOne(t, create+"  scope: '{job=\"shop\"}'\n  coverage: 0.5\n"); sc.Expect.Coverage != 0.5 {
		t.Fatalf("coverage = %v, want 0.5", sc.Expect.Coverage)
	}
	cases := map[string]string{
		"needs expect.scope":                  create + "  coverage: 0.5\n",
		"above 0 and at most 1":               create + "  scope: '{job=\"shop\"}'\n  coverage: 1.5\n",
		"is not a Prometheus series selector": create + "  scope: '{job='\n",
		"applies to create scenarios only":    "id: scoped\nprompt: Add a panel.\nkind: revise\nsuites: [live]\nseed:\n  corpus: slashobserve/demo-shop.json\nexpect:\n  scope: '{job=\"shop\"}'\n",
	}
	for want, body := range cases {
		if _, err := writeScenarios(t, body); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v, want %q", err, want)
		}
	}
}

func TestCoverageFlagDefaultsToTheEvalsStack(t *testing.T) {
	o, err := parseFlags(nil, &strings.Builder{})
	if err != nil || o.prometheus != defaultPrometheus {
		t.Fatalf("prometheus = %q, %v", o.prometheus, err)
	}
}
