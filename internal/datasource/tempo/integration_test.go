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
	"strings"
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
	otlpURL  string
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
	otlpURL = "http://" + net.JoinHostPort(host, otlp.Port())
	if err := send(ctx, otlpURL); err != nil {
		fmt.Fprintln(os.Stderr, "tempo send:", err)
		return 1
	}
	return m.Run()
}

func str(key, value string) map[string]any {
	return map[string]any{"key": key, "value": map[string]string{"stringValue": value}}
}

func sendRoot(ctx context.Context, traceID, service string, start time.Time) error {
	body, err := json.Marshal(map[string]any{"resourceSpans": []map[string]any{{
		"resource": map[string]any{"attributes": []any{str("service.name", service)}},
		"scopeSpans": []map[string]any{{"scope": map[string]string{"name": "shop"}, "spans": []map[string]any{{
			"traceId": traceID, "spanId": traceID[:16], "name": "GET /newest", "kind": 2,
			"startTimeUnixNano": strconv.FormatInt(start.UnixNano(), 10), "endTimeUnixNano": strconv.FormatInt(start.Add(5*time.Millisecond).UnixNano(), 10),
		}}}},
	}}})
	if err != nil {
		return err
	}
	return postTraces(ctx, otlpURL, body)
}

func postTraces(ctx context.Context, otlpURL string, body []byte) error {
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

func send(ctx context.Context, otlpURL string) error {
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
	return postTraces(ctx, otlpURL, body)
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

func TestTempoSearchPageHoldsTheNewestTraces(t *testing.T) {
	ds := integrationDS(t)
	ctx := context.Background()
	base := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	var newest []any
	for i := range 12 {
		id := fmt.Sprintf("%016x%016x", 0xabc0+i, 0xdef0+i)
		if err := sendRoot(ctx, id, "newest-first", base.Add(time.Duration(i)*10*time.Second)); err != nil {
			t.Fatal(err)
		}
		if i >= 9 {
			newest = append([]any{id}, newest...)
		}
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		frames, _, err := ds.QueryPage(ctx, datasource.PageRequest{Query: datasource.Query{Text: `{ resource.service.name = "newest-first" }`}, Range: aroundSend(), Page: 1, PageSize: 3})
		if err != nil {
			t.Fatal(err)
		}
		if len(frames) == 1 && frames[0].Rows() == 3 {
			if got := fieldValues(t, frames[0], "TraceId"); !slices.Equal(got, newest) {
				t.Fatalf("trace ids = %v, want the three newest %v", got, newest)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the search returned no page within a minute")
		}
		time.Sleep(time.Second)
	}
}

func TestTempoSaysWhatIsWrongWithARangeOrAQuery(t *testing.T) {
	ds := integrationDS(t)
	ctx := context.Background()
	now := time.Now()
	ok := `{ resource.service.name = "x" }`
	_, err := ds.QueryInstant(ctx, datasource.Query{Text: ok}, datasource.TimeRange{From: now.Add(time.Hour), To: now.Add(2 * time.Hour)})
	if !errors.Is(err, datasource.ErrRangeInFuture) {
		t.Errorf("future range: err = %v, want the future range kind", err)
	}
	_, err = ds.QueryInstant(ctx, datasource.Query{Text: ok}, datasource.TimeRange{From: now.Add(-30 * 24 * time.Hour), To: now})
	if !errors.Is(err, datasource.ErrRangeTooLong) || !strings.Contains(err.Error(), "Tempo searches at most 7 days at a time") {
		t.Errorf("long range: err = %v, want the range limit kind naming 7 days", err)
	}
	_, err = ds.QueryInstant(ctx, datasource.Query{Text: "{ name = "}, aroundSend())
	if !errors.Is(err, datasource.ErrInvalidQuery) || !strings.Contains(err.Error(), `Invalid TraceQL at line 1, column 9 after "{ name =": unexpected end of the query.`) {
		t.Errorf("parse error: err = %v, want a cleaned position message", err)
	}
	t.Logf("parse error reads: %v", err)
}

func TestTempoTraceDurationsAreExactSpanDurations(t *testing.T) {
	ds := integrationDS(t)
	eventually(t, ds, pushedTraceID)
	req := datasource.TraceDurationRequest{Query: `{ resource.service.name = "checkout" }`, GroupBy: "name", Range: aroundSend()}
	var got datasource.TraceDurations
	deadline := time.Now().Add(60 * time.Second)
	for {
		var err error
		if got, err = ds.TraceDurations(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if got.Window.Spans == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	if !got.Exact || got.Window.Spans != 2 || got.Window.Errors != 1 || got.Window.P50 != 50 || got.Window.P99 != 50 {
		t.Fatalf("window = %+v, exact %v", got.Window, got.Exact)
	}
	byName := map[string]datasource.DurationQuantiles{}
	for _, g := range got.Groups {
		byName[g.Value] = g
	}
	if byName["charge"].P99 != 30 || byName["POST /checkout"].P50 != 50 || byName["POST /checkout"].Errors != 1 {
		t.Fatalf("groups = %+v", got.Groups)
	}
}
