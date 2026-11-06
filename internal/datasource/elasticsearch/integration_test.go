package elasticsearch_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/tctest"
)

const (
	elasticsearchImage = "docker.elastic.co/elasticsearch/elasticsearch:8.19.4@sha256:13bb550524faa2aef6950b13fb6414c73b957e97e6169e5808743c8378614599"
	opensearchImage    = "opensearchproject/opensearch:2.19.1@sha256:72fe2fc84be8295906b8efca020b46c58ed45c8da60cd9b8b49e1991e38e89a4"

	indexPattern = "app-logs-*"
	shopIndex    = "app-logs-shop"
	legacyIndex  = "app-logs-legacy"
	documents    = 200
)

type server struct {
	flavor elasticsearch.Flavor
	url    string
}

var (
	servers   = map[elasticsearch.Flavor]server{}
	indexedAt time.Time
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		return m.Run()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	indexedAt = time.Now().Add(-10 * time.Minute).Truncate(time.Millisecond)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []error
	var stops []func()
	defer func() {
		for _, stop := range stops {
			stop()
		}
	}()
	for _, flavor := range []elasticsearch.Flavor{elasticsearch.Elasticsearch, elasticsearch.OpenSearch} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, stop, err := startServer(ctx, flavor)
			mu.Lock()
			defer mu.Unlock()
			if stop != nil {
				stops = append(stops, stop)
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", flavor, err))
				return
			}
			servers[flavor] = s
		}()
	}
	wg.Wait()
	for _, err := range failures {
		fmt.Fprintln(os.Stderr, err)
	}
	if len(failures) > 0 {
		return 1
	}
	return m.Run()
}

