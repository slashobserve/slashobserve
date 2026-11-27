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
		if _, err := selectorParser.ParseExpr(plan.query); err != nil {
			t.Errorf("%s: %q is not PromQL: %v", tc.name, plan.query, err)
		}
	}
}

func TestPlanAggregateBuildsTheWholeWindowLogQL(t *testing.T) {
	window := time.Hour
	for _, tc := range []struct {
		name  string
		in    aggregateInput
		query string
	}{
		{
			"the lines a log query matches",
			aggregateInput{Op: opTotal, Query: `{service_name="cart"} |= "error"`},
			`sum(count_over_time({service_name="cart"} |= "error" [3600s]))`,
		},
		{
			"lines per group",
			aggregateInput{Op: opTotal, Query: `{service_name=~".+"}`, By: []string{"service_name"}},
			`sum by (service_name) (count_over_time({service_name=~".+"} [3600s]))`,
		},
		{
			"the sum of a field",
			aggregateInput{Op: opTotal, Query: `{service_name="cart"} | json`, Unwrap: "bytes(size)"},
			`sum(sum_over_time({service_name="cart"} | json | unwrap bytes(size) | __error__="" [3600s]))`,
		},
		{
			"a quantile of a field over every line",
			aggregateInput{Op: opQuantile, Query: `{service_name="cart"} | json`, Unwrap: "ms", Quantile: quantileOf(0.99)},
			`quantile_over_time(0.99, {service_name="cart"} | json | unwrap ms | __error__="" [3600s]) by ()`,
		},
		{
			"a peak of a field per group",
			aggregateInput{Op: opMax, Query: `{namespace="shop"} | logfmt`, Unwrap: "duration( took )", By: []string{"service_name"}},
			`max_over_time({namespace="shop"} | logfmt | unwrap duration(took) | __error__="" [3600s]) by (service_name)`,
		},
	} {
		plan, problem := planAggregate(tc.in, spec.Loki, window)
		if problem != "" || plan.query != tc.query {
			t.Errorf("%s:\n got  %q (%s)\n want %q", tc.name, plan.query, problem, tc.query)
		}
	}
}

func TestPlanAggregateGivesALokiRatioThatIsZeroWithoutMatches(t *testing.T) {
	plan, problem := planAggregate(aggregateInput{Op: opRatio, Query: `{service_name="cart"} |= "error"`, Denominator: `{service_name="cart"}`}, spec.Loki, time.Hour)
	num, den := `sum(count_over_time({service_name="cart"} |= "error" [3600s]))`, `sum(count_over_time({service_name="cart"} [3600s]))`
	if problem != "" || plan.query != "("+num+" or 0 * "+den+") / "+den || plan.numerator != num || plan.denominator != den {
		t.Fatalf("plan = %+v (%s)", plan, problem)
	}
}

func TestPlanAggregateRefusesWhatLokiCannotMeasure(t *testing.T) {
	for name, in := range map[string]aggregateInput{
		"a metric query":               {Op: opTotal, Query: `count_over_time({a="b"}[5m])`},
		"an unwrap stage in the query": {Op: opMax, Query: `{a="b"} | unwrap ms`},
		"a peak without a field":       {Op: opMax, Query: `{a="b"}`},
		"a quantile without a field":   {Op: opQuantile, Query: `{a="b"}`, Quantile: quantileOf(0.5)},
		"a field that is not a name":   {Op: opMax, Query: `{a="b"}`, Unwrap: "ms | line_format"},
		"a ratio of a field":           {Op: opRatio, Query: `{a="b"}`, Denominator: `{a=~".+"}`, Unwrap: "ms"},
		"a denominator metric query":   {Op: opRatio, Query: `{a="b"}`, Denominator: `rate({a="b"}[1m])`},
	} {
		if plan, problem := planAggregate(in, spec.Loki, time.Hour); problem == "" {
			t.Errorf("%s: planned %q, want a refusal", name, plan.query)
		}
	}
	for name, in := range map[string]aggregateInput{
		"a peak on Prometheus":    {Op: opMax, Query: "up"},
		"unwrap on Prometheus":    {Op: opTotal, Query: "up", Unwrap: "ms"},
		"another datasource type": {Op: opTotal, Query: "up"},
	} {
		dsType := spec.Prometheus
		if name == "another datasource type" {
			dsType = spec.Tempo
		}
		if plan, problem := planAggregate(in, dsType, time.Hour); problem == "" {
			t.Errorf("%s: planned %q, want a refusal", name, plan.query)
		}
	}
}

func TestAggregateCountsLokiLinesAtTheWindowEnd(t *testing.T) {
	fake := &measureFake{answer: func(string) []datasource.Frame {
		return []datasource.Frame{timeNumberFrame(nil, 4)}
	}}
	out := aggregated(t, fake.registry(t, "loki"), `{"datasource":"fake","op":"total","query":"{service_name=\"cart\"} |= \"error\"","from":"2026-11-20T09:00:00Z","to":"2026-11-20T10:00:00Z"}`)
	if len(out.Values) != 1 || *out.Values[0].Value != 4 || fake.queries[0] != `sum(count_over_time({service_name="cart"} |= "error" [3600s]))` {
		t.Fatalf("out = %+v queries = %q", out, fake.queries)
	}
	if !fake.ranges[0].To.Equal(time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("evaluated at %s, want the end of the window", fake.ranges[0].To)
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

func TestAggregateQuantileFallsBackToANativeHistogram(t *testing.T) {
	fake := &measureFake{answer: func(query string) []datasource.Frame {
		if strings.Contains(query, bucketSuffix) {
			return nil
		}
		return []datasource.Frame{timeNumberFrame(map[string]string{"route": "/cart"}, 0.42)}
	}}
	out := aggregated(t, fake.registry(t, ""), `{"datasource":"fake","op":"quantile","quantile":0.99,"query":"http_server_duration_seconds","by":["route"]}`)
	want := `histogram_quantile(0.99, sum by (route) (increase(http_server_duration_seconds[3600s])))`
	if out.Empty || out.Query != want || *out.Values[0].Value != 0.42 || len(fake.queries) != 2 {
		t.Fatalf("out = %+v queries = %q", out, fake.queries)
	}
}
