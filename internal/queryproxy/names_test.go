package queryproxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func namesRunner(t *testing.T, fake *fakeDatasource, opts ...RunnerOption) *Runner {
	t.Helper()
	runner := newTestRunner(t, discardLogger(), map[string]*fakeDatasource{"ds": fake})
	runner.slots = NewSlots(map[string]int{"ds": 1}, nil)
	for _, opt := range opts {
		opt(runner)
	}
	return runner
}

func slotsInUse(r *Runner, datasourceUID string) int {
	for _, u := range r.slots.Usage() {
		if u.Datasource == datasourceUID {
			return u.InUse
		}
	}
	return 0
}

func TestNamesReportsABusyUpstreamAfterTheWaitBound(t *testing.T) {
	fake := newFake("prometheus")
	runner := namesRunner(t, fake, WithUpstreamWait(30*time.Millisecond))
	release, err := runner.AcquireUpstream(context.Background(), "ds")
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = runner.Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics})
	if !errors.Is(err, ErrUpstreamBusy) {
		t.Fatalf("err = %v, want ErrUpstreamBusy", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %v for a slot", elapsed)
	}
	status, code, message := Classify(err)
	if status != http.StatusTooManyRequests || code != "too_many_requests" || message == "" {
		t.Fatalf("Classify = (%d, %q, %q)", status, code, message)
	}
	if got := len(fake.discoverRequests()); got != 0 {
		t.Fatalf("discovery ran %d times without a slot", got)
	}

	release()
	if _, err := runner.Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics}); err != nil {
		t.Fatalf("Names after release: %v", err)
	}
}

func TestNamesKeepsTheCallersCancellationApartFromABusyUpstream(t *testing.T) {
	runner := namesRunner(t, newFake("prometheus"), WithUpstreamWait(time.Minute))
	release, err := runner.AcquireUpstream(context.Background(), "ds")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err = runner.Names(ctx, NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics})
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrUpstreamBusy) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func catalogOf(n int) *datasource.Catalog {
	c := &datasource.Catalog{}
	for i := range n {
		name := fmt.Sprintf("n%03d", i)
		c.Metrics = append(c.Metrics, datasource.Metric{Name: name})
		c.Labels = append(c.Labels, name)
		c.Values = append(c.Values, name)
	}
	return c
}