func startServer(ctx context.Context, flavor elasticsearch.Flavor) (server, func(), error) {
	req := testcontainers.ContainerRequest{
		Image:        elasticsearchImage,
		ExposedPorts: []string{"9200/tcp"},
		Env:          map[string]string{"ES_JAVA_OPTS": "-Xms512m -Xmx512m"},
		Files:        []testcontainers.ContainerFile{{HostFilePath: "testdata/elasticsearch.yml", ContainerFilePath: "/usr/share/elasticsearch/config/elasticsearch.yml", FileMode: 0o644}},
		WaitingFor:   wait.ForHTTP("/_cluster/health?wait_for_status=yellow&timeout=1s").WithPort("9200/tcp").WithStartupTimeout(3 * time.Minute),
	}
	if flavor == elasticsearch.OpenSearch {
		req.Image = opensearchImage
		req.Env = map[string]string{"OPENSEARCH_JAVA_OPTS": "-Xms512m -Xmx512m", "DISABLE_SECURITY_PLUGIN": "true"}
		req.Files = []testcontainers.ContainerFile{{HostFilePath: "testdata/opensearch.yml", ContainerFilePath: "/usr/share/opensearch/config/opensearch.yml", FileMode: 0o644}}
	}
	container, err := tctest.Retry(ctx, func() (testcontainers.Container, error) {
		return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	})
	var stop func()
	if container != nil {
		stop = func() { _ = testcontainers.TerminateContainer(container) }
	}
	if err != nil {
		return server{}, stop, fmt.Errorf("container: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return server{}, stop, fmt.Errorf("host: %w", err)
	}
	port, err := container.MappedPort(ctx, "9200/tcp")
	if err != nil {
		return server{}, stop, fmt.Errorf("port: %w", err)
	}
	s := server{flavor: flavor, url: "http://" + net.JoinHostPort(host, port.Port())}
	if err := seed(ctx, s); err != nil {
		return server{}, stop, fmt.Errorf("seed: %w", err)
	}
	return s, stop, nil
}

func mappings(flavor elasticsearch.Flavor) (shop, legacy string) {
	flattened := "flattened"
	if flavor == elasticsearch.OpenSearch {
		flattened = "flat_object"
	}
	shop = `{"mappings":{"properties":{
		"@timestamp":{"type":"date_nanos"},
		"event":{"properties":{"ingested":{"type":"date"},"sequence":{"type":"long"}}},
		"message":{"type":"text","fields":{"keyword":{"type":"keyword","ignore_above":1024}}},
		"log":{"properties":{"level":{"type":"keyword"}}},
		"service":{"properties":{"name":{"type":"keyword"}}},
		"trace":{"properties":{"id":{"type":"keyword"}}},
		"span":{"properties":{"id":{"type":"keyword"}}},
		"labels":{"type":"` + flattened + `"},
		"http":{"properties":{"response":{"properties":{"status_code":{"type":"long"}}}}},
		"error":{"properties":{"stack_trace":{"type":"text"}}},
		"host":{"properties":{"ip":{"type":"ip"}}},
		"tags":{"type":"keyword"}
	}}}`
	legacy = `{"mappings":{"properties":{
		"@timestamp":{"type":"date_nanos"},
		"event":{"properties":{"sequence":{"type":"keyword"}}},
		"message":{"type":"text","fields":{"keyword":{"type":"keyword","ignore_above":1024}}},
		"log":{"properties":{"level":{"type":"keyword"}}},
		"service":{"properties":{"name":{"type":"keyword"}}}
	}}}`
	return shop, legacy
}

func seed(ctx context.Context, s server) error {
	shop, legacy := mappings(s.flavor)
	for _, index := range []struct{ name, body string }{{shopIndex, shop}, {legacyIndex, legacy}} {
		if err := send(ctx, http.MethodPut, s.url+"/"+index.name, "application/json", index.body); err != nil {
			return fmt.Errorf("create %s: %w", index.name, err)
		}
	}
	services := []string{"checkout", "frontend", "payments"}
	levels := []string{"info", "info", "warn", "error"}
	var bulk strings.Builder
	for i := range documents {
		at := indexedAt.Add(time.Duration(i) * time.Second).UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
		fmt.Fprintf(&bulk, `{"index":{"_index":%q}}`+"\n", shopIndex)
		fmt.Fprintf(&bulk, `{"@timestamp":%q,"event":{"ingested":%q,"sequence":%d},"message":"request %d handled","log":{"level":%q},"service":{"name":%q},"trace":{"id":"%032x"},"span":{"id":"%016x"},"labels":{"region":"eu","pod":"pod-%d"},"http":{"response":{"status_code":%d}},"error":{"stack_trace":"at line %d"},"host":{"ip":"10.0.0.%d"},"tags":["shop"]}`+"\n",
			at, at, i, i, levels[i%len(levels)], services[i%len(services)], i, i, i%3, 200+100*(i%4), i, i%250)
	}
	fmt.Fprintf(&bulk, `{"index":{"_index":%q}}`+"\n", legacyIndex)
	fmt.Fprintf(&bulk, `{"@timestamp":%q,"event":{"sequence":"legacy"},"message":"legacy line","log":{"level":"info"},"service":{"name":"legacy"}}`+"\n",
		indexedAt.UTC().Format(time.RFC3339Nano))
	return send(ctx, http.MethodPost, s.url+"/_bulk?refresh=true", "application/x-ndjson", bulk.String())
}

func send(ctx context.Context, method, target, contentType, body string) error {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s %s answered HTTP %d: %s", method, target, resp.StatusCode, body)
	}
	return nil
}

