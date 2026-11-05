package tools

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func TestListToolsRejectFieldsTheirSchemaDoesNotDeclare(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input string
		field string
	}{
		{"metrics table", nameListMetrics, `{"datasource":"fake","table":"x"}`, "table"},
		{"metrics label", nameListMetrics, `{"datasource":"fake","label":"x"}`, "label"},
		{"metrics metric", nameListMetrics, `{"datasource":"fake","metric":"x"}`, "metric"},
		{"labels label", nameListLabels, `{"datasource":"fake","label":"x"}`, "label"},
		{"label values unknown", nameListLabelValues, `{"datasource":"fake","label":"a","bogus":1}`, "bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &fakeDS{}
			r := newTestRegistry(t, ds, nil)
			res, err := call(t, r, tt.tool, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != codeInvalidRequest {
				t.Fatalf("code = %q, want %q", got, codeInvalidRequest)
			}
			if got, want := res.Content.(errorBody).Message, `unknown field "`+tt.field+`"; `+tt.tool+` takes `; !strings.HasPrefix(got, want) {
				t.Fatalf("message = %q, want prefix %q", got, want)
			}
			if len(ds.requests) != 0 {
				t.Fatalf("discovery ran for a rejected input: %#v", ds.requests)
			}
		})
	}
}

func TestListToolsPassTheirDeclaredFieldsToDiscovery(t *testing.T) {
	ds := &fakeDS{}
	r := newTestRegistry(t, ds, nil)
	for _, tt := range []struct{ tool, input string }{
		{nameListMetrics, `{"datasource":"fake","search":"http","limit":7}`},
		{nameListLabels, `{"datasource":"fake","metric":"m","table":"t","search":"s","limit":7}`},
		{nameListLabelValues, `{"datasource":"fake","label":"l","metric":"m","table":"t","search":"s","limit":7}`},
	} {
		res, err := call(t, r, tt.tool, tt.input)
		if err != nil || res.IsError {
			t.Fatalf("%s: res = %#v, err = %v", tt.tool, res, err)
		}
	}
	got := ds.requests
	if len(got) != 3 || got[0].Search != "http" || got[0].Limit != 8 ||
		got[1].Metric != "m" || got[1].Table != "t" ||
		got[2].Label != "l" || got[2].Search != "s" {
		t.Fatalf("discovery requests = %#v", got)
	}
}

