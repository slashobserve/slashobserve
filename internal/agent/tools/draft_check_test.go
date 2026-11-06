package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDraftCheckLanguageFor(t *testing.T) {
	cases := []struct {
		dsType   spec.DatasourceType
		signal   string
		hasQuery bool
		want     string
		ok       bool
	}{
		{spec.Loki, signalLogs, true, languageLogQL, true},
		{spec.ClickHouse, signalLogs, true, languageClickHouseSQL, true},
		{spec.ClickHouse, signalTraces, false, languageFilters, true},
		{spec.Tempo, signalTraces, false, languageFilters, true},
		{spec.Tempo, signalTraces, true, languageTraceQL, true},
		{spec.Elasticsearch, signalLogs, true, languageKQL, true},
		{spec.OpenSearch, signalLogs, true, languageKQL, true},
		{spec.Elasticsearch, signalLogs, false, "", false},
		{spec.OpenSearch, signalTraces, true, "", false},
		{spec.Loki, signalLogs, false, "", false},
		{spec.Loki, signalTraces, true, "", false},
		{spec.ClickHouse, signalLogs, false, "", false},
		{spec.ClickHouse, signalTraces, true, "", false},
		{spec.Tempo, signalLogs, true, "", false},
		{spec.Prometheus, signalLogs, true, "", false},
	}
	for _, c := range cases {
		got, ok := languageFor(c.dsType, c.signal, c.hasQuery)
		if got != c.want || ok != c.ok {
			t.Errorf("languageFor(%s, %s, %t) = %q, %t; want %q, %t", c.dsType, c.signal, c.hasQuery, got, ok, c.want, c.ok)
		}
	}
}

func TestDraftCheckFilters(t *testing.T) {
	accepted := []TraceFilters{
		{},
		{Min: "250ms", Max: "2s"},
		{Min: "1.5µs"},
		{Max: "1h", Errors: true},
		{Min: "2s", Max: "2000ms"},
		{Service: strings.Repeat("s", maxFilterRunes)},
	}
	for _, f := range accepted {
		if err := checkFilters(f); err != nil {
			t.Errorf("checkFilters(%+v) = %v", f, err)
		}
	}
	refused := []struct {
		filters TraceFilters
		want    string
	}{
		{TraceFilters{Min: "fast"}, "trace_filters.min must be a duration"},
		{TraceFilters{Max: "250"}, "trace_filters.max must be a duration"},
		{TraceFilters{Min: "-1s"}, "trace_filters.min must be a duration"},
		{TraceFilters{Min: "2s", Max: "250ms"}, "must not be greater than max"},
		{TraceFilters{Service: strings.Repeat("s", maxFilterRunes+1)}, "trace_filters.service must be at most 200 characters"},
		{TraceFilters{Span: "a\nb"}, "trace_filters.span must not contain control characters"},
	}
	for _, c := range refused {
		if err := checkFilters(c.filters); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("checkFilters(%+v) = %v, want %q", c.filters, err, c.want)
		}
	}
}

func TestDraftCheckSampleRange(t *testing.T) {
	to := time.Date(2026, 11, 6, 11, 0, 0, 0, time.UTC)
	hour := datasource.TimeRange{From: to.Add(-time.Hour), To: to}
	if got := sampleRange(hour); !got.From.Equal(to.Add(-15*time.Minute)) || !got.To.Equal(to) {
		t.Errorf("1 h range samples %v", got)
	}
	short := datasource.TimeRange{From: to.Add(-5 * time.Minute), To: to}
	if got := sampleRange(short); got != short {
		t.Errorf("5 min range samples %v", got)
	}
}

func TestDraftCheckSample(t *testing.T) {
	cases := []struct {
		name        string
		instant     func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error)
		wantEmpty   bool
		wantFailure string
	}{
		{"limit exceeded passes", func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return nil, datasource.Errorf(datasource.ErrLimitExceeded, "more than 1000 rows")
		}, false, ""},
		{"invalid query fails", func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return nil, datasource.Errorf(datasource.ErrInvalidQuery, "unknown column Message")
		}, false, "the query failed on a sample of the last 15 minutes: query_invalid: "},
		{"no rows is empty", func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return nil, nil
		}, true, ""},
		{"rows", func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return []datasource.Frame{logTableFrame()}, nil
		}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sampled datasource.TimeRange
			ds := &fakeDS{typ: "clickhouse", instantFn: func(ctx context.Context, q datasource.Query, rng datasource.TimeRange) ([]datasource.Frame, error) {
				sampled = rng
				return c.instant(ctx, q, rng)
			}}
			r := draftRegistry(t, nil, map[string]*fakeDS{"ch": ds})
			inst, _ := r.deps.Datasources.Get("ch")
			empty, failure, err := r.sample(context.Background(), inst, "SELECT 1", draftRange)
			if err != nil {
				t.Fatal(err)
			}
			if empty != c.wantEmpty || !strings.HasPrefix(failure, c.wantFailure) || (c.wantFailure == "") != (failure == "") {
				t.Fatalf("empty %t, failure %q", empty, failure)
			}
			if c.wantFailure != "" && !strings.Contains(failure, "unknown column Message") {
				t.Fatalf("failure %q lacks the datasource message", failure)
			}
			if !sampled.From.Equal(draftRange.To.Add(-sampleWindow)) || !sampled.To.Equal(draftRange.To) {
				t.Fatalf("sampled %v", sampled)
			}
		})
	}
}
