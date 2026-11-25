package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func promCatalog(metrics []datasource.Metric, truncated bool) func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		switch req.Kind {
		case datasource.DiscoverLabels:
			return &datasource.Catalog{Labels: []string{"instance", "job", "service"}}, nil
		case datasource.DiscoverMetrics:
			var found []datasource.Metric
			for _, m := range metrics {
				if req.Search == "" || strings.Contains(m.Name, req.Search) {
					found = append(found, m)
				}
			}
			return &datasource.Catalog{Metrics: found, Truncated: truncated && req.Search == ""}, nil
		}
		return &datasource.Catalog{Values: []string{"checkout", "payments"}}, nil
	}
}

var promMetrics = []datasource.Metric{
	{Name: "http_requests_total", Type: "counter"},
	{Name: "http_request_duration_seconds_bucket", Type: "histogram"},
	{Name: "process_cpu_seconds_total", Type: "counter"},
}

func promDraftDS(truncated bool) *fakeDS {
	return &fakeDS{typ: "prometheus", instantFn: oneRow, discoverFn: promCatalog(promMetrics, truncated)}
}

func TestDraftContextPrometheusListsEveryMetricWhenTheCatalogIsSmall(t *testing.T) {
	ds := promDraftDS(false)
	facts, err := contextFor(t, ds, signalMetrics)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"datasource: ds (prometheus), signal: metrics\n",
		"answer: a PromQL query in submit_query's query; it is evaluated at the end of the time range.\n",
		"labels: instance, job, service\n",
		"metrics: http_requests_total counter, http_request_duration_seconds_bucket histogram, process_cpu_seconds_total counter\n",
		"values of service: \"checkout\", \"payments\"\n",
	} {
		if !strings.Contains(facts.Text, want) {
			t.Errorf("facts lack %q:\n%s", want, facts.Text)
		}
	}
	if facts.Reads != 3 {
		t.Errorf("%d reads, want labels, metrics and one service label", facts.Reads)
	}
}

func TestDraftContextPrometheusSearchesTheQuestionsKeywordsWhenTheCatalogIsCut(t *testing.T) {
	ds := promDraftDS(true)
	r := draftRegistry(t, nil, map[string]*fakeDS{"ds": ds})
	inst, _ := registered(t, r.deps.Datasources, "ds")
	facts, err := r.draftContext(reading(context.Background()), inst, signalMetrics, draftRange, "What is the cpu usage of the checkout service?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "metrics: process_cpu_seconds_total counter\n") || strings.Contains(facts.Text, "http_requests_total") {
		t.Fatalf("facts:\n%s", facts.Text)
	}
}

func TestDraftContextPrometheusKeepsTheFirstMetricsWhenNoKeywordMatches(t *testing.T) {
	ds := promDraftDS(true)
	r := draftRegistry(t, nil, map[string]*fakeDS{"ds": ds})
	inst, _ := registered(t, r.deps.Datasources, "ds")
	facts, err := r.draftContext(reading(context.Background()), inst, signalMetrics, draftRange, "disk")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "metrics: http_requests_total counter") || !strings.Contains(facts.Text, moreNotShown+"\n") {
		t.Fatalf("facts:\n%s", facts.Text)
	}
}

func TestQuestionKeywordsDropShortAndCommonWordsAndKeepTheFirstTen(t *testing.T) {
	got := questionKeywords("What is the 95th percentile latency of the checkout service, per route?")
	want := []string{"95th", "percentile", "latency", "checkout", "service", "route"}
	if !slices.Equal(got, want) {
		t.Fatalf("keywords %q, want %q", got, want)
	}
	long := questionKeywords("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima")
	if len(long) != draftKeywords || long[draftKeywords-1] != "juliet" {
		t.Fatalf("keywords %q, want the first %d", long, draftKeywords)
	}
	if got := questionKeywords("is it up"); len(got) != 0 {
		t.Fatalf("keywords %q, want none", got)
	}
}