func TestNamesRunnerRules(t *testing.T) {
	boom := datasource.Errorf(datasource.ErrNotFound, "table %q not found", "nope")
	tests := []struct {
		name      string
		typ       string
		req       NamesRequest
		catalog   *datasource.Catalog
		discErr   error
		wantErr   string
		wantIs    error
		wantLimit int
		wantCount int
		wantCut   bool
	}{
		{name: "absent limit is 100", req: NamesRequest{Kind: datasource.DiscoverMetrics}, catalog: catalogOf(101), wantLimit: 101, wantCount: 100, wantCut: true},
		{name: "limit 1", req: NamesRequest{Kind: datasource.DiscoverMetrics, Limit: 1}, catalog: catalogOf(2), wantLimit: 2, wantCount: 1, wantCut: true},
		{name: "limit 500", req: NamesRequest{Kind: datasource.DiscoverMetrics, Limit: 500}, catalog: catalogOf(10), wantLimit: 501, wantCount: 10},
		{name: "limit 501", req: NamesRequest{Kind: datasource.DiscoverMetrics, Limit: 501}, wantErr: "limit must be between 1 and 500", wantIs: ErrInvalidRequest},
		{name: "limit -1", req: NamesRequest{Kind: datasource.DiscoverMetrics, Limit: -1}, wantErr: "limit must be between 1 and 500", wantIs: ErrInvalidRequest},
		{name: "exactly limit items", req: NamesRequest{Kind: datasource.DiscoverLabels, Limit: 3}, catalog: catalogOf(3), wantLimit: 4, wantCount: 3},
		{name: "limit plus one items", req: NamesRequest{Kind: datasource.DiscoverLabels, Limit: 3}, catalog: catalogOf(4), wantLimit: 4, wantCount: 3, wantCut: true},
		{name: "values at limit", req: NamesRequest{Kind: datasource.DiscoverLabelValues, Label: "l", Limit: 3}, catalog: catalogOf(3), wantLimit: 4, wantCount: 3},
		{name: "values over limit", req: NamesRequest{Kind: datasource.DiscoverLabelValues, Label: "l", Limit: 3}, catalog: catalogOf(4), wantLimit: 4, wantCount: 3, wantCut: true},
		{name: "search 200 bytes", req: NamesRequest{Kind: datasource.DiscoverMetrics, Search: strings.Repeat("s", 200)}, catalog: catalogOf(1), wantLimit: 101, wantCount: 1},
		{name: "search 201 bytes", req: NamesRequest{Kind: datasource.DiscoverMetrics, Search: strings.Repeat("s", 201)}, wantErr: "search must be at most 200 bytes", wantIs: ErrInvalidRequest},
		{name: "metric 256 bytes", req: NamesRequest{Kind: datasource.DiscoverLabels, Metric: strings.Repeat("m", 256)}, catalog: catalogOf(1), wantLimit: 101, wantCount: 1},
		{name: "metric 257 bytes", req: NamesRequest{Kind: datasource.DiscoverLabels, Metric: strings.Repeat("m", 257)}, wantErr: "metric must be at most 256 bytes", wantIs: ErrInvalidRequest},
		{name: "label 257 bytes", req: NamesRequest{Kind: datasource.DiscoverLabelValues, Label: strings.Repeat("l", 257)}, wantErr: "label must be at most 256 bytes", wantIs: ErrInvalidRequest},
		{name: "table 257 bytes", req: NamesRequest{Kind: datasource.DiscoverLabels, Table: strings.Repeat("t", 257)}, wantErr: "table must be at most 256 bytes", wantIs: ErrInvalidRequest},
		{name: "multibyte search counts bytes", req: NamesRequest{Kind: datasource.DiscoverMetrics, Search: strings.Repeat("\u00e9", 101)}, wantErr: "search must be at most 200 bytes", wantIs: ErrInvalidRequest},
		{name: "label values need a label", req: NamesRequest{Kind: datasource.DiscoverLabelValues}, wantErr: "label is required", wantIs: ErrInvalidRequest},
		{name: "labels do not need a label", req: NamesRequest{Kind: datasource.DiscoverLabels}, catalog: catalogOf(1), wantLimit: 101, wantCount: 1},
		{name: "validation runs before the datasource lookup", req: NamesRequest{Datasource: "missing", Kind: datasource.DiscoverMetrics, Limit: 501}, wantErr: "limit must be between 1 and 500", wantIs: ErrInvalidRequest},
		{name: "unknown datasource", req: NamesRequest{Datasource: "missing", Kind: datasource.DiscoverMetrics}, wantIs: ErrUnknownDatasource},
		{name: "clickhouse labels need table or metric", typ: "clickhouse", req: NamesRequest{Kind: datasource.DiscoverLabels}, wantErr: "table or metric is required for ClickHouse labels", wantIs: ErrInvalidRequest},
		{name: "clickhouse label values need table or metric", typ: "clickhouse", req: NamesRequest{Kind: datasource.DiscoverLabelValues, Label: "l"}, wantErr: "table or metric is required for ClickHouse labels", wantIs: ErrInvalidRequest},
		{name: "clickhouse labels with a table", typ: "clickhouse", req: NamesRequest{Kind: datasource.DiscoverLabels, Table: "otel_logs"}, catalog: catalogOf(1), wantLimit: 101, wantCount: 1},
		{name: "clickhouse labels with a metric", typ: "clickhouse", req: NamesRequest{Kind: datasource.DiscoverLabels, Metric: "m"}, catalog: catalogOf(1), wantLimit: 101, wantCount: 1},
		{name: "clickhouse metrics need nothing", typ: "clickhouse", req: NamesRequest{Kind: datasource.DiscoverMetrics}, catalog: catalogOf(1), wantLimit: 101, wantCount: 1},
		{name: "prometheus labels need nothing", typ: "prometheus", req: NamesRequest{Kind: datasource.DiscoverLabels}, catalog: catalogOf(1), wantLimit: 101, wantCount: 1},
		{name: "discovery error comes back unwrapped", req: NamesRequest{Kind: datasource.DiscoverLabels}, discErr: boom, wantIs: datasource.ErrNotFound},
		{name: "nil catalog is empty", req: NamesRequest{Kind: datasource.DiscoverMetrics}, wantLimit: 101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake(cmpOr(tt.typ, "prometheus"))
			fake.onDiscover = func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
				return tt.catalog, tt.discErr
			}
			runner := namesRunner(t, fake)
			req := tt.req
			if req.Datasource == "" {
				req.Datasource = "ds"
			}
			got, err := runner.Names(context.Background(), req)

			if got := slotsInUse(runner, "ds"); got != 0 {
				t.Fatalf("%d upstream slots still held", got)
			}

			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("err = %v, want %v", err, tt.wantIs)
				}
				if tt.wantErr != "" {
					if err.Error() != tt.wantErr {
						t.Fatalf("err.Error() = %q, want %q", err.Error(), tt.wantErr)
					}
					status, code, message := Classify(err)
					if status != http.StatusBadRequest || code != "invalid_request" || message != tt.wantErr {
						t.Fatalf("Classify = (%d, %q, %q), want (400, invalid_request, %q)", status, code, message, tt.wantErr)
					}
				}
				if tt.discErr != nil && err.Error() != tt.discErr.Error() {
					t.Fatalf("err = %q, want the datasource's own error %q", err, tt.discErr)
				}
				rejected := errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrUnknownDatasource)
				if rejected && len(fake.discoverRequests()) != 0 {
					t.Fatal("discovery ran for a rejected request")
				}
				return
			}
			if err != nil {
				t.Fatalf("Names: %v", err)
			}
			reqs := fake.discoverRequests()
			if len(reqs) != 1 || reqs[0].Limit != tt.wantLimit {
				t.Fatalf("discover requests = %#v, want one with limit %d", reqs, tt.wantLimit)
			}
			if got.Truncated != tt.wantCut {
				t.Fatalf("Truncated = %v, want %v", got.Truncated, tt.wantCut)
			}
			for kind, n := range map[datasource.DiscoverKind]int{
				datasource.DiscoverMetrics:     len(got.Metrics),
				datasource.DiscoverLabels:      len(got.Labels),
				datasource.DiscoverLabelValues: len(got.Values),
			} {
				if kind == req.Kind && n != tt.wantCount {
					t.Fatalf("%s items = %d, want %d", kind, n, tt.wantCount)
				}
			}
			if got.Metrics == nil || got.Labels == nil || got.Values == nil {
				t.Fatalf("a list is nil: %#v", got)
			}
		})
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func TestNamesClipsHelpAtAUTF8Boundary(t *testing.T) {
	multibyte := "a" + strings.Repeat("\u00e9", 100)
	exact := strings.Repeat("h", maxHelpBytes)
	tests := []struct {
		name, help, want string
	}{
		{"short help is kept", "Requests served.", "Requests served."},
		{"help at the limit is kept", exact, exact},
		{"help over the limit is clipped", exact + "x", exact + "..."},
		{"a cut inside a rune backs up to its start", multibyte, "a" + strings.Repeat("\u00e9", 79) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onDiscover = func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
				return &datasource.Catalog{Metrics: []datasource.Metric{{Name: "m", Help: tt.help}}}, nil
			}
			got, err := namesRunner(t, fake).Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics})
			if err != nil {
				t.Fatal(err)
			}
			if help := got.Metrics[0].Help; help != tt.want || !utf8.ValidString(help) {
				t.Fatalf("help = %q, want %q", help, tt.want)
			}
		})
	}
}

