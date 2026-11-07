package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

var draftRange = datasource.TimeRange{From: time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 6, 11, 0, 0, 0, time.UTC)}

func contextFor(t *testing.T, ds *fakeDS, signal string) (draftFacts, error) {
	t.Helper()
	r := draftRegistry(t, nil, map[string]*fakeDS{"ds": ds})
	inst, ok := r.deps.Datasources.Get("ds")
	if !ok {
		t.Fatal("no datasource")
	}
	return r.draftContext(context.Background(), inst, signal, draftRange)
}

func valueRequests(ds *fakeDS) []datasource.DiscoverRequest {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	var out []datasource.DiscoverRequest
	for _, req := range ds.requests {
		if req.Kind == datasource.DiscoverLabelValues || req.Kind == datasource.DiscoverColumns {
			out = append(out, req)
		}
	}
	return out
}

func TestDraftContextLokiPrefersServiceNameAndDetectedLevel(t *testing.T) {
	ds := &fakeDS{typ: "loki", discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		if req.Kind == datasource.DiscoverLabels {
			return &datasource.Catalog{Labels: []string{"job", "level", "service_name", "detected_level"}}, nil
		}
		return &datasource.Catalog{Values: []string{"checkout", "payments"}}, nil
	}}
	facts, err := contextFor(t, ds, signalLogs)
	if err != nil {
		t.Fatal(err)
	}
	reqs := valueRequests(ds)
	if len(reqs) != 2 || reqs[0].Label != "service_name" || reqs[1].Label != "detected_level" || reqs[0].Limit != draftValuesLimit {
		t.Fatalf("value reads %+v", reqs)
	}
	for _, want := range []string{
		"datasource: ds (loki), signal: logs\n",
		"time range: 2026-11-06T10:00:00Z to 2026-11-06T11:00:00Z\n",
		"labels: job, level, service_name, detected_level\n",
		`values of service_name: "checkout", "payments"` + "\n",
		`values of detected_level: "checkout", "payments"` + "\n",
	} {
		if !strings.Contains(facts.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, facts.Text)
		}
	}
	if facts.Reads != 3 || !facts.ServicesComplete || strings.Join(facts.Services, ",") != "checkout,payments" {
		t.Fatalf("facts %+v", facts)
	}
}

func clickhouseCatalog(tables ...string) func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		switch req.Kind {
		case datasource.DiscoverTables:
			cat := &datasource.Catalog{}
			for _, name := range tables {
				cat.Tables = append(cat.Tables, datasource.Table{Name: name})
			}
			return cat, nil
		case datasource.DiscoverColumns:
			return &datasource.Catalog{Columns: []datasource.Column{{Name: "TraceId", Type: "String"}, {Name: "Duration", Type: "UInt64"}}}, nil
		}
		return &datasource.Catalog{Values: []string{"checkout", "payments"}}, nil
	}
}

func TestDraftContextClickHouseTracesReadsOtelTraces(t *testing.T) {
	ds := &fakeDS{typ: "clickhouse", discoverFn: clickhouseCatalog("otel_logs", "otel_traces")}
	facts, err := contextFor(t, ds, signalTraces)
	if err != nil {
		t.Fatal(err)
	}
	reqs := valueRequests(ds)
	if len(reqs) != 2 || reqs[0].Kind != datasource.DiscoverColumns || reqs[0].Table != "otel_traces" ||
		reqs[1].Table != "otel_traces" || reqs[1].Label != "ServiceName" {
		t.Fatalf("reads %+v", reqs)
	}
	if !strings.Contains(facts.Text, "columns of otel_traces: TraceId String, Duration UInt64\n") || !strings.Contains(facts.Text, "answer: trace_filters") {
		t.Fatalf("text:\n%s", facts.Text)
	}

	ds = &fakeDS{typ: "clickhouse", discoverFn: clickhouseCatalog("otel_logs")}
	if facts, err = contextFor(t, ds, signalTraces); err != nil {
		t.Fatal(err)
	}
	if reqs := valueRequests(ds); len(reqs) != 0 || facts.Reads != 1 || len(facts.Services) != 0 {
		t.Fatalf("reads %+v, facts %+v", reqs, facts)
	}
}

func TestDraftContextCapsTheTextAndMarksTheCutList(t *testing.T) {
	values := make([]string, 5000)
	for i := range values {
		values[i] = fmt.Sprintf("service-%04d", i)
	}
	ds := &fakeDS{typ: "tempo", discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		if req.Kind == datasource.DiscoverLabels {
			return &datasource.Catalog{Labels: []string{"resource.service.name"}}, nil
		}
		return &datasource.Catalog{Values: values}, nil
	}}
	facts, err := contextFor(t, ds, signalTraces)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Text) > draftContextBytes || !strings.HasSuffix(facts.Text, moreNotShown+"\n") {
		t.Fatalf("text is %d bytes, ends %q", len(facts.Text), facts.Text[len(facts.Text)-40:])
	}
	if facts.ServicesComplete || len(facts.Services) != 5000 {
		t.Fatalf("services complete %t, %d services", facts.ServicesComplete, len(facts.Services))
	}
}

func TestDraftContextMarksAnUpstreamTruncatedList(t *testing.T) {
	ds := &fakeDS{typ: "tempo", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return &datasource.Catalog{Values: []string{"checkout"}, Truncated: true}, nil
	}}
	facts, err := contextFor(t, ds, signalTraces)
	if err != nil {
		t.Fatal(err)
	}
	if facts.ServicesComplete || !strings.Contains(facts.Text, `values of resource.service.name: "checkout"`+moreNotShown) {
		t.Fatalf("facts %+v", facts)
	}
}