func integrationDS(t *testing.T, flavor elasticsearch.Flavor, mutate ...func(*elasticsearch.Options)) *elasticsearch.Datasource {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	s, ok := servers[flavor]
	if !ok {
		t.Fatalf("no %s server", flavor)
	}
	opts := elasticsearch.Options{Flavor: flavor, URL: s.url, Index: indexPattern}
	for _, m := range mutate {
		m(&opts)
	}
	ds, err := elasticsearch.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func aroundIndexing() datasource.TimeRange {
	return datasource.TimeRange{From: indexedAt.Add(-time.Hour), To: time.Now().Add(time.Hour)}
}

func eachFlavor(t *testing.T, test func(t *testing.T, flavor elasticsearch.Flavor)) {
	t.Helper()
	for _, flavor := range []elasticsearch.Flavor{elasticsearch.Elasticsearch, elasticsearch.OpenSearch} {
		t.Run(string(flavor), func(t *testing.T) { test(t, flavor) })
	}
}

func TestIntegrationHealthAndRoles(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		ctx := context.Background()
		if err := ds.CheckHealth(ctx); err != nil {
			t.Fatalf("CheckHealth: %v", err)
		}
		roles, err := ds.Roles(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := elasticsearch.FieldRoles{Time: "@timestamp", Message: "message", Level: "log.level", Service: "service.name", TraceID: "trace.id", SpanID: "span.id"}
		if roles != want {
			t.Fatalf("roles = %+v, want %+v", roles, want)
		}
		m, err := ds.Mapping(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if f := m.Fields["message"]; f.Kind != elasticsearch.KindText || f.Keyword != "message.keyword" {
			t.Errorf("message = %+v", f)
		}
		if f := m.Fields["labels"]; f.Kind != elasticsearch.KindFlattened {
			t.Errorf("labels = %+v", f)
		}
		if f := m.Fields["@timestamp"]; f.Kind != elasticsearch.KindDateNanos {
			t.Errorf("@timestamp = %+v", f)
		}
		if f := m.Fields["event.sequence"]; !f.Conflict {
			t.Errorf("event.sequence = %+v, want a conflict", f)
		}
		if f := m.Fields["http.response.status_code"]; f.Kind != elasticsearch.KindNumber || !f.Aggregatable {
			t.Errorf("status code = %+v", f)
		}

		wrong := elasticsearch.OpenSearch
		wantErr := datasource.ErrIsElasticsearch
		if flavor == elasticsearch.OpenSearch {
			wrong, wantErr = elasticsearch.Elasticsearch, datasource.ErrIsOpenSearch
		}
		if err := integrationDS(t, flavor, func(o *elasticsearch.Options) { o.Flavor = wrong }).CheckHealth(ctx); !errors.Is(err, wantErr) {
			t.Fatalf("wrong flavour health = %v, want %v", err, wantErr)
		}
		if err := integrationDS(t, flavor, func(o *elasticsearch.Options) { o.Index = "nothing-*" }).CheckHealth(ctx); !errors.Is(err, datasource.ErrNoIndex) {
			t.Fatalf("empty pattern health = %v, want ErrNoIndex", err)
		}
		if err := integrationDS(t, flavor, func(o *elasticsearch.Options) { o.TimeField = "message" }).CheckHealth(ctx); !errors.Is(err, datasource.ErrNoTimeField) {
			t.Fatalf("text time field health = %v, want ErrNoTimeField", err)
		}
		if err := integrationDS(t, flavor, func(o *elasticsearch.Options) { o.Fields.Level = "severity" }).CheckHealth(ctx); !errors.Is(err, datasource.ErrNoRoleField) {
			t.Fatalf("missing role health = %v, want ErrNoRoleField", err)
		}
	})
}

func TestIntegrationDiscoverAndVariables(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		ctx := context.Background()
		labels, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels})
		if err != nil {
			t.Fatal(err)
		}
		wantLabels := []string{"message", "log.level", "service.name", "http.response.status_code"}
		if flavor == elasticsearch.Elasticsearch {
			wantLabels = append(wantLabels, "labels")
		}
		for _, want := range wantLabels {
			if !slices.Contains(labels.Labels, want) {
				t.Errorf("labels %v lack %s", labels.Labels, want)
			}
		}
		if slices.Contains(labels.Labels, "error.stack_trace") {
			t.Errorf("labels %v include a text field without keyword", labels.Labels)
		}

		values, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "service.name", Range: aroundIndexing()})
		if err != nil || !slices.Equal(values.Values, []string{"checkout", "frontend", "legacy", "payments"}) {
			t.Fatalf("service values = %v, %v", values, err)
		}
		codes, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "http.response.status_code", Range: aroundIndexing()})
		if err != nil || !slices.Equal(codes.Values, []string{"200", "300", "400", "500"}) {
			t.Fatalf("status values = %v, %v", codes, err)
		}
		messages, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "message", Search: "request 1 ", Range: aroundIndexing()})
		if err != nil || !slices.Equal(messages.Values, []string{"request 1 handled"}) {
			t.Fatalf("message values = %v, %v", messages, err)
		}
		if _, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "error.stack_trace"}); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("text field values error = %v", err)
		}

		tables, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
		if err != nil || !slices.Equal(tables.Tables, []datasource.Table{{Name: legacyIndex, Kind: "index"}, {Name: shopIndex, Kind: "index"}}) {
			t.Fatalf("tables = %v, %v", tables, err)
		}
		columns, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverColumns, Search: "message"})
		if err != nil || !slices.Equal(columns.Columns, []datasource.Column{{Name: "message", Type: "text"}, {Name: "message.keyword", Type: "keyword"}}) {
			t.Fatalf("columns = %v, %v", columns, err)
		}

		sample, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverSampleRows, Limit: 3})
		if err != nil {
			t.Fatal(err)
		}
		if sample.Sample.Rows() != 3 || sample.Sample.Fields[0].Name != "Time" || sample.Sample.Fields[2].Name != "Body" {
			t.Fatalf("sample = %+v", sample.Sample)
		}
		newest := indexedAt.Add(time.Duration(documents-1) * time.Second).UTC()
		if got := sample.Sample.Fields[0].Values[0]; got != newest {
			t.Fatalf("newest sample time = %v, want %v", got, newest)
		}
		if got := sample.Sample.Fields[1].Values[0]; got != strconv.FormatInt(newest.UnixNano(), 10) {
			t.Fatalf("newest TimeNs = %v", got)
		}
		if got := sample.Sample.Fields[2].Values[0]; got != fmt.Sprintf("request %d handled", documents-1) {
			t.Fatalf("newest body = %v", got)
		}

		got, err := ds.VariableValues(ctx, datasource.Query{Text: "label_values(log.level)"}, aroundIndexing())
		if err != nil || !slices.Equal(got, []string{"error", "info", "warn"}) {
			t.Fatalf("variable values = %v, %v", got, err)
		}
		if _, err := ds.VariableValues(ctx, datasource.Query{Text: "label_values(nope)"}, aroundIndexing()); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("unknown field error = %v", err)
		}
	})
}