func TestDraftContextPrometheusSearchesAKeywordThatComesLateInTheQuestion(t *testing.T) {
	spanMetric := datasource.Metric{Name: "traces_span_metrics_duration_milliseconds_bucket", Type: "histogram"}
	all := promCatalog(append(slices.Clone(promMetrics), spanMetric), true)
	ds := &fakeDS{typ: "prometheus", instantFn: oneRow, discoverFn: func(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		cat, err := all(ctx, req)
		if err == nil && req.Kind == datasource.DiscoverMetrics && req.Search == "" {
			cat.Metrics = cat.Metrics[:len(promMetrics)]
		}
		return cat, err
	}}
	r := draftRegistry(t, nil, map[string]*fakeDS{"ds": ds})
	inst, _ := registered(t, r.deps.Datasources, "ds")
	question := "p95 latency per service for the shipping and checkout services, in milliseconds, from the span metrics"
	facts, err := r.draftContext(reading(context.Background()), inst, signalMetrics, draftRange, question)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "traces_span_metrics_duration_milliseconds_bucket histogram") {
		t.Fatalf("facts:\n%s", facts.Text)
	}
}

func TestDraftContextPrometheusMarksTheListCutOnlyWhenAKeywordSearchWasCut(t *testing.T) {
	for _, tc := range []struct {
		name         string
		searchCut    bool
		wantMoreNote bool
	}{{"every keyword search complete", false, false}, {"a keyword search hit its limit", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ds := &fakeDS{typ: "prometheus", instantFn: oneRow, discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
				if req.Kind != datasource.DiscoverMetrics {
					return &datasource.Catalog{}, nil
				}
				if req.Search == "" {
					return &datasource.Catalog{Metrics: promMetrics[:1], Truncated: true}, nil
				}
				return &datasource.Catalog{Metrics: promMetrics[2:], Truncated: tc.searchCut}, nil
			}}
			r := draftRegistry(t, nil, map[string]*fakeDS{"ds": ds})
			inst, _ := registered(t, r.deps.Datasources, "ds")
			facts, err := r.draftContext(reading(context.Background()), inst, signalMetrics, draftRange, "cpu usage")
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(facts.Text, "process_cpu_seconds_total counter"+moreNotShown+"\n"); got != tc.wantMoreNote {
				t.Fatalf("marker %v, want %v:\n%s", got, tc.wantMoreNote, facts.Text)
			}
		})
	}
}

func TestDraftContextPrometheusSearchesKeywordsConcurrentlyUpToTheCap(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	ds := &fakeDS{typ: "prometheus", instantFn: oneRow, discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		if req.Kind != datasource.DiscoverMetrics || req.Search == "" {
			return &datasource.Catalog{Metrics: promMetrics[:1], Truncated: req.Kind == datasource.DiscoverMetrics}, nil
		}
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		return &datasource.Catalog{Metrics: []datasource.Metric{{Name: "m_" + req.Search}}}, nil
	}}
	r := draftRegistry(t, nil, map[string]*fakeDS{"ds": ds})
	inst, _ := registered(t, r.deps.Datasources, "ds")
	facts, err := r.draftContext(reading(context.Background()), inst, signalMetrics, draftRange, "alpha bravo charlie delta echo foxtrot golf hotel")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if peak < 2 || peak > draftKeywordReads {
		t.Fatalf("peak %d searches at once, want 2 to %d", peak, draftKeywordReads)
	}
	if !strings.Contains(facts.Text, "metrics: m_alpha, m_bravo, m_charlie") {
		t.Fatalf("facts:\n%s", facts.Text)
	}
}

func TestInterleaveMetricsTakesOneFromEachKeywordInTurnWithoutRepeats(t *testing.T) {
	named := func(names ...string) []datasource.Metric {
		out := make([]datasource.Metric, len(names))
		for i, n := range names {
			out[i] = datasource.Metric{Name: n}
		}
		return out
	}
	got := interleaveMetrics([][]datasource.Metric{named("a1", "a2", "a3"), named("b1", "a2"), nil, named("c1")})
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	if want := []string{"a1", "b1", "c1", "a2", "a3"}; !slices.Equal(names, want) {
		t.Fatalf("metrics %q, want %q", names, want)
	}
}

