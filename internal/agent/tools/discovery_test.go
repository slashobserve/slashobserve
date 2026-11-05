package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
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

func shopFamilies(n int) []datasource.Family {
	families := make([]datasource.Family, n)
	for i := range families {
		families[i] = datasource.Family{
			Name:   fmt.Sprintf("shop_checkout_requests_%03d_total", i),
			Type:   "counter",
			Help:   "Requests served by the checkout service.",
			Labels: []string{"instance", "job", "pod", "route", "status"},
			Series: 12,
		}
	}
	return families
}

func inventoryFake(families []datasource.Family, truncated bool) *fakeDS {
	return &fakeDS{discoverFn: func(_ context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
		if req.Kind != datasource.DiscoverInventory {
			return nil, fmt.Errorf("discover kind %q, want inventory", req.Kind)
		}
		return &datasource.Catalog{Families: families, FamiliesTotal: len(families), Truncated: truncated}, nil
	}}
}

func TestListMetricsScopeFitsThreeHundredFamiliesUnderTheCap(t *testing.T) {
	ds := inventoryFake(shopFamilies(300), false)
	r := newTestRegistry(t, ds, nil)
	seen := map[string]bool{}
	offset := 0
	for page := 0; ; page++ {
		res, err := call(t, r, nameListMetrics, fmt.Sprintf(`{"datasource":"fake","scope":"{job=\"shop\"}","offset":%d}`, offset))
		if err != nil || res.IsError {
			t.Fatalf("page %d: res = %#v err = %v", page, res.Content, err)
		}
		out := res.Content.(inventoryOut)
		body, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) > maxContentBytes {
			t.Fatalf("page %d is %d bytes, over the %d byte cap", page, len(body), maxContentBytes)
		}
		if out.FamiliesTotal != 300 || out.Scope != `{job="shop"}` || out.Datasource != "fake" {
			t.Fatalf("page %d: families_total = %d, scope = %q, datasource = %q", page, out.FamiliesTotal, out.Scope, out.Datasource)
		}
		for _, f := range out.Families {
			if seen[f.Name] {
				t.Fatalf("page %d repeats %s", page, f.Name)
			}
			seen[f.Name] = true
		}
		if !out.Truncated {
			break
		}
		if out.TruncatedBy != truncatedByBytes || out.NextOffset == nil || *out.NextOffset != offset+len(out.Families) {
			t.Fatalf("page %d: truncation = %+v", page, out.truncation)
		}
		offset = *out.NextOffset
	}
	if len(seen) != 300 {
		t.Fatalf("pages listed %d families, want 300", len(seen))
	}
	if got := ds.lastRequest(); got.Scope != `{job="shop"}` {
		t.Fatalf("discovery scope = %q", got.Scope)
	}
}

func TestListMetricsScopeCompactsLabelsBeforePaging(t *testing.T) {
	tests := []struct {
		name       string
		families   int
		wantOwn    []string
		wantCommon []string
		wantListed int
		wantPaged  bool
	}{
		{"a small scope keeps every label", 10, []string{"instance", "job", "pod", "route", "status"}, nil, 10, false},
		{"a larger scope names shared labels once", 100, nil, []string{"instance", "job", "pod", "route", "status"}, 100, false},
		{"a large scope pages without own labels", 300, nil, []string{"instance", "job", "pod", "route", "status"}, -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			families := shopFamilies(tt.families)
			if tt.families == 100 {
				for i := range families {
					families[i].Labels = append(slices.Clone(families[i].Labels), fmt.Sprintf("own_%d", i%2))
				}
			}
			r := newTestRegistry(t, inventoryFake(families, false), nil)
			res, err := call(t, r, nameListMetrics, `{"datasource":"fake","scope":"{job=\"shop\"}"}`)
			if err != nil || res.IsError {
				t.Fatalf("res = %#v err = %v", res.Content, err)
			}
			out := res.Content.(inventoryOut)
			if tt.wantListed >= 0 && len(out.Families) != tt.wantListed {
				t.Fatalf("listed %d families, want %d", len(out.Families), tt.wantListed)
			}
			if out.Truncated != tt.wantPaged {
				t.Fatalf("truncated = %v, want %v", out.Truncated, tt.wantPaged)
			}
			if !slices.Equal(out.CommonLabels, tt.wantCommon) {
				t.Fatalf("common_labels = %v, want %v", out.CommonLabels, tt.wantCommon)
			}
			switch {
			case tt.families == 100:
				if got := out.Families[1].Labels; !slices.Equal(got, []string{"own_1"}) {
					t.Fatalf("labels of the second family = %v, want only its own", got)
				}
			case !slices.Equal(out.Families[0].Labels, tt.wantOwn):
				t.Fatalf("labels of the first family = %v, want %v", out.Families[0].Labels, tt.wantOwn)
			}
			if group := out.Grouped["shop_checkout"]; len(group) != len(out.Families) {
				t.Fatalf("grouped shop_checkout = %d names for %d listed families", len(group), len(out.Families))
			}
		})
	}
}

