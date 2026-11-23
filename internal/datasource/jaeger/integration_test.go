package jaeger_test

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
	"github.com/slashobserve/slashobserve/internal/datasource/jaeger"
	"github.com/slashobserve/slashobserve/internal/tctest"
)

const (
	jaegerImage   = "quay.io/jaegertracing/jaeger:2.21.0@sha256:3d0ac795ff98aa04d1be04311d2dac6c25b4bfc8322dc02e53bc5b170c5018c3"
	pushedTraceID = "5b8efff798038103d269b633813fc60c"
	pushedRootID  = "eee19b7ec3c1b174"
	pushedChildID = "eee19b7ec3c1b173"
)

var (
	jaegerURL string
	otlpURL   string
	sentAt    time.Time
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
				Image:        jaegerImage,
				ExposedPorts: []string{"16686/tcp", "4318/tcp"},
				WaitingFor:   wait.ForHTTP("/api/v3/services").WithPort("16686/tcp").WithStartupTimeout(2 * time.Minute),
			},
			Started: true,
		})
	})
	if container != nil {
		defer func() { _ = testcontainers.TerminateContainer(container) }()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "jaeger container:", err)
		return 1
	}
	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jaeger host:", err)
		return 1
	}
	api, err := container.MappedPort(ctx, "16686/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "jaeger port:", err)
		return 1
	}
	otlp, err := container.MappedPort(ctx, "4318/tcp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "jaeger otlp port:", err)
		return 1
	}
	jaegerURL = "http://" + net.JoinHostPort(host, api.Port())
	otlpURL = "http://" + net.JoinHostPort(host, otlp.Port())
	sentAt = time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	if err := send(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "jaeger send:", err)
		return 1
	}
	return m.Run()
}

func str(key, value string) map[string]any {
	return map[string]any{"key": key, "value": map[string]string{"stringValue": value}}
}

func send(ctx context.Context) error {
	ns := func(offset time.Duration) string { return strconv.FormatInt(sentAt.Add(offset).UnixNano(), 10) }
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
	return postTraces(ctx, body)
}

