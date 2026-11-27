package tools

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func quantileOf(q float64) *float64 { return &q }

func TestPlanAggregateBuildsTheWholeWindowQuery(t *testing.T) {
	window := 6 * time.Hour
	for _, tc := range []struct {
		name  string
		in    aggregateInput
		query string
		parts [2]string
	}{
		{
			"a counter's total",
			aggregateInput{Op: opTotal, Query: `http_requests_total{service_name="cart"}`},
			`sum by () (increase(http_requests_total{service_name="cart"}[21600s]))`, [2]string{},
		},
		{
			"a total per group",
			aggregateInput{Op: opTotal, Query: `http_requests_total`, By: []string{"service_name", "route"}},
			`sum by (service_name, route) (increase(http_requests_total[21600s]))`, [2]string{},
		},
		{
			"a ratio is 0 when the numerator is absent and the denominator is not",
			aggregateInput{Op: opRatio, Query: `http_requests_total{code=~"5.."}`, Denominator: `http_requests_total`, By: []string{"service_name"}},
			`(sum by (service_name) (increase(http_requests_total{code=~"5.."}[21600s])) or 0 * sum by (service_name) (increase(http_requests_total[21600s]))) / sum by (service_name) (increase(http_requests_total[21600s]))`,
			[2]string{`sum by (service_name) (increase(http_requests_total{code=~"5.."}[21600s]))`, `sum by (service_name) (increase(http_requests_total[21600s]))`},
		},
		{
			"a quantile over the window from the buckets' increase",
			aggregateInput{Op: opQuantile, Query: `http_server_duration_seconds{service_name="cart"}`, Quantile: quantileOf(0.99), By: []string{"route"}},
			`histogram_quantile(0.99, sum by (le, route) (increase(http_server_duration_seconds_bucket{service_name="cart"}[21600s])))`, [2]string{},
		},
		{
			"a quantile of a selector that already names the buckets",
			aggregateInput{Op: opQuantile, Query: `rpc_duration_bucket`, Quantile: quantileOf(0.5)},
			`histogram_quantile(0.5, sum by (le) (increase(rpc_duration_bucket[21600s])))`, [2]string{},
		},
	} {
		plan, problem := planAggregate(tc.in, spec.Prometheus, window)
		if problem != "" || plan.query != tc.query || plan.numerator != tc.parts[0] || plan.denominator != tc.parts[1] {
			t.Errorf("%s:\n got  %q %q %q (%s)\n want %q %q", tc.name, plan.query, plan.numerator, plan.denominator, problem, tc.query, tc.parts)
		}
	}
}

func TestPlanAggregateRefusesWhatItCannotMeasure(t *testing.T) {
	for name, in := range map[string]aggregateInput{
		"an unknown op":                  {Op: "p99", Query: "up"},
		"a ratio without a denominator":  {Op: opRatio, Query: "errors_total"},
		"a denominator on a total":       {Op: opTotal, Query: "errors_total", Denominator: "requests_total"},
		"a quantile without its value":   {Op: opQuantile, Query: "latency"},
		"a quantile above 1":             {Op: opQuantile, Query: "latency", Quantile: quantileOf(99)},
		"a quantile value on a total":    {Op: opTotal, Query: "errors_total", Quantile: quantileOf(0.9)},
		"an expression in place of one":  {Op: opTotal, Query: "rate(errors_total[5m])"},
		"a denominator expression":       {Op: opRatio, Query: "errors_total", Denominator: "sum(requests_total)"},
		"a group that is not a label":    {Op: opTotal, Query: "errors_total", By: []string{"service name"}},
		"a quantile grouped by le":       {Op: opQuantile, Query: "latency", Quantile: quantileOf(0.9), By: []string{"le"}},
		"more groups than it may take":   {Op: opTotal, Query: "errors_total", By: []string{"a", "b", "c", "d", "e", "f"}},
		"a selector that does not parse": {Op: opTotal, Query: `errors_total{`},
	} {
		if plan, problem := planAggregate(in, spec.Prometheus, time.Hour); problem == "" {
			t.Errorf("%s: planned %q, want a refusal", name, plan.query)
		}
	}
}

func aggregated(t *testing.T, r *Registry, input string) aggregateOut {
	t.Helper()
	res, err := call(t, r, nameAggregate, input)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	out, ok := res.Content.(aggregateOut)
	if !ok {
		t.Fatalf("content is %T", res.Content)
	}
	return out
}

func TestAggregateRatioCarriesItsPartsPerGroup(t *testing.T) {
	cart, shop := map[string]string{"service_name": "cart"}, map[string]string{"service_name": "shop"}
	fake := &measureFake{answer: func(query string) []datasource.Frame {
		switch {
		case strings.HasPrefix(query, "("):
			return []datasource.Frame{timeNumberFrame(cart, 0.025), timeNumberFrame(shop, 0)}
		case strings.Contains(query, `code=~"5.."`):
			return []datasource.Frame{timeNumberFrame(cart, 30)}
		}
		return []datasource.Frame{timeNumberFrame(cart, 1200), timeNumberFrame(shop, 400)}
	}}
	out := aggregated(t, fake.registry(t, ""), `{"datasource":"fake","op":"ratio","query":"http_requests_total{code=~\"5..\"}","denominator":"http_requests_total","by":["service_name"],"from":"2026-11-20T04:00:00Z","to":"2026-11-20T10:00:00Z"}`)

	if len(out.Values) != 2 || *out.Values[0].Value != 0.025 || *out.Values[0].Numerator != 30 || *out.Values[0].Denominator != 1200 {
		t.Fatalf("values = %+v", out.Values)
	}
	if *out.Values[1].Value != 0 || *out.Values[1].Numerator != 0 || *out.Values[1].Denominator != 400 {
		t.Errorf("an absent numerator should read 0 of 400: %+v", out.Values[1])
	}
	if len(fake.queries) != 3 || fake.queries[0] != out.Query || fake.queries[1] != out.NumeratorQuery || fake.queries[2] != out.DenominatorQuery {
		t.Errorf("queries = %q, out = %+v", fake.queries, out)
	}
	if !fake.ranges[0].To.Equal(time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("evaluated at %s, want the end of the window", fake.ranges[0].To)
	}
}

func TestAggregateMarksAnUndefinedQuantile(t *testing.T) {
	fake := &measureFake{answer: func(string) []datasource.Frame {
		return []datasource.Frame{timeNumberFrame(nil, math.NaN())}
	}}
	out := aggregated(t, fake.registry(t, ""), `{"datasource":"fake","op":"quantile","quantile":0.99,"query":"latency_seconds"}`)
	if len(out.Values) != 1 || !out.Values[0].Undefined || out.Values[0].Value != nil || len(fake.queries) != 1 {
		t.Fatalf("out = %+v queries = %q", out, fake.queries)
	}
}

func TestAggregateSaysAnEmptyResultIsNotAZero(t *testing.T) {
	fake := &measureFake{answer: func(string) []datasource.Frame { return nil }}
	out := aggregated(t, fake.registry(t, ""), `{"datasource":"fake","op":"total","query":"errors_total"}`)
	if !out.Empty || !strings.Contains(out.Note, "does not show the value was zero") || len(fake.queries) != 1 {
		t.Fatalf("out = %+v", out)
	}
}
