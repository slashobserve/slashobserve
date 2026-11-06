package tempo_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
)

const (
	tagsPath     = "/api/v2/search/tags"
	servicesPath = "/api/v2/search/tag/resource.service.name/values"
	spanNamePath = "/api/v2/search/tag/name/values"
	tagsBody     = `{"scopes":[{"name":"intrinsic","tags":["name","duration"]},{"name":"span","tags":["http.route"]},{"name":"resource","tags":["service.name"]},{"name":"span","tags":["http.route"]}]}`
)

func tagValuesBody(values ...string) string {
	items := make([]string, len(values))
	for i, v := range values {
		items[i] = fmt.Sprintf(`{"type":"string","value":%q}`, v)
	}
	return `{"tagValues":[` + strings.Join(items, ",") + `]}`
}

func spanNameQueries(fake *fakeTempo) []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	var out []string
	for _, r := range fake.requests {
		if r.path == spanNamePath {
			out = append(out, r.params.Get("q"))
		}
	}
	return out
}

func TestDiscoverSpansListsServicesAndSpanNames(t *testing.T) {
	ds, fake := serve(t, map[string]reply{
		tagsPath:     {0, tagsBody},
		servicesPath: {0, tagValuesBody("frontend", "checkout", "checkout")},
		spanNamePath: {0, tagValuesBody("POST /checkout", "charge")},
	})
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"duration", "name", "resource.service.name", "span.http.route"}; !reflect.DeepEqual(got.Labels, want) {
		t.Fatalf("attributes = %v, want %v", got.Labels, want)
	}
	fields := got.Sample.Fields
	if len(fields) != 2 || fields[0].Name != "service" || fields[1].Name != "span_name" || fields[0].Type != datasource.FieldString {
		t.Fatalf("fields = %+v", fields)
	}
	if want := []any{"checkout", "checkout", "frontend", "frontend"}; !reflect.DeepEqual(fields[0].Values, want) {
		t.Fatalf("services = %v, want %v", fields[0].Values, want)
	}
	if want := []any{"POST /checkout", "charge", "POST /checkout", "charge"}; !reflect.DeepEqual(fields[1].Values, want) {
		t.Fatalf("span names = %v, want %v", fields[1].Values, want)
	}
	want := []string{`{ resource.service.name = "checkout" }`, `{ resource.service.name = "frontend" }`}
	if got := spanNameQueries(fake); !reflect.DeepEqual(slices.Sorted(slices.Values(got)), want) {
		t.Fatalf("span name q = %v, want %v in any order", got, want)
	}
	if got.Truncated {
		t.Fatal("truncated without a cut")
	}
}

func TestDiscoverSpansEscapesTheServiceName(t *testing.T) {
	ds, fake := serve(t, map[string]reply{
		tagsPath:     {0, tagsBody},
		servicesPath: {0, tagValuesBody(`a "quoted" \ name`)},
		spanNamePath: {0, tagValuesBody()},
	})
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables}); err != nil {
		t.Fatal(err)
	}
	if got, want := spanNameQueries(fake), []string{`{ resource.service.name = "a \"quoted\" \\ name" }`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("q = %v, want %v", got, want)
	}
}

func TestDiscoverSpansCapsServicesAndSpanNames(t *testing.T) {
	services := make([]string, 31)
	for i := range services {
		services[i] = fmt.Sprintf("svc-%02d", i)
	}
	ds, fake := serve(t, map[string]reply{
		tagsPath:     {0, tagsBody},
		servicesPath: {0, tagValuesBody(services...)},
		spanNamePath: {0, tagValuesBody("a")},
	})
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(spanNameQueries(fake)) != 30 || len(got.Sample.Fields[0].Values) != 30 {
		t.Fatalf("truncated %v after %d span name queries", got.Truncated, len(spanNameQueries(fake)))
	}

	names := make([]string, 51)
	for i := range names {
		names[i] = fmt.Sprintf("span-%02d", i)
	}
	ds, _ = serve(t, map[string]reply{
		tagsPath:     {0, tagsBody},
		servicesPath: {0, tagValuesBody("checkout")},
		spanNamePath: {0, tagValuesBody(names...)},
	})
	got, err = ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Sample.Fields[1].Values) != 50 {
		t.Fatalf("truncated %v with %d span names", got.Truncated, len(got.Sample.Fields[1].Values))
	}
}