func integrationDS(t *testing.T) *jaeger.Datasource {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	ds, err := jaeger.New(jaeger.Options{URL: jaegerURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func aroundSend() datasource.TimeRange {
	return datasource.TimeRange{From: sentAt.Add(-time.Hour), To: time.Now().Add(time.Minute)}
}

func eventually(t *testing.T, ds *jaeger.Datasource, text string) datasource.Frame {
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

func TestJaegerReadsBackSentSpans(t *testing.T) {
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

	missing, err := ds.QueryInstant(context.Background(), datasource.Query{Text: "ffffffffffffffffffffffffffffffff"}, aroundSend())
	if err != nil || len(missing) != 0 {
		t.Fatalf("unknown trace = %+v, %v", missing, err)
	}
}

func TestJaegerSearchFiltersByServiceOperationTagAndDuration(t *testing.T) {
	ds := integrationDS(t)
	found := eventually(t, ds, `service=checkout`)
	if got := fieldValues(t, found, "TraceId"); !slices.Equal(got, []any{pushedTraceID}) {
		t.Fatalf("search trace ids = %v", got)
	}
	if got := fieldValues(t, found, "SpanName"); !slices.Equal(got, []any{"POST /checkout"}) {
		t.Fatalf("search names = %v", got)
	}
	if got := fieldValues(t, found, "Spans"); !slices.Equal(got, []any{datasource.Number(2)}) {
		t.Fatalf("span counts = %v", got)
	}
	if got := fieldValues(t, found, "Errors"); !slices.Equal(got, []any{datasource.Number(1)}) {
		t.Fatalf("error counts = %v", got)
	}
	for _, text := range []string{
		`service=checkout operation="POST /checkout"`,
		`service=checkout http.method=POST`,
		`service=checkout error=true`,
		`service=checkout minDuration=10ms maxDuration=1s`,
	} {
		if f := eventually(t, ds, text); !slices.Equal(fieldValues(t, f, "TraceId"), []any{pushedTraceID}) {
			t.Errorf("%s: trace ids = %v", text, fieldValues(t, f, "TraceId"))
		}
	}
	for _, text := range []string{`service=nobody`, `service=checkout http.method=GET`, `service=checkout minDuration=1s`} {
		frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, aroundSend())
		if err != nil || len(frames) != 0 {
			t.Errorf("%s: frames = %+v, %v; want none", text, frames, err)
		}
	}
}

func TestJaegerDiscovery(t *testing.T) {
	ds := integrationDS(t)
	eventually(t, ds, `service=checkout`)
	ctx := context.Background()
	services, err := ds.Services(ctx, datasource.ServiceRequest{Limit: 50})
	if err != nil || !slices.Contains(services.Values, "checkout") {
		t.Fatalf("services = %+v, %v", services, err)
	}
	ops, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "operation", Metric: "service=checkout"})
	if err != nil || !slices.Contains(ops.Values, "POST /checkout") || !slices.Contains(ops.Values, "charge") {
		t.Fatalf("operations = %+v, %v", ops, err)
	}
	tables, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
	if err != nil || tables.Sample == nil || !slices.Contains(fieldValues(t, *tables.Sample, "service"), any("checkout")) {
		t.Fatalf("tables = %+v, %v", tables, err)
	}
}

func TestJaegerReadsDependencies(t *testing.T) {
	ds := integrationDS(t)
	eventually(t, ds, `service=checkout`)
	if _, err := ds.Dependencies(context.Background(), aroundSend()); err != nil {
		t.Fatalf("Dependencies: %v", err)
	}
}

func TestJaegerRejectsBadQueries(t *testing.T) {
	ds := integrationDS(t)
	ctx := context.Background()
	_, err := ds.QueryInstant(ctx, datasource.Query{Text: `checkout`}, aroundSend())
	if !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("err = %v, want an invalid query", err)
	}
	now := time.Now()
	_, err = ds.QueryInstant(ctx, datasource.Query{Text: `service=checkout`}, datasource.TimeRange{From: now.Add(time.Hour), To: now.Add(2 * time.Hour)})
	if !errors.Is(err, datasource.ErrRangeInFuture) {
		t.Fatalf("future range: err = %v", err)
	}
}

func TestJaegerSearchPageHoldsTheNewestTraces(t *testing.T) {
	ds := integrationDS(t)
	ctx := context.Background()
	for i := range 4 {
		id := fmt.Sprintf("%016x%016x", 0xabc0+i, 0xdef0+i)
		if err := sendRoot(ctx, id, "paged", sentAt.Add(time.Duration(i)*10*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		frames, info, err := ds.QueryPage(ctx, datasource.PageRequest{Query: datasource.Query{Text: `service=paged`}, Range: aroundSend(), Page: 1, PageSize: 3})
		if err != nil {
			t.Fatal(err)
		}
		if len(frames) == 1 && frames[0].Rows() == 3 && info.HasNext {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the search returned no full page within a minute")
		}
		time.Sleep(time.Second)
	}
}

func sendRoot(ctx context.Context, traceID, service string, start time.Time) error {
	body, err := json.Marshal(map[string]any{"resourceSpans": []map[string]any{{
		"resource": map[string]any{"attributes": []any{str("service.name", service)}},
		"scopeSpans": []map[string]any{{"spans": []map[string]any{{
			"traceId": traceID, "spanId": traceID[:16], "name": "GET /paged", "kind": 2,
			"startTimeUnixNano": strconv.FormatInt(start.UnixNano(), 10), "endTimeUnixNano": strconv.FormatInt(start.Add(5*time.Millisecond).UnixNano(), 10),
		}}}},
	}}})
	if err != nil {
		return err
	}
	return postTraces(ctx, body)
}

func postTraces(ctx context.Context, body []byte) error {
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
