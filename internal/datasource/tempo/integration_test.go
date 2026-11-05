package tempo_test

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
	"github.com/slashobserve/slashobserve/internal/datasource/tempo"
	"github.com/slashobserve/slashobserve/internal/tctest"
)

const (
	tempoImage    = "grafana/tempo:3.1.1@sha256:64e2fbd56d9c2b3412a4b430a58ea59e91a670b1e75f920a0c702213ad145044"
	pushedTraceID = "5b8efff798038103d269b633813fc60c"
	pushedRootID  = "eee19b7ec3c1b174"
	pushedChildID = "eee19b7ec3c1b173"
)

var (
	tempoURL string
	sentAt   time.Time
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
				Image:        tempoImage,
				ExposedPorts: []string{"3200/tcp", "4318/tcp"},
				Cmd:          []string{"-config.file=/etc/tempo/slashobserve.yaml"},
				Files:        []testcontainers.ContainerFile{{HostFilePath: "testdata/tempo.yaml", ContainerFilePath: "/etc/tempo/slashobserve.yaml", FileMode: 0o644}},
				WaitingFor:   wait.ForHTTP("/ready").WithPort("3200/tcp").WithStartupTimeout(2 * time.Minute),
			},
			Started: true,
		})
	})
	if container != nil {
		defer func() { _ = testcontainers.TerminateContainer(container) }()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempo container:", err)
		return 1
	}
	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempo host:", err)
		return 1
	}
	api, err := container.MappedPort(ctx, "3200/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempo port:", err)
		return 1
	}
	otlp, err := container.MappedPort(ctx, "4318/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempo otlp port:", err)
		return 1
	}
	tempoURL = "http://" + net.JoinHostPort(host, api.Port())
	sentAt = time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	if err := send(ctx, "http://"+net.JoinHostPort(host, otlp.Port())); err != nil {
		fmt.Fprintln(os.Stderr, "tempo send:", err)
		return 1
	}
	return m.Run()
}

func send(ctx context.Context, otlpURL string) error {
	ns := func(offset time.Duration) string { return strconv.FormatInt(sentAt.Add(offset).UnixNano(), 10) }
	str := func(key, value string) map[string]any {
		return map[string]any{"key": key, "value": map[string]string{"stringValue": value}}
	}
	body, err := json.Marshal(map[string]any{"resourceSpans": []map[string]any{{
		"resource": map[string]any{"attributes": []any{str("service.name", "checkout")}},
		"scopeSpans": []map[string]any{{"scope": map[string]string{"name": "shop"}, "spans": []map[string]any{
			{
				"traceId": pushedTraceID, "spanId": pushedRootID, "name": "POST /checkout", "kind": 2,
				"startTimeUnixNano": ns(0), "endTimeUnixNano": ns(50 * time.Millisecond),
				"attributes": []any{str("http.method", "POST")}, "status": map[string]any{"code": 2},
			},
			{
				"traceId": pushedTraceID, "spanId": pushedChildID, "parentSpanId": pushedRootID, "name": "charge", "kind": 3,
				"startTimeUnixNano": ns(10 * time.Millisecond), "endTimeUnixNano": ns(40 * time.Millisecond),
			},
		}}},
	}}})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, otlpURL+"/v1/traces", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("otlp answered HTTP %d", resp.StatusCode)
	}
	return nil
}

func integrationDS(t *testing.T) *tempo.Datasource {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	ds, err := tempo.New(tempo.Options{URL: tempoURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func aroundSend() datasource.TimeRange {
	return datasource.TimeRange{From: sentAt.Add(-time.Hour), To: time.Now().Add(time.Minute)}
}

func eventually(t *testing.T, ds *tempo.Datasource, text string) datasource.Frame {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, aroundSend())
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if len(frames) == 1 {
			return frames[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s returned no rows within a minute", text)
		}
		time.Sleep(time.Second)
	}
}

func TestTempoReadsBackSentSpans(t *testing.T) {
	ds := integrationDS(t)
	if err := ds.CheckHealth(context.Background()); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	f := eventually(t, ds, pushedTraceID)
	if got := fieldValues(t, f, "SpanId"); !slices.Equal(got, []any{pushedRootID, pushedChildID}) {
		t.Fatalf("span ids = %v", got)
	}
	if got := fieldValues(t, f, "ParentSpanId"); !slices.Equal(got, []any{"", pushedRootID}) {
		t.Fatalf("parents = %v", got)
	}
	if got := fieldValues(t, f, "TraceId"); got[0] != pushedTraceID {
		t.Fatalf("trace ids = %v", got)
	}
	if got := fieldValues(t, f, "StatusCode"); !slices.Equal(got, []any{"Error", "Unset"}) {
		t.Fatalf("statuses = %v", got)
	}
	if got := fieldValues(t, f, "Duration"); !slices.Equal(got, []any{int64(50 * time.Millisecond), int64(30 * time.Millisecond)}) {
		t.Fatalf("durations = %v", got)
	}
	if got := fieldValues(t, f, "StartTime"); got[0] != sentAt.UTC() {
		t.Fatalf("start = %v, want %v", got[0], sentAt.UTC())
	}
	if got := fieldValues(t, f, "SpanAttributes"); got[0] != `{"http.method":"POST"}` {
		t.Fatalf("attributes = %v", got)
	}
	if got := fieldValues(t, f, "ServiceName"); got[1] != "checkout" {
		t.Fatalf("services = %v", got)
	}

	found := eventually(t, ds, `{ resource.service.name = "checkout" }`)
	if got := fieldValues(t, found, "TraceId"); !slices.Equal(got, []any{pushedTraceID}) {
		t.Fatalf("search trace ids = %v", got)
	}
	if got := fieldValues(t, found, "SpanName"); !slices.Equal(got, []any{"POST /checkout"}) {
		t.Fatalf("search names = %v", got)
	}

	missing, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "ffffffffffffffffffffffffffffffff"}, aroundSend())
	if err != nil || len(missing) != 0 {
		t.Fatalf("unknown trace = %+v, %v", missing, err)
	}
}

func TestTempoDiscovery(t *testing.T) {
	ds := integrationDS(t)
	eventually(t, ds, `{ resource.service.name = "checkout" }`)
	ctx := context.Background()
	tags, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels})
	if err != nil || !slices.Contains(tags.Labels, "resource.service.name") || !slices.Contains(tags.Labels, "span.http.method") {
		t.Fatalf("tags = %+v, %v", tags, err)
	}
	values, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "resource.service.name"})
	if err != nil || !slices.Equal(values.Values, []string{"checkout"}) {
		t.Fatalf("values = %+v, %v", values, err)
	}
}

func TestTempoRejectsBadQueries(t *testing.T) {
	ds := integrationDS(t)
	_, err := ds.QueryInstant(context.Background(), datasource.Query{Text: `{ resource.service.name = }`}, aroundSend())
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want an invalid query", err)
	}
}