const shopServices = "service.name:(checkout or frontend or payments)"

func TestIntegrationQueryInstantReturnsNewestLinesFirst(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: shopServices, Limits: datasource.Limits{MaxRows: 1000}}, aroundIndexing())
		if err != nil || len(frames) != 1 {
			t.Fatalf("frames = %v, err = %v", frames, err)
		}
		f := frames[0]
		if f.Rows() != documents {
			t.Fatalf("rows = %d, want %d", f.Rows(), documents)
		}
		for i := range f.Rows() {
			want := indexedAt.Add(time.Duration(documents-1-i) * time.Second).UTC()
			if got := f.Fields[1].Values[i]; got != strconv.FormatInt(want.UnixNano(), 10) {
				t.Fatalf("row %d TimeNs = %v, want %d", i, got, want.UnixNano())
			}
		}
		if f.Meta == nil || f.Meta.Truncated || f.Meta.RowsTotal != documents {
			t.Fatalf("meta = %+v", f.Meta)
		}
	})
}

func TestIntegrationQueryInstantOnADateTimeFieldMixesIndices(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor, func(o *elasticsearch.Options) { o.TimeField = "event.ingested" })
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Limits: datasource.Limits{MaxRows: 5}}, aroundIndexing())
		if err != nil {
			t.Fatal(err)
		}
		newest := indexedAt.Add(time.Duration(documents-1) * time.Second)
		if got := frames[0].Fields[1].Values[0]; got != strconv.FormatInt(newest.UnixNano(), 10) {
			t.Fatalf("TimeNs = %v, want %d", got, newest.UnixNano())
		}
		if !frames[0].Meta.Truncated {
			t.Fatalf("meta = %+v, want Truncated at the limit", frames[0].Meta)
		}
	})
}

