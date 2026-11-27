package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestEmptySelectorsFindEverySelectorOfAQuery(t *testing.T) {
	for _, tc := range []struct {
		store spec.DatasourceType
		query string
		want  []string
	}{
		{spec.Prometheus, `sum(rate(http_requests_total{service="cart",code=~"5.."}[5m])) / sum(rate(http_requests_total{service="cart"}[5m]))`, []string{`http_requests_total{code=~"5..",service="cart"}`, `http_requests_total{service="cart"}`}},
		{spec.Prometheus, `max_over_time((sum(queue_depth))[21600s:60s])`, []string{`queue_depth`}},
		{spec.Prometheus, `up{job=`, nil},
		{spec.Loki, `sum by (pod) (count_over_time({service_name="cart"} |= "{not a selector}" | json | line_format "{{.msg}}" [5m]))`, []string{`{service_name="cart"}`}},
		{spec.Loki, `{app="shop", level=~"error|warn"} | json`, []string{`{app="shop", level=~"error|warn"}`}},
		{spec.ClickHouse, `SELECT 1`, nil},
	} {
		var got []string
		for _, sel := range emptySelectors(tc.store, tc.query) {
			got = append(got, sel.text)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %q:\n got  %q\n want %q", tc.store, tc.query, got, tc.want)
		}
	}
}