func TestListMetricsScopeGroupsOnlyRelatedFamilies(t *testing.T) {
	families := []datasource.Family{
		{Name: "go_memstats_alloc_bytes", Type: "gauge", Unit: "bytes", Series: 4},
		{Name: "go_memstats_heap_inuse_bytes", Type: "gauge", Series: 4},
		{Name: "go_goroutines", Type: "gauge", Series: 4},
		{Name: "up", Series: 4},
	}
	r := newTestRegistry(t, inventoryFake(families, false), nil)
	res, err := call(t, r, nameListMetrics, `{"datasource":"fake","scope":"{job=\"shop\"}"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	out := res.Content.(inventoryOut)
	want := map[string][]string{"go_memstats": {"go_memstats_alloc_bytes", "go_memstats_heap_inuse_bytes"}}
	if len(out.Grouped) != 1 || !slices.Equal(out.Grouped["go_memstats"], want["go_memstats"]) {
		t.Fatalf("grouped = %v, want %v", out.Grouped, want)
	}
	if !reflect.DeepEqual(out.Families[0], familyOut{Name: "go_memstats_alloc_bytes", Type: "gauge", Unit: "bytes", Series: 4}) {
		t.Fatalf("first family = %+v", out.Families[0])
	}
	if got := res.Discovered; got == nil || got.Of != `metric families in {job="shop"}` || len(got.Names) != 4 {
		t.Fatalf("discovered = %+v", got)
	}
	if want := `Listed 4 metric families in {job="shop"} on fake`; res.Step != want {
		t.Fatalf("step = %q, want %q", res.Step, want)
	}
}

func TestListMetricsScopeReportsAnUpstreamCut(t *testing.T) {
	r := newTestRegistry(t, inventoryFake(shopFamilies(3), true), nil)
	res, err := call(t, r, nameListMetrics, `{"datasource":"fake","scope":"{job=\"shop\"}"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	out := res.Content.(inventoryOut)
	if !out.Truncated || out.TruncatedBy != truncatedByLimit || out.NextOffset != nil || len(out.Families) != 3 {
		t.Fatalf("out = %+v, want every family and a limit cut without next_offset", out)
	}
}

func TestListMetricsScopeTakesNoSearchOrLimit(t *testing.T) {
	for _, input := range []string{
		`{"datasource":"fake","scope":"{job=\"shop\"}","search":"http"}`,
		`{"datasource":"fake","scope":"{job=\"shop\"}","limit":10}`,
	} {
		ds := &fakeDS{}
		r := newTestRegistry(t, ds, nil)
		res, err := call(t, r, nameListMetrics, input)
		if err != nil {
			t.Fatal(err)
		}
		if errorCode(t, res) != codeInvalidRequest || res.Content.(errorBody).Message != scopeTakesNoFilter {
			t.Fatalf("%s: res = %#v", input, res.Content)
		}
		if len(ds.requests) != 0 {
			t.Fatalf("%s: discovery ran for a rejected input", input)
		}
	}
}

func TestListMetricsScopeReportsTheRuntimeFamilyCount(t *testing.T) {
	ds := &fakeDS{discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return &datasource.Catalog{Families: shopFamilies(2), FamiliesTotal: 2, RuntimeFamilies: 43}, nil
	}}
	res, err := call(t, newTestRegistry(t, ds, nil), nameListMetrics, `{"datasource":"fake","scope":"{job=\"shop\"}"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"families_total":2,"runtime_families":43,`) {
		t.Fatalf("result = %s, want families_total 2 and runtime_families 43", body)
	}
}

func TestSampleRowsWithoutATableSamplesEveryLokiStream(t *testing.T) {
	ds := &fakeDS{typ: "loki"}
	res, err := call(t, newTestRegistry(t, ds, nil), nameSampleRows, `{"datasource":"fake"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	if req := ds.lastRequest(); req.Kind != datasource.DiscoverSampleRows || req.Table != "" {
		t.Fatalf("discovery request = %+v", req)
	}

	prom := &fakeDS{}
	res, err = call(t, newTestRegistry(t, prom, nil), nameSampleRows, `{"datasource":"fake"}`)
	if err != nil {
		t.Fatal(err)
	}
	if errorCode(t, res) != codeInvalidRequest || res.Content.(errorBody).Message != "table is required" || len(prom.requests) != 0 {
		t.Fatalf("prometheus without a table: res = %#v after %d discoveries", res.Content, len(prom.requests))
	}
}

func TestSampleRowsPassesALokiSelectorAndRendersTheLines(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	ds := &fakeDS{typ: "loki", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return &datasource.Catalog{Sample: &datasource.Frame{Fields: []datasource.Field{
			{Name: "Time", Type: datasource.FieldTime, Values: []any{at}},
			{Name: "Body", Type: datasource.FieldString, Values: []any{"payment gateway unavailable"}},
			{Name: "service_name", Type: datasource.FieldString, Values: []any{"payments"}},
		}}}, nil
	}}
	res, err := call(t, newTestRegistry(t, ds, nil), nameSampleRows, `{"datasource":"fake","table":"{service_name=\"payments\"}"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	if got := ds.lastRequest().Table; got != `{service_name="payments"}` {
		t.Fatalf("table = %q", got)
	}
	out := res.Content.(sampleOut)
	if !slices.Equal(out.Columns, []string{"Time", "Body", "service_name"}) || len(out.Rows) != 1 || out.Rows[0][1] != "payment gateway unavailable" || out.Table != `{service_name="payments"}` {
		t.Fatalf("sample = %+v", out)
	}
}

func spanCatalog(truncated bool) *datasource.Catalog {
	return &datasource.Catalog{
		Sample: &datasource.Frame{Fields: []datasource.Field{
			{Name: "service", Type: datasource.FieldString, Values: []any{"checkout", "checkout", "payments"}},
			{Name: "span_name", Type: datasource.FieldString, Values: []any{"POST /checkout", "charge", "charge"}},
		}},
		Labels:    []string{"duration", "resource.service.name", "span.http.route"},
		Truncated: truncated,
	}
}

func TestGetSchemaOnTempoListsServicesAndSpanNames(t *testing.T) {
	ds := &fakeDS{typ: "tempo", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return spanCatalog(false), nil
	}}
	r := newTestRegistry(t, ds, nil)
	res, err := call(t, r, nameGetSchema, `{"datasource":"fake"}`)
	if err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res.Content, err)
	}
	if req := ds.lastRequest(); req.Kind != datasource.DiscoverTables {
		t.Fatalf("discovery request = %+v", req)
	}
	out := res.Content.(spanSchemaOut)
	wantRows := [][]any{{"checkout", "POST /checkout"}, {"checkout", "charge"}, {"payments", "charge"}}
	if out.Datasource != "fake" || !slices.Equal(out.Columns, []string{"service", "span_name"}) || !reflect.DeepEqual(out.Rows, wantRows) || out.Truncated {
		t.Fatalf("schema = %+v", out)
	}
	if !slices.Equal(out.Attributes, []string{"duration", "resource.service.name", "span.http.route"}) {
		t.Fatalf("attributes = %v", out.Attributes)
	}
	if res.Step != "Listed 2 services and their span names on fake" {
		t.Fatalf("step = %q", res.Step)
	}
	wantNames := []string{"checkout", "payments", "POST /checkout", "charge", "duration", "resource.service.name", "span.http.route"}
	if got := res.Discovered; got == nil || got.Of != "services, span names and attributes" || !slices.Equal(got.Names, wantNames) {
		t.Fatalf("discovered = %+v", got)
	}

	for _, input := range []string{`{"datasource":"fake","table":"traces"}`, `{"datasource":"fake","offset":1}`} {
		before := len(ds.requests)
		res, err := call(t, r, nameGetSchema, input)
		if err != nil {
			t.Fatal(err)
		}
		if errorCode(t, res) != codeInvalidRequest || res.Content.(errorBody).Message != "tempo has no tables; leave out table and offset" || len(ds.requests) != before {
			t.Fatalf("%s: res = %#v", input, res.Content)
		}
	}

	truncated := &fakeDS{typ: "tempo", discoverFn: func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return spanCatalog(true), nil
	}}
	res, err = call(t, newTestRegistry(t, truncated, nil), nameGetSchema, `{"datasource":"fake"}`)
	if err != nil || res.IsError || !res.Content.(spanSchemaOut).Truncated {
		t.Fatalf("an upstream cut: res = %#v err = %v", res.Content, err)
	}
}