func TestIntegrationQueryRangeCountHasOneBucketPerStep(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		from := indexedAt.Truncate(time.Minute).Add(-time.Minute)
		r := datasource.TimeRange{From: from, To: from.Add(6 * time.Minute)}
		frames, err := ds.QueryRange(context.Background(), datasource.Query{}, r, time.Minute)
		if err != nil || len(frames) != 1 || frames[0].Name != "count" {
			t.Fatalf("frames = %v, err = %v", frames, err)
		}
		if frames[0].Rows() != 7 {
			t.Fatalf("buckets = %d, want 7", frames[0].Rows())
		}
		sum := 0.0
		for _, v := range frames[0].Fields[1].Values {
			sum += v.(float64)
		}
		if sum != documents+1 {
			t.Fatalf("counted %v documents, want %d", sum, documents+1)
		}
	})
}

func TestIntegrationDSLTermsTable(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		text := `{"aggs": {"svc": {"terms": {"field": "service.name"}}}}`
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, aroundIndexing())
		if err != nil || len(frames) != 1 {
			t.Fatalf("frames = %v, err = %v", frames, err)
		}
		counts := map[any]any{}
		for i, name := range frames[0].Fields[0].Values {
			counts[name] = frames[0].Fields[1].Values[i]
		}
		want := map[any]any{"checkout": 67.0, "frontend": 67.0, "payments": 66.0, "legacy": 1.0}
		if frames[0].Fields[0].Name != "service.name" || frames[0].Fields[1].Name != "count" || fmt.Sprint(counts) != fmt.Sprint(want) {
			t.Fatalf("table = %+v", frames[0].Fields)
		}
	})
}

func TestIntegrationLuceneAndAdhocFilter(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "lucene:log.level:error AND service.name:*"}, aroundIndexing())
		if err != nil || frames[0].Meta.RowsTotal != documents/4 {
			t.Fatalf("lucene meta = %+v, err = %v", frames[0].Meta, err)
		}
		filters := []datasource.AdhocFilter{{Key: "service.name", Op: "=", Value: "checkout"}}
		narrowed, err := ds.QueryInstant(context.Background(), datasource.Query{Filters: filters}, aroundIndexing())
		if err != nil || narrowed[0].Meta.RowsTotal != 67 {
			t.Fatalf("ad hoc meta = %+v, err = %v", narrowed[0].Meta, err)
		}
		if _, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "service.name:(a or"}, aroundIndexing()); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("bad kql error = %v", err)
		}
	})
}

func TestIntegrationQueryPage(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		ctx := context.Background()
		frames, info, err := ds.QueryPage(ctx, datasource.PageRequest{Range: aroundIndexing(), Page: 5, PageSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		if frames[0].Rows() != 1 || info.Total != documents+1 || info.TotalRelation != datasource.TotalExact || info.HasNext || info.Next != nil || info.Window != 10000 {
			t.Fatalf("rows = %d, info = %+v", frames[0].Rows(), info)
		}
		_, info, err = ds.QueryPage(ctx, datasource.PageRequest{Range: aroundIndexing(), Page: 2, PageSize: 100})
		if err != nil || !info.HasNext || info.Page != 2 {
			t.Fatalf("info = %+v, err = %v", info, err)
		}
		if _, _, err := ds.QueryPage(ctx, datasource.PageRequest{Range: aroundIndexing(), Page: 201, PageSize: 50}); !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("page past the window: %v", err)
		}
	})
}