func TestDraftContextKeepsAnUnavailableLineForAFailedRead(t *testing.T) {
	ds := &fakeDS{typ: "loki", discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		if req.Kind == datasource.DiscoverLabels {
			return &datasource.Catalog{Labels: []string{"service_name", "level"}}, nil
		}
		if req.Label == "level" {
			return nil, datasource.Errorf(datasource.ErrUnavailable, "connection refused")
		}
		return &datasource.Catalog{Values: []string{"checkout"}}, nil
	}}
	facts, err := contextFor(t, ds, signalLogs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "values of level: unavailable (datasource_unavailable)\n") || !strings.Contains(facts.Text, `values of service_name: "checkout"`) {
		t.Fatalf("text:\n%s", facts.Text)
	}
}

func TestDraftContextFailsWhenEveryReadFails(t *testing.T) {
	ds := &fakeDS{typ: "tempo", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "connection refused")
	}}
	_, err := contextFor(t, ds, signalTraces)
	var dsErr *DatasourceError
	if !errors.As(err, &dsErr) || dsErr.Code != "datasource_unavailable" || !strings.Contains(dsErr.Message, "connection refused") {
		t.Fatalf("err = %v", err)
	}
}

func TestDraftContextReturnsACancel(t *testing.T) {
	ds := &fakeDS{typ: "tempo", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return nil, context.Canceled
	}}
	if _, err := contextFor(t, ds, signalTraces); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

type roleFake struct {
	*fakeDS
	roles elasticsearch.FieldRoles
	err   error
}

func (f *roleFake) Roles(context.Context) (elasticsearch.FieldRoles, error) { return f.roles, f.err }

func searchCatalog(labels []string, kinds map[string]string, values map[string][]string) func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		switch req.Kind {
		case datasource.DiscoverLabels:
			return &datasource.Catalog{Labels: labels}, nil
		case datasource.DiscoverColumns:
			var cols []datasource.Column
			for name, kind := range kinds {
				cols = append(cols, datasource.Column{Name: name, Type: kind})
			}
			return &datasource.Catalog{Columns: cols}, nil
		}
		return &datasource.Catalog{Values: values[req.Label]}, nil
	}
}

func searchRegistry(t *testing.T, provider llm.Provider, ds datasource.Datasource) *Registry {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: "es", Name: "es", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Provider: provider, Logger: logger})
}

func searchFacts(t *testing.T, ds datasource.Datasource) (draftFacts, error) {
	t.Helper()
	r := searchRegistry(t, nil, ds)
	inst, _ := r.deps.Datasources.Get("es")
	return r.draftContext(context.Background(), inst, signalLogs, draftRange)
}

func TestDraftContextSearchListsFieldsWithKindsAndTheRoleValues(t *testing.T) {
	inner := &fakeDS{typ: "elasticsearch", discoverFn: searchCatalog(
		[]string{"log.level", "message", "service.name"},
		map[string]string{"log.level": "keyword", "message": "text", "service.name": "keyword", "@timestamp": "date"},
		map[string][]string{"service.name": {"checkout", "payments"}, "log.level": {"error", "info"}},
	)}
	ds := &roleFake{fakeDS: inner, roles: elasticsearch.FieldRoles{Time: "@timestamp", Service: "service.name", Level: "log.level"}}
	facts, err := searchFacts(t, ds)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"datasource: es (elasticsearch), signal: logs\n",
		"answer: a KQL query in submit_query's query (Kibana query language, such as service.name:checkout and log.level:error); prefix lucene: only when KQL cannot express it.\n",
		"fields: log.level keyword, message text (facet via .keyword), service.name keyword\n",
		`values of service.name: "checkout", "payments"` + "\n",
		`values of log.level: "error", "info"` + "\n",
	} {
		if !strings.Contains(facts.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, facts.Text)
		}
	}
	reqs := valueRequests(inner)
	if len(reqs) != 3 || reqs[1].Label != "service.name" || reqs[2].Label != "log.level" || reqs[1].Limit != draftValuesLimit {
		t.Fatalf("reads %+v", reqs)
	}
	if facts.Reads != 4 || !facts.ServicesComplete || strings.Join(facts.Services, ",") != "checkout,payments" {
		t.Fatalf("facts %+v", facts)
	}
}

func TestDraftContextSearchShowsTheFirstSixtyFields(t *testing.T) {
	labels := make([]string, 100)
	for i := range labels {
		labels[i] = fmt.Sprintf("field.%03d", i)
	}
	facts, err := searchFacts(t, &fakeDS{typ: "opensearch", discoverFn: searchCatalog(labels, nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	line := ""
	for _, l := range strings.Split(facts.Text, "\n") {
		if strings.HasPrefix(l, "fields: ") {
			line = l
		}
	}
	if !strings.Contains(line, "field.059") || strings.Contains(line, "field.060") || !strings.HasSuffix(line, moreNotShown) {
		t.Fatalf("fields line: %s", line)
	}
}

func TestDraftContextSearchKeepsTheFieldsWhenTheRolesCannotBeRead(t *testing.T) {
	inner := &fakeDS{typ: "elasticsearch", discoverFn: searchCatalog([]string{"service.name"}, nil, nil)}
	ds := &roleFake{fakeDS: inner, err: datasource.Errorf(datasource.ErrNoTimeField, "time field is missing")}
	facts, err := searchFacts(t, ds)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts.Text, "fields: service.name\n") || !strings.Contains(facts.Text, "field roles: unavailable\n") || len(facts.Services) != 0 {
		t.Fatalf("facts %+v\n%s", facts, facts.Text)
	}
}