func TestClosestNamesTheNearestKeysAndValues(t *testing.T) {
	for _, tc := range []struct {
		target     string
		candidates []string
		want       []string
	}{
		{"service", []string{"pod", "job", "service_name", "instance"}, []string{"service_name", "job"}},
		{"servce_name", []string{"namespace", "service_name"}, []string{"service_name"}},
		{"checkout", []string{"cart", "checkoutservice", "checkout-v2", "frontend"}, []string{"checkout-v2", "checkoutservice"}},
		{"http_server_duration_seconds", []string{"http_server_request_duration_seconds_bucket", "go_gc_duration_seconds", "process_cpu_seconds_total"}, []string{"http_server_request_duration_seconds_bucket", "go_gc_duration_seconds"}},
		{"payments", []string{"cart", "frontend"}, []string{}},
		{"/api/v9", []string{"/", "/api/v1/read", "/metrics"}, []string{"/api/v1/read"}},
	} {
		if got := closest(tc.target, tc.candidates, maxCloseNames); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("closest(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func read(names ...string) nameRead { return nameRead{done: true, names: names} }

func onlySelector(t *testing.T, store spec.DatasourceType, query string) emptySelector {
	t.Helper()
	sels := emptySelectors(store, query)
	if len(sels) != 1 {
		t.Fatalf("%q has %d selectors", query, len(sels))
	}
	return sels[0]
}

func TestDiagnoseSelectorSaysWhyNothingMatched(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store spec.DatasourceType
		query string
		facts emptyFacts
		want  []string
	}{
		{
			"a key the metric does not carry, with the nearest ones",
			spec.Prometheus, `http_requests_total{service="cart"}`,
			emptyFacts{keys: read("code", "job", "service_name"), keysEver: read("code", "job", "service_name")},
			[]string{`Label key service does not exist on metric http_requests_total`, `"service_name", "job"`},
		},
		{
			"a value the key does not take, with close ones",
			spec.Prometheus, `http_requests_total{service_name="checkout"}`,
			emptyFacts{keys: read("service_name"), values: map[string]nameRead{"service_name": read("cart", "checkoutservice")}, ever: map[string]nameRead{"service_name": read("cart", "checkoutservice")}},
			[]string{`service_name="checkout" matches no series of metric http_requests_total in this window`, `"checkoutservice"`},
		},
		{
			"a series with no samples in the window",
			spec.Prometheus, `queue_depth{queue="orders"}`,
			emptyFacts{keys: read(), keysEver: read("queue")},
			[]string{"Metric queue_depth has series in this datasource, but none with samples in this window"},
		},
		{
			"a value seen only around the window",
			spec.Prometheus, `queue_depth{queue="orders"}`,
			emptyFacts{keys: read("queue"), values: map[string]nameRead{"queue": read("billing")}, ever: map[string]nameRead{"queue": read("billing", "orders")}},
			[]string{`queue="orders" matches series of metric queue_depth in the 7 days around this window, but none with samples in it`},
		},
		{
			"a metric that does not exist",
			spec.Prometheus, `http_server_duration_seconds_bucket{route="/cart"}`,
			emptyFacts{keys: read(), keysEver: read(), metrics: read("http_server_request_duration_seconds_bucket", "http_client_requests_total")},
			[]string{"No metric named http_server_duration_seconds_bucket exists in this datasource", `"http_server_request_duration_seconds_bucket"`},
		},
		{
			"a stream key that is a field, not a stream label",
			spec.Loki, `{service_name="cart", level="error"}`,
			emptyFacts{keys: read("namespace", "service_name"), keysEver: read("namespace", "service_name"), values: map[string]nameRead{"service_name": read("cart")}},
			[]string{"Label key level does not exist on any stream", "| level=\"error\""},
		},
		{
			"a pipeline that dropped every line",
			spec.Loki, `{service_name="cart"} |= "timeout"`,
			emptyFacts{keys: read("service_name"), values: map[string]nameRead{"service_name": read("cart")}, matches: read("120")},
			[]string{`120 lines of stream selector {service_name="cart"} fall in this window, so the line filters`},
		},
		{
			"a stream selector with no line in the window",
			spec.Loki, `{service_name="cart", pod="a"} |= "timeout"`,
			emptyFacts{keys: read("pod", "service_name"), values: map[string]nameRead{"service_name": read("cart"), "pod": read("a", "b")}, matches: read()},
			[]string{`Stream selector {service_name="cart", pod="a"} matches no line in this window`},
		},
	} {
		d := diagnoseSelector(tc.store, onlySelector(t, tc.store, tc.query), tc.facts)
		if d.need.kind != factNone {
			t.Errorf("%s: still needs %+v", tc.name, d.need)
			continue
		}
		for _, want := range tc.want {
			if !strings.Contains(d.note, want) {
				t.Errorf("%s:\n note %q\n lacks %q", tc.name, d.note, want)
			}
		}
	}
}

func TestDiagnoseSelectorAsksForOneFactAtATime(t *testing.T) {
	sel := onlySelector(t, spec.Prometheus, `http_requests_total{service_name="cart",code!="200"}`)
	f := emptyFacts{values: map[string]nameRead{}, ever: map[string]nameRead{}}
	var asked []factNeed
	for range 5 {
		d := diagnoseSelector(spec.Prometheus, sel, f)
		if d.need.kind == factNone {
			if !d.allExists || d.note != "" {
				t.Fatalf("diagnosis = %+v, want every label to exist", d)
			}
			break
		}
		asked = append(asked, d.need)
		f.set(d.need, read("service_name", "code", "cart"))
	}
	want := []factNeed{{kind: factKeys}, {kind: factValues, key: "service_name"}}
	if !reflect.DeepEqual(asked, want) {
		t.Fatalf("asked %+v, want %+v", asked, want)
	}
}

func TestDiagnoseSelectorSaysNothingWhenItCannotRead(t *testing.T) {
	sel := onlySelector(t, spec.Prometheus, `up{job="x"}`)
	failed := nameRead{done: true, failed: true}
	for name, f := range map[string]emptyFacts{
		"no keys":            {keys: failed},
		"no values":          {keys: read("job"), values: map[string]nameRead{"job": failed}},
		"too many values":    {keys: read("job"), values: map[string]nameRead{"job": {done: true, truncated: true, names: []string{"a"}}}},
		"no wider key check": {keys: read(), keysEver: failed},
	} {
		if d := diagnoseSelector(spec.Prometheus, sel, f); d.note != "" || d.need.kind != factNone {
			t.Errorf("%s: diagnosis = %+v, want none", name, d)
		}
	}
}

func TestEmptyQueryNamesTheLabelKeyTheStoreUses(t *testing.T) {
	ds := &fakeDS{discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		if req.Kind == datasource.DiscoverLabels && req.Metric == "http_requests_total" {
			return &datasource.Catalog{Labels: []string{"code", "job", "service_name"}}, nil
		}
		return &datasource.Catalog{}, nil
	}}
	res, err := call(t, newTestRegistry(t, ds, nil), nameQueryInstant, `{"datasource":"fake","query":"sum(rate(http_requests_total{service=\"cart\"}[5m]))","from":"2026-11-20T09:00:00Z","to":"2026-11-20T10:00:00Z"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	out := res.Content.(querySummary)
	if !out.Empty || !strings.Contains(out.Note, `Label key service does not exist on metric http_requests_total in this window; the closest keys it has are "service_name", "job"`) {
		t.Fatalf("note = %q", out.Note)
	}
	if len(ds.requests) != 2 || !ds.requests[0].Range.To.Equal(time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("requests = %+v, want the window's keys then the keys around it", ds.requests)
	}
}

func TestEmptyMeasureSaysTheMetricDoesNotExist(t *testing.T) {
	ds := &fakeDS{discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		if req.Kind == datasource.DiscoverLabelValues && req.Label == metricNameLabel {
			return &datasource.Catalog{Values: []string{"queue_depth_items", "queue_lag_seconds"}}, nil
		}
		return &datasource.Catalog{}, nil
	}}
	out := measured(t, newTestRegistry(t, ds, nil), `{"datasource":"fake","query":"queue_depth","stat":"max"}`)
	if !strings.HasPrefix(out.Note, `No metric named queue_depth exists in this datasource; the closest names are "queue_depth_items"`) || !strings.Contains(out.Note, "does not show the value was zero") {
		t.Fatalf("note = %q", out.Note)
	}
	if len(ds.requests) != 3 || ds.requests[2].Search != "queue" {
		t.Errorf("requests = %+v", ds.requests)
	}
}

func TestEmptyDiagnosisStaysWithinItsCallsAndNeverFailsTheRead(t *testing.T) {
	ds := &fakeDS{discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "down")
	}}
	res, err := call(t, newTestRegistry(t, ds, nil), nameQueryRange, `{"datasource":"fake","query":"a{x=\"1\"} + b{y=\"2\"} + c{z=\"3\"}"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	if out := res.Content.(querySummary); !out.Empty || out.Note != "" {
		t.Errorf("note = %q, want none when discovery fails", out.Note)
	}
	if len(ds.requests) > maxDiagnosisCalls {
		t.Errorf("%d discovery calls, at most %d allowed", len(ds.requests), maxDiagnosisCalls)
	}
}