func TestNamesPassesTheRangeToDiscoveryAndRejectsHalfOfOne(t *testing.T) {
	from := time.Date(2026, 11, 6, 6, 0, 0, 0, time.UTC)
	to := from.Add(6 * time.Hour)
	fake := newFake("loki")
	runner := namesRunner(t, fake)
	if _, err := runner.Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverLabels, Range: datasource.TimeRange{From: from, To: to}}); err != nil {
		t.Fatal(err)
	}
	sent := fake.discoverRequests()
	if len(sent) != 1 || !sent[0].Range.From.Equal(from) || !sent[0].Range.To.Equal(to) {
		t.Fatalf("discover requests = %#v, want one over %v to %v", sent, from, to)
	}
	for name, rng := range map[string]datasource.TimeRange{"from only": {From: from}, "to only": {To: to}} {
		_, err := runner.Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverLabels, Range: rng})
		if !errors.Is(err, ErrInvalidRequest) || err.Error() != "from and to must be given together" {
			t.Fatalf("%s: err = %v, want the half range rejected", name, err)
		}
	}
	if len(fake.discoverRequests()) != 1 {
		t.Fatal("discovery ran for a half range")
	}
}

func TestNamesReleasesTheSlotWhenDiscoveryIsCanceled(t *testing.T) {
	fake := newFake("prometheus")
	fake.onDiscover = func(ctx context.Context, _ datasource.DiscoverRequest) (*datasource.Catalog, error) {
		return nil, context.Canceled
	}
	runner := namesRunner(t, fake)
	if _, err := runner.Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if got := slotsInUse(runner, "ds"); got != 0 {
		t.Fatalf("%d upstream slots still held", got)
	}
}

