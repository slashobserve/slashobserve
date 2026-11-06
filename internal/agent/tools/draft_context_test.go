package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
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