func TestListToolsReportABusyUpstreamAsTooManyRequests(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: &fakeDS{}})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(reg, logger, queryproxy.WithUpstreamWait(30*time.Millisecond))
	r := New(Deps{Datasources: reg, Runner: runner, Logger: logger})
	for range 32 {
		release, err := runner.AcquireUpstream(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
	}

	res, err := call(t, r, nameListMetrics, `{"datasource":"fake"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := errorCode(t, res); got != "too_many_requests" {
		t.Fatalf("code = %q", got)
	}
	if got, want := res.Content.(errorBody).Message, "the server is making too many datasource calls right now; try again shortly"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if want := "Listing metrics on fake: too_many_requests"; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}

func TestListLabelsInvalidLimitMessage(t *testing.T) {
	for _, limit := range []string{"0", "-1", "501"} {
		t.Run(limit, func(t *testing.T) {
			ds := &fakeDS{}
			r := newTestRegistry(t, ds, nil)
			res, err := call(t, r, nameListLabels, `{"datasource":"fake","limit":`+limit+`}`)
			if err != nil {
				t.Fatal(err)
			}
			if got := errorCode(t, res); got != codeInvalidRequest {
				t.Fatalf("code = %q", got)
			}
			if got, want := res.Content.(errorBody).Message, "limit must be between 1 and 500"; got != want {
				t.Fatalf("message = %q, want %q", got, want)
			}
			if len(ds.requests) != 0 {
				t.Fatalf("discovery ran for a rejected limit")
			}
		})
	}
}

func numbered(prefix string, n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("%s%03d", prefix, i)
	}
	return names
}

func metricsFake(total int) *fakeDS {
	return &fakeDS{discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		cat := &datasource.Catalog{}
		for _, name := range numbered("m", total)[:min(total, req.Limit)] {
			cat.Metrics = append(cat.Metrics, datasource.Metric{Name: name})
		}
		return cat, nil
	}}
}

func metricNames(out metricsOut) []string {
	var names []string
	for _, m := range out.Metrics {
		names = append(names, m.Name)
	}
	return names
}

func TestListMetricsPagesWithOffset(t *testing.T) {
	ds := metricsFake(20)
	r := newTestRegistry(t, ds, nil)

	res, err := call(t, r, nameListMetrics, `{"datasource":"fake","limit":4,"offset":3}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if got := ds.lastRequest().Limit; got != 8 {
		t.Fatalf("the runner was asked for %d names, want offset + limit + 1 = 8", got)
	}
	out := res.Content.(metricsOut)
	if !slices.Equal(metricNames(out), []string{"m003", "m004", "m005", "m006"}) {
		t.Fatalf("metrics = %v", metricNames(out))
	}
	if !out.Truncated || out.TruncatedBy != "limit" || out.NextOffset == nil || *out.NextOffset != 7 {
		t.Fatalf("truncation = %+v, want limit and next_offset 7", out.truncation)
	}

	res, err = call(t, r, nameListMetrics, `{"datasource":"fake","limit":4,"offset":16}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	out = res.Content.(metricsOut)
	if !slices.Equal(metricNames(out), []string{"m016", "m017", "m018", "m019"}) || out.truncation != (truncation{}) {
		t.Fatalf("last page = %v %+v", metricNames(out), out.truncation)
	}
}

func TestListToolsRejectAnOffsetOutOfRange(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	for _, offset := range []string{"-1", "500"} {
		res, err := call(t, r, nameListLabels, `{"datasource":"fake","offset":`+offset+`}`)
		if err != nil {
			t.Fatal(err)
		}
		if errorCode(t, res) != codeInvalidRequest || res.Content.(errorBody).Message != "offset must be between 0 and 499" {
			t.Fatalf("offset %s: res = %#v", offset, res.Content)
		}
	}
}

func TestListToolsStillRejectALimitAboveTheMaximumWithAnOffset(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameListMetrics, `{"datasource":"fake","limit":501,"offset":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeInvalidRequest || res.Content.(errorBody).Message != "limit must be between 1 and 500" {
		t.Fatalf("res = %#v", res.Content)
	}
}

func TestGetSchemaPagesItsTables(t *testing.T) {
	longNames := make([]string, 700)
	for i := range longNames {
		longNames[i] = fmt.Sprintf("table_%03d_%s", i, strings.Repeat("x", 40))
	}
	ds := &fakeDS{discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		cat := &datasource.Catalog{}
		for _, name := range longNames[:min(len(longNames), req.Limit)] {
			cat.Tables = append(cat.Tables, datasource.Table{Name: name})
		}
		return cat, nil
	}}
	r := newTestRegistry(t, ds, nil)

	res, err := call(t, r, nameGetSchema, `{"datasource":"fake"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	first := res.Content.(tablesOut)
	if !first.Truncated || first.TruncatedBy != "bytes" || first.NextOffset == nil || *first.NextOffset != len(first.Tables) || len(first.Tables) >= maxDiscoverItems {
		t.Fatalf("first page: %d tables, truncation = %+v", len(first.Tables), first.truncation)
	}

	res, err = call(t, r, nameGetSchema, fmt.Sprintf(`{"datasource":"fake","offset":%d}`, *first.NextOffset))
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	second := res.Content.(tablesOut)
	if second.Tables[0].Name != longNames[*first.NextOffset] {
		t.Fatalf("second page starts at %q, want %q", second.Tables[0].Name, longNames[*first.NextOffset])
	}
	if got, want := ds.lastRequest().Limit, *first.NextOffset+maxDiscoverItems+1; got != want {
		t.Fatalf("discovery asked for %d tables, want offset + %d", got, maxDiscoverItems+1)
	}
}

func TestDiscoveryToolsReportTheNamesTheyReturned(t *testing.T) {
	ds := &fakeDS{discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return &datasource.Catalog{
			Metrics: []datasource.Metric{{Name: "up", Help: "is up"}, {Name: "down"}},
			Labels:  []string{"job", "instance"},
			Values:  []string{"api", "web"},
			Tables:  []datasource.Table{{Name: "otel_logs"}, {Name: "otel_traces"}},
			Columns: []datasource.Column{{Name: "Body"}, {Name: "Timestamp"}},
		}, nil
	}}
	r := newTestRegistry(t, ds, nil)
	tests := []struct {
		name  string
		tool  string
		input string
		of    string
		names []string
	}{
		{"metrics", nameListMetrics, `{"datasource":"fake"}`, "metrics", []string{"up", "down"}},
		{"metrics matching", nameListMetrics, `{"datasource":"fake","search":"http"}`, "metrics matching http", []string{"up", "down"}},
		{"labels", nameListLabels, `{"datasource":"fake"}`, "labels", []string{"job", "instance"}},
		{"labels of a metric", nameListLabels, `{"datasource":"fake","metric":"up"}`, "labels of up", []string{"job", "instance"}},
		{"labels of a table", nameListLabels, `{"datasource":"fake","table":"otel_logs"}`, "labels of otel_logs", []string{"job", "instance"}},
		{"values of a label", nameListLabelValues, `{"datasource":"fake","label":"job"}`, "values of job", []string{"api", "web"}},
		{"values of a label on a metric", nameListLabelValues, `{"datasource":"fake","label":"job","metric":"up"}`, "values of job on up", []string{"api", "web"}},
		{"tables", nameGetSchema, `{"datasource":"fake"}`, "tables", []string{"otel_logs", "otel_traces"}},
		{"columns", nameGetSchema, `{"datasource":"fake","table":"otel_logs"}`, "columns of otel_logs", []string{"Body", "Timestamp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := call(t, r, tt.tool, tt.input)
			if err != nil || res.IsError {
				t.Fatalf("res = %#v err = %v", res, err)
			}
			got := res.Discovered
			if got == nil || got.Datasource != "fake" || got.Of != tt.of || !slices.Equal(got.Names, tt.names) {
				t.Fatalf("discovered = %+v, want fake / %q / %v", got, tt.of, tt.names)
			}
		})
	}
}

func TestOtherToolsDiscoverNothing(t *testing.T) {
	r := newTestRegistry(t, &fakeDS{}, nil)
	res, err := call(t, r, nameListDatasources, `{}`)
	if err != nil || res.Discovered != nil {
		t.Fatalf("discovered = %+v err = %v", res.Discovered, err)
	}
}