func TestDraftWritesPromQLOnPrometheus(t *testing.T) {
	provider := &scriptedDraft{turns: []llm.Turn{submitTurn(`{"query":"sum by (service) (rate(http_requests_total[5m]))","explanation":"Requests per second by service."}`)}}
	r := draftRegistry(t, provider, map[string]*fakeDS{"prom": promDraftDS(false)})
	draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "prom", Question: "requests per service"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Language != languagePromQL || draft.Query != "sum by (service) (rate(http_requests_total[5m]))" || draft.TraceFilters != nil || len(draft.Warnings) != 0 {
		t.Fatalf("draft %+v", draft)
	}
	if !strings.Contains(provider.requests[0].Context, "http_requests_total counter") {
		t.Fatalf("facts:\n%s", provider.requests[0].Context)
	}
}

func TestDraftOnPrometheusWarnsAboutAnEmptySample(t *testing.T) {
	ds := promDraftDS(false)
	ds.instantFn = func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
		return nil, nil
	}
	provider := &scriptedDraft{turns: []llm.Turn{submitTurn(`{"query":"up{job=\"none\"}","explanation":"Targets of a job."}`)}}
	r := draftRegistry(t, provider, map[string]*fakeDS{"prom": ds})
	draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "prom", Question: "up", Signal: signalMetrics})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{warnNoSampleSeries}; !slices.Equal(draft.Warnings, want) {
		t.Fatalf("warnings %q", draft.Warnings)
	}
}

func TestDraftFixesAPromQLQueryThatFailsItsLint(t *testing.T) {
	ds := promDraftDS(false)
	ds.lintFn = func(_ context.Context, query string) ([]datasource.Diagnostic, error) {
		if strings.Count(query, "(") != strings.Count(query, ")") {
			return []datasource.Diagnostic{{Severity: datasource.SeverityError, Message: "unclosed left parenthesis"}}, nil
		}
		return nil, nil
	}
	provider := &scriptedDraft{turns: []llm.Turn{
		submitTurn(`{"query":"sum(rate(http_requests_total[5m])","explanation":"Requests per second."}`),
		submitTurn(`{"query":"sum(rate(http_requests_total[5m]))","explanation":"Requests per second."}`),
	}}
	r := draftRegistry(t, provider, map[string]*fakeDS{"prom": ds})
	draft, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "prom", Question: "requests"})
	if err != nil || draft.Query != "sum(rate(http_requests_total[5m]))" {
		t.Fatalf("draft %+v, err %v", draft, err)
	}
	if got := provider.results[1][0].Content; got != "the query fails the lint check: unclosed left parenthesis"+lastTurnNote {
		t.Fatalf("corrective result %q", got)
	}
}

func TestDraftOnPrometheusRefusesALogsSignal(t *testing.T) {
	r := draftRegistry(t, &scriptedDraft{}, map[string]*fakeDS{"prom": promDraftDS(false)})
	_, err := r.DraftQuery(viewerContext(), DraftInput{DatasourceUID: "prom", Question: "errors", Signal: signalLogs})
	if !errors.Is(err, ErrDraftInvalid) || !strings.Contains(err.Error(), "prom is a Prometheus datasource, which holds metrics; signal must be metrics") {
		t.Fatalf("err = %v", err)
	}
}

func largePrometheus(t *testing.T) datasource.Datasource {
	t.Helper()
	names := make([]string, 300)
	for i := range names {
		names[i] = fmt.Sprintf("go_metric_%03d", i)
	}
	names = append(names, "checkout_latency_seconds_bucket")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := []string{}
		switch r.URL.Path {
		case "/api/v1/label/__name__/values":
			data = names
		case "/api/v1/labels":
			data = []string{"job", "service"}
		case "/api/v1/metadata":
			_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
			return
		}
		body, _ := json.Marshal(map[string]any{"status": "success", "data": data})
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	ds, err := prometheus.New(prometheus.Options{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func TestDraftContextOnARealPrometheusSearchesALargeCatalog(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Name: "prom", Datasource: largePrometheus(t)})
	if err != nil {
		t.Fatal(err)
	}
	r := New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
	inst, _ := registered(t, reg, "prom")
	facts, err := r.draftContext(reading(context.Background()), inst, signalMetrics, draftRange, "p95 checkout latency")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "metrics: checkout_latency_seconds_bucket\n") || strings.Contains(facts.Text, "go_metric_000") {
		t.Fatalf("facts:\n%s", facts.Text)
	}
}