func TestDiscoverSpansFailsOnAnUpstreamError(t *testing.T) {
	ds, _ := serve(t, map[string]reply{
		tagsPath:     {0, tagsBody},
		servicesPath: {0, tagValuesBody("checkout")},
		spanNamePath: {500, "boom"},
	})
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables}); !errors.Is(err, datasource.ErrUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestDiscoverLabelsSendsTheFilter(t *testing.T) {
	ds, fake := serve(t, map[string]reply{tagsPath: {0, tagsBody}})
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Metric: `{ resource.service.name = "checkout" }`}); err != nil {
		t.Fatal(err)
	}
	if got := fake.last(t).params.Get("q"); got != `{ resource.service.name = "checkout" }` {
		t.Fatalf("q = %q", got)
	}
}

func TestDiscoverTagsSendTheRange(t *testing.T) {
	from := time.Date(2026, 11, 6, 6, 0, 0, 0, time.UTC)
	rng := datasource.TimeRange{From: from, To: from.Add(6*time.Hour + 500*time.Millisecond)}
	ds, fake := serve(t, map[string]reply{tagsPath: {0, tagsBody}, servicesPath: {0, tagValuesBody("checkout")}})
	for _, req := range []datasource.DiscoverRequest{
		{Kind: datasource.DiscoverLabels, Range: rng},
		{Kind: datasource.DiscoverLabelValues, Label: "resource.service.name", Range: rng},
	} {
		if _, err := ds.Discover(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if got := fake.last(t).params; got.Get("start") != "1793944800" || got.Get("end") != "1793966401" {
			t.Fatalf("%s: sent start %q end %q, want unix seconds with the end rounded up", req.Kind, got.Get("start"), got.Get("end"))
		}
	}
	if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverLabels}); err != nil {
		t.Fatal(err)
	}
	if got := fake.last(t).params; got.Has("start") || got.Has("end") {
		t.Fatalf("a request without a range sent start %q end %q", got.Get("start"), got.Get("end"))
	}
}

func TestDiscoverSpansKeepsServiceOrderAndRunsAtMostFourAtOnce(t *testing.T) {
	services := make([]string, 8)
	for i := range services {
		services[i] = fmt.Sprintf("svc-%d", i)
	}
	var (
		mu       sync.Mutex
		inFlight int
		peak     int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case tagsPath:
			_, _ = w.Write([]byte(tagsBody))
		case servicesPath:
			_, _ = w.Write([]byte(tagValuesBody(services...)))
		case spanNamePath:
			mu.Lock()
			inFlight++
			peak = max(peak, inFlight)
			mu.Unlock()
			q := r.URL.Query().Get("q")
			var i int
			if _, err := fmt.Sscanf(q, `{ resource.service.name = "svc-%d" }`, &i); err != nil {
				http.Error(w, "unknown service", http.StatusBadRequest)
				return
			}
			time.Sleep(time.Duration(8-i) * 5 * time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
			_, _ = w.Write([]byte(tagValuesBody("span of " + services[i])))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ds, err := tempo.New(tempo.Options{URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	got, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
	if err != nil {
		t.Fatal(err)
	}
	wantServices := make([]any, len(services))
	wantNames := make([]any, len(services))
	for i, service := range services {
		wantServices[i], wantNames[i] = service, "span of "+service
	}
	if !reflect.DeepEqual(got.Sample.Fields[0].Values, wantServices) || !reflect.DeepEqual(got.Sample.Fields[1].Values, wantNames) {
		t.Fatalf("services = %v, span names = %v; want service order kept", got.Sample.Fields[0].Values, got.Sample.Fields[1].Values)
	}
	if peak > 4 || peak < 2 {
		t.Fatalf("peak in-flight span name calls = %d, want between 2 and 4", peak)
	}
}

func TestDiscoverKeepsTheUnsupportedKinds(t *testing.T) {
	ds, _ := serve(t, nil)
	for _, kind := range []datasource.DiscoverKind{datasource.DiscoverColumns, datasource.DiscoverSampleRows, datasource.DiscoverMetrics, datasource.DiscoverInventory} {
		if _, err := ds.Discover(context.Background(), datasource.DiscoverRequest{Kind: kind}); err == nil || !strings.Contains(err.Error(), "not supported by tempo") {
			t.Errorf("%s: err = %v", kind, err)
		}
	}
}