func TestIntegrationLint(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		ctx := context.Background()
		for _, tc := range []struct {
			text     string
			severity datasource.Severity
		}{
			{"lucene:log.level:[", datasource.SeverityError},
			{`{"query": {"query_string": {"query": "log.level:["}}}`, datasource.SeverityError},
			{`{"size": 1, "foo": 2}`, datasource.SeverityError},
			{"nosuch:1", datasource.SeverityWarning},
		} {
			diags, err := ds.Lint(ctx, tc.text)
			if err != nil || len(diags) != 1 || diags[0].Severity != tc.severity {
				t.Errorf("%q: diagnostics = %+v, err = %v", tc.text, diags, err)
			}
		}
		for _, text := range []string{"lucene:log.level:error", "service.name:checkout", `{"query": {"match_all": {}}}`} {
			if diags, err := ds.Lint(ctx, text); err != nil || len(diags) != 0 {
				t.Errorf("%q: diagnostics = %+v, err = %v", text, diags, err)
			}
		}
	})
}

func TestIntegrationLogVolumeSumsEveryDocumentByLevel(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		from := indexedAt.Truncate(time.Minute).Add(-time.Minute)
		req := datasource.VolumeRequest{Range: datasource.TimeRange{From: from, To: from.Add(6 * time.Minute)}, Step: time.Minute}
		hist, err := ds.LogVolume(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		got := hist.Volume()
		var failures, warnings, infos, others int64
		for _, b := range got.Buckets {
			failures, warnings, infos, others = failures+b.Error, warnings+b.Warning, infos+b.Info, others+b.Other
		}
		if len(got.Buckets) != 6 || got.Total != documents+1 {
			t.Fatalf("%d buckets, total %d, want 6 and %d", len(got.Buckets), got.Total, documents+1)
		}
		if failures != 50 || warnings != 50 || infos != 101 || others != 0 {
			t.Fatalf("error %d, warning %d, info %d, other %d", failures, warnings, infos, others)
		}
	})
}

func TestIntegrationLogFacetsAreExact(t *testing.T) {
	eachFlavor(t, func(t *testing.T, flavor elasticsearch.Flavor) {
		ds := integrationDS(t, flavor)
		facets, err := ds.LogFacets(context.Background(), datasource.FacetRequest{Range: aroundIndexing(), Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if facets.Sampled || len(facets.Fields) < 2 || facets.Fields[0].Name != "log.level" || facets.Fields[1].Name != "service.name" {
			t.Fatalf("facets = %+v", facets)
		}
		level := facets.Fields[0]
		want := map[string]int64{"info": 101, "warn": 50, "error": 50}
		if !level.Exact || level.Truncated || level.Cardinality != 3 || len(level.Values) != 3 {
			t.Fatalf("log.level = %+v", level)
		}
		for _, v := range level.Values {
			if want[v.Value] != v.Count {
				t.Errorf("log.level %s = %d, want %d", v.Value, v.Count, want[v.Value])
			}
		}
		asked, err := ds.LogFacets(context.Background(), datasource.FacetRequest{Range: aroundIndexing(), Limit: 5, Fields: []string{"labels", "http.response.status_code", "error.stack_trace", "message"}})
		if err != nil || len(asked.Fields) != 4 {
			t.Fatalf("facets = %+v, err = %v", asked, err)
		}
		if labels := asked.Fields[0]; labels.Kind != "flattened" || (flavor == elasticsearch.Elasticsearch) != (len(labels.Values) > 0) {
			t.Fatalf("labels = %+v, want values on Elasticsearch only (flat_object is not aggregatable)", labels)
		}
		if stack := asked.Fields[2]; stack.Kind != "text" || len(stack.Values) != 0 {
			t.Fatalf("error.stack_trace = %+v", stack)
		}
	})
}