func TestNamesRoutesScopedRequestsToTheInventory(t *testing.T) {
	families := make([]datasource.Family, 150)
	for i := range families {
		families[i] = datasource.Family{Name: fmt.Sprintf("f%03d", i), Help: strings.Repeat("h", maxHelpBytes+1)}
	}
	tests := []struct {
		name          string
		req           NamesRequest
		catalog       *datasource.Catalog
		wantKind      datasource.DiscoverKind
		wantFamilies  int
		wantTotal     int
		wantRuntime   int
		wantTruncated bool
	}{
		{name: "a scope asks for the inventory", req: NamesRequest{Kind: datasource.DiscoverMetrics, Scope: `{job="shop"}`},
			catalog: &datasource.Catalog{Families: families, FamiliesTotal: 150, RuntimeFamilies: 41}, wantKind: datasource.DiscoverInventory, wantFamilies: 150, wantTotal: 150, wantRuntime: 41},
		{name: "inventory without a scope", req: NamesRequest{Kind: datasource.DiscoverMetrics, Inventory: true},
			catalog: &datasource.Catalog{Families: families[:2], FamiliesTotal: 2}, wantKind: datasource.DiscoverInventory, wantFamilies: 2, wantTotal: 2},
		{name: "a truncated scan stays truncated", req: NamesRequest{Kind: datasource.DiscoverMetrics, Scope: `{job="shop"}`},
			catalog: &datasource.Catalog{Families: families[:3], FamiliesTotal: 9, Truncated: true}, wantKind: datasource.DiscoverInventory, wantFamilies: 3, wantTotal: 9, wantTruncated: true},
		{name: "a missing total counts the families", req: NamesRequest{Kind: datasource.DiscoverMetrics, Scope: "otel_logs"},
			catalog: &datasource.Catalog{Families: families[:4]}, wantKind: datasource.DiscoverInventory, wantFamilies: 4, wantTotal: 4},
		{name: "no scope keeps the kind", req: NamesRequest{Kind: datasource.DiscoverMetrics},
			catalog: catalogOf(1), wantKind: datasource.DiscoverMetrics},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake("prometheus")
			fake.onDiscover = func(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
				return tt.catalog, nil
			}
			req := tt.req
			req.Datasource = "ds"
			got, err := namesRunner(t, fake).Names(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			sent := fake.discoverRequests()
			if len(sent) != 1 || sent[0].Kind != tt.wantKind || sent[0].Scope != tt.req.Scope {
				t.Fatalf("discover requests = %#v, want one %s with scope %q", sent, tt.wantKind, tt.req.Scope)
			}
			if len(got.Families) != tt.wantFamilies || got.FamiliesTotal != tt.wantTotal || got.RuntimeFamilies != tt.wantRuntime || got.Truncated != tt.wantTruncated {
				t.Fatalf("families = %d, total = %d, runtime = %d, truncated = %v; want %d, %d, %d, %v",
					len(got.Families), got.FamiliesTotal, got.RuntimeFamilies, got.Truncated, tt.wantFamilies, tt.wantTotal, tt.wantRuntime, tt.wantTruncated)
			}
			for _, f := range got.Families {
				if f.Help != strings.Repeat("h", maxHelpBytes)+"..." {
					t.Fatalf("help of %s = %q, want it clipped", f.Name, f.Help)
				}
			}
		})
	}
}

func TestNamesRejectsAScopeOverTheFieldLimit(t *testing.T) {
	fake := newFake("prometheus")
	_, err := namesRunner(t, fake).Names(context.Background(), NamesRequest{Datasource: "ds", Kind: datasource.DiscoverMetrics, Scope: strings.Repeat("s", 257)})
	if !errors.Is(err, ErrInvalidRequest) || err.Error() != "scope must be at most 256 bytes" {
		t.Fatalf("err = %v, want the scope length rejected", err)
	}
	if len(fake.discoverRequests()) != 0 {
		t.Fatal("discovery ran for a rejected scope")
	}
}
