package tools

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/llm"
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
	inst, _ := r.deps.Datasources.Get("ds")
	facts, err := r.draftContext(context.Background(), inst, signalMetrics, draftRange, "What is the cpu usage of the checkout service?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "metrics: process_cpu_seconds_total counter"+moreNotShown+"\n") || strings.Contains(facts.Text, "http_requests_total") {
		t.Fatalf("facts:\n%s", facts.Text)
	}
}

func TestDraftContextPrometheusKeepsTheFirstMetricsWhenNoKeywordMatches(t *testing.T) {
	ds := promDraftDS(true)
	r := draftRegistry(t, nil, map[string]*fakeDS{"ds": ds})
	inst, _ := r.deps.Datasources.Get("ds")
	facts, err := r.draftContext(context.Background(), inst, signalMetrics, draftRange, "disk")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "metrics: http_requests_total counter") || !strings.Contains(facts.Text, moreNotShown+"\n") {
		t.Fatalf("facts:\n%s", facts.Text)
	}
}

func TestQuestionKeywordsDropShortAndCommonWordsAndKeepTheFirstFour(t *testing.T) {
	got := questionKeywords("What is the 95th percentile latency of the checkout service, per route?")
	want := []string{"95th", "percentile", "latency", "checkout"}
	if !slices.Equal(got, want) {
		t.Fatalf("keywords %q, want %q", got, want)
	}
	if got := questionKeywords("is it up"); len(got) != 0 {
		t.Fatalf("keywords %q, want none", got)
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
