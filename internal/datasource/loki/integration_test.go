package loki_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/loki"
	"github.com/slashobserve/slashobserve/internal/tctest"
)

const (
	lokiImage = "grafana/loki:3.7.9@sha256:0db216b630ae1f348a1584c67f3cead8b4c1fc839169a39501db664744158e28"
	tenant    = "shop"
)

var (
	lokiURL  string
	pushedAt time.Time
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	flag.Parse()
	if testing.Short() {
		return m.Run()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	container, err := tctest.Retry(ctx, func() (testcontainers.Container, error) {
		return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        lokiImage,
				ExposedPorts: []string{"3100/tcp"},
				Cmd:          []string{"-config.file=/etc/loki/slashobserve.yaml"},
				Files:        []testcontainers.ContainerFile{{HostFilePath: "testdata/loki.yaml", ContainerFilePath: "/etc/loki/slashobserve.yaml", FileMode: 0o644}},
				WaitingFor:   wait.ForHTTP("/ready").WithPort("3100/tcp").WithStartupTimeout(2 * time.Minute),
			},
			Started: true,
		})
	})
	if container != nil {
		defer func() { _ = testcontainers.TerminateContainer(container) }()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "loki container:", err)
		return 1
	}
	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "loki host:", err)
		return 1
	}
	port, err := container.MappedPort(ctx, "3100/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "loki port:", err)
		return 1
	}
	lokiURL = "http://" + net.JoinHostPort(host, port.Port())
	pushedAt = time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	if err := push(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "loki push:", err)
		return 1
	}
	return m.Run()
}

func push(ctx context.Context) error {
	ns := func(offset time.Duration) string { return strconv.FormatInt(pushedAt.Add(offset).UnixNano(), 10) }
	body, err := json.Marshal(map[string]any{"streams": []map[string]any{
		{"stream": map[string]string{"service_name": "checkout", "level": "error"}, "values": [][]string{{ns(0), "payment failed"}, {ns(time.Second), "retrying payment"}}},
		{"stream": map[string]string{"service_name": "frontend"}, "values": [][]string{{ns(2 * time.Second), "level=warn msg=slow"}}},
	}})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lokiURL+"/loki/api/v1/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Scope-OrgID", tenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("push answered HTTP %d", resp.StatusCode)
	}
	return nil
}

func integrationDS(t *testing.T, tenantID string) *loki.Datasource {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	ds, err := loki.New(loki.Options{URL: lokiURL, TenantID: tenantID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func aroundPush() datasource.TimeRange {
	return datasource.TimeRange{From: pushedAt.Add(-time.Hour), To: time.Now().Add(time.Minute)}
}

func TestLokiReadsBackPushedLines(t *testing.T) {
	ds := integrationDS(t, tenant)
	ctx := context.Background()
	if err := ds.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	var frames []datasource.Frame
	deadline := time.Now().Add(30 * time.Second)
	for {
		var err error
		frames, err = ds.QueryInstant(ctx, datasource.Query{Text: `{service_name=~".+"}`}, aroundPush())
		if err != nil {
			t.Fatalf("QueryInstant: %v", err)
		}
		if len(frames) == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(frames) != 2 {
		t.Fatalf("frames = %+v, want the two pushed streams", frames)
	}
	checkout := frames[0]
	if checkout.Labels["service_name"] != "checkout" {
		t.Fatalf("first frame = %v", checkout.Labels)
	}
	if got := fieldValues(t, checkout, "Body"); !slices.Equal(got, []any{"retrying payment", "payment failed"}) {
		t.Fatalf("bodies = %v, want newest first", got)
	}
	if got := fieldValues(t, checkout, "Time"); got[1] != pushedAt.UTC() {
		t.Fatalf("times = %v, want %v last", got, pushedAt.UTC())
	}
	if got := fieldValues(t, checkout, "level"); got[0] != "error" {
		t.Fatalf("levels = %v", got)
	}
	if got := fieldValues(t, frames[1], "level"); got[0] != "warn" {
		t.Fatalf("detected level = %v, want Loki's detected_level", got)
	}

	limited, err := ds.QueryInstant(ctx, datasource.Query{Text: `{service_name="checkout"}`, Limits: datasource.Limits{MaxRows: 1}}, aroundPush())
	if err != nil || len(limited) != 1 || !slices.Equal(fieldValues(t, limited[0], "Body"), []any{"retrying payment"}) {
		t.Fatalf("row-limited query = %+v, %v", limited, err)
	}

	r := aroundPush()
	series, err := ds.QueryRange(ctx, datasource.Query{Text: `sum by (service_name) (count_over_time({service_name=~".+"}[1h]))`}, r, time.Minute)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(series) != 2 || series[0].Labels["service_name"] != "checkout" || fieldNames(series[0])[1] != "number:Value" {
		t.Fatalf("series = %+v", series)
	}
	instant, err := ds.QueryInstant(ctx, datasource.Query{Text: `sum(count_over_time({service_name="checkout"}[2h]))`}, r)
	if err != nil || len(instant) != 1 || !slices.Equal(fieldValues(t, instant[0], "Value"), []any{2.0}) {
		t.Fatalf("instant = %+v, %v", instant, err)
	}
}

func TestLokiDiscoveryAndVariables(t *testing.T) {
	ds := integrationDS(t, tenant)
	ctx := context.Background()
	labels, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels})
	if err != nil || !slices.Contains(labels.Labels, "service_name") || !slices.Contains(labels.Labels, "level") {
		t.Fatalf("labels = %+v, %v", labels, err)
	}
	values, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "service_name", Metric: `{level="error"}`})
	if err != nil || !slices.Equal(values.Values, []string{"checkout"}) {
		t.Fatalf("values = %+v, %v", values, err)
	}
	got, err := ds.VariableValues(ctx, datasource.Query{Text: "label_values(service_name)"}, aroundPush())
	if err != nil || !slices.Equal(got, []string{"checkout", "frontend"}) {
		t.Fatalf("variable values = %v, %v", got, err)
	}
}

func TestLokiTenantsAreSeparate(t *testing.T) {
	ctx := context.Background()
	frames, err := integrationDS(t, "another-team").QueryInstant(ctx, datasource.Query{Text: `{service_name=~".+"}`}, aroundPush())
	if err != nil || len(frames) != 0 {
		t.Fatalf("another tenant read %+v, %v", frames, err)
	}
	_, err = integrationDS(t, "").QueryInstant(ctx, datasource.Query{Text: `{service_name=~".+"}`}, aroundPush())
	if err == nil {
		t.Fatal("a query without a tenant succeeded against a multi-tenant Loki")
	}
}

func TestLokiRejectsBadQueries(t *testing.T) {
	ds := integrationDS(t, tenant)
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{service_name=~".+"} | json | `}, aroundPush())
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want an invalid query", err)
	}
}