func TestListMetricsScopeOnLokiKeepsTheStoredInventory(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	promFamilies := []datasource.Family{{Name: "http_requests_total", Type: "counter", Series: 4}}
	lokiFamilies := []datasource.Family{{Name: "service_name", Type: "label", Series: 3}}
	reg, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: "prom", Name: "Prometheus", Datasource: inventoryFake(promFamilies, false)},
		datasource.Instance{UID: "loki", Name: "Loki", Datasource: &fakeDS{typ: "loki", discoverFn: inventoryFake(lokiFamilies, false).discoverFn}},
	)
	if err != nil {
		t.Fatal(err)
	}
	r := New(Deps{Datasources: reg, Runner: queryproxy.NewRunner(reg, logger), Logger: logger})
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: "loki-inventory-user", Role: auth.RoleEditor})
	for _, tc := range []struct{ input, family string }{
		{`{"datasource":"prom","scope":"{job=\"shop\"}"}`, "http_requests_total"},
		{`{"datasource":"loki","scope":"{service_name=\"checkout\"}"}`, "service_name"},
	} {
		res, err := r.Call(ctx, nameListMetrics, []byte(tc.input))
		if err != nil || res.IsError {
			t.Fatalf("%s: res = %#v err = %v", tc.input, res.Content, err)
		}
		if got := res.Content.(inventoryOut).Families; len(got) != 1 || got[0].Name != tc.family {
			t.Fatalf("%s: families = %+v", tc.input, got)
		}
	}
	if got, ok := inventories.get(ctx); !ok || !reflect.DeepEqual(got, promFamilies) {
		t.Fatalf("stored inventory = %v, %v; want the Prometheus families", got, ok)
	}
}
