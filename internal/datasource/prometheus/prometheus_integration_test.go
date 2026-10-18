package prometheus_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/bcrypt"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/prometheus"
	"github.com/slashobserve/slashobserve/internal/tctest"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	prometheusImage = "prom/prometheus:v3.15.0@sha256:efd719c99d83b060d9daefdcf00360461adf279f45ef5391f8d111892118753e"
	basicAuthUser   = "slash"
	requestsMetric  = "prometheus_http_requests_total"
)

type environment struct {
	url      string
	host     string
	port     string
	password string
}

var env *environment

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
	container, e, err := startPrometheus(ctx)
	if container != nil {
		defer func() {
			_ = testcontainers.TerminateContainer(container)
		}()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "prometheus container:", err)
		return 1
	}
	env = e
	return m.Run()
}

func startPrometheus(ctx context.Context) (testcontainers.Container, *environment, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, nil, err
	}
	password := hex.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return nil, nil, err
	}
	scrapeConfig := fmt.Sprintf("global:\n  scrape_interval: 1s\nscrape_configs:\n  - job_name: prometheus\n    basic_auth:\n      username: %s\n      password: %s\n    static_configs:\n      - targets: [\"localhost:9090\"]\n", basicAuthUser, password)
	webConfig := fmt.Sprintf("basic_auth_users:\n  %s: %q\n", basicAuthUser, string(hash))
	container, err := tctest.Retry(ctx, func() (testcontainers.Container, error) {
		return testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        prometheusImage,
				ExposedPorts: []string{"9090/tcp"},
				Cmd: []string{
					"--config.file=/etc/prometheus/prometheus.yml",
					"--web.config.file=/etc/prometheus/web.yml",
					"--storage.tsdb.path=/prometheus",
				},
				Files: []testcontainers.ContainerFile{
					{Reader: strings.NewReader(scrapeConfig), ContainerFilePath: "/etc/prometheus/prometheus.yml", FileMode: 0o644},
					{Reader: strings.NewReader(webConfig), ContainerFilePath: "/etc/prometheus/web.yml", FileMode: 0o644},
				},
				WaitingFor: wait.ForHTTP("/-/healthy").WithPort("9090/tcp").WithBasicAuth(basicAuthUser, password).WithStartupTimeout(2 * time.Minute),
			},
			Started: true,
		})
	})
	if err != nil {
		return container, nil, err
	}
	host, err := container.Host(ctx)
	if err != nil {
		return container, nil, err
	}
	port, err := container.MappedPort(ctx, "9090/tcp")
	if err != nil {
		return container, nil, err
	}
	e := &environment{url: "http://" + net.JoinHostPort(host, port.Port()), host: host, port: port.Port(), password: password}
	if err := e.waitReady(ctx); err != nil {
		return container, nil, err
	}
	return container, e, nil
}

func (e *environment) options() prometheus.Options {
	return prometheus.Options{
		URL:               e.url,
		BasicAuthUser:     basicAuthUser,
		BasicAuthPassword: datasource.NewSecret(e.password),
	}
}

func (e *environment) waitReady(ctx context.Context) error {
	ds, err := prometheus.New(e.options())
	if err != nil {
		return err
	}
	defer func() { _ = ds.Close() }()
	_, _ = ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels})
	deadline := time.Now().Add(60 * time.Second)
	for {
		now := time.Now()
		r := datasource.TimeRange{From: now.Add(-time.Minute), To: now}
		up, upErr := ds.QueryInstant(ctx, datasource.Query{Text: "up"}, r)
		reqs, reqErr := ds.QueryInstant(ctx, datasource.Query{Text: requestsMetric}, r)
		rates, rateErr := ds.QueryRange(ctx, datasource.Query{Text: "rate(" + requestsMetric + "[10s])"}, r, 5*time.Second)
		if upErr == nil && reqErr == nil && rateErr == nil && len(up) == 1 && up[0].Fields[1].Values[0] == float64(1) && len(reqs) >= 2 && len(rates) >= 1 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("prometheus not ready: up=%d series, %s=%d series, rate=%d series: %w", len(up), requestsMetric, len(reqs), len(rates), errors.Join(upErr, reqErr, rateErr))
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func newDS(t *testing.T, mutate ...func(*prometheus.Options)) *prometheus.Datasource {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker")
	}
	opts := env.options()
	for _, m := range mutate {
		m(&opts)
	}
	ds, err := prometheus.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = ds.Close() })
	return ds
}

func lint(ctx context.Context, t *testing.T, ds *prometheus.Datasource, query string) []datasource.Diagnostic {
	t.Helper()
	diags, err := ds.Lint(ctx, query)
	if err != nil {
		t.Fatalf("Lint(%q): %v", query, err)
	}
	return diags
}

func lastMinutes(n int) datasource.TimeRange {
	now := time.Now()
	return datasource.TimeRange{From: now.Add(-time.Duration(n) * time.Minute), To: now}
}

func hasNone(t *testing.T, msg string, forbidden ...string) {
	t.Helper()
	for _, f := range forbidden {
		if strings.Contains(msg, f) {
			t.Errorf("error message %q leaks %q", msg, f)
		}
	}
}

func TestCheckHealth(t *testing.T) {
	ctx := context.Background()
	t.Run("ok", func(t *testing.T) {
		if err := newDS(t).CheckHealth(ctx); err != nil {
			t.Fatalf("CheckHealth: %v", err)
		}
	})
	t.Run("wrong password", func(t *testing.T) {
		wrong := "not-the-password"
		ds := newDS(t, func(o *prometheus.Options) { o.BasicAuthPassword = datasource.NewSecret(wrong) })
		err := ds.CheckHealth(ctx)
		if !errors.Is(err, datasource.ErrUnavailable) {
			t.Fatalf("want ErrUnavailable, got %v", err)
		}
		hasNone(t, err.Error(), env.host, env.port, "http", wrong, env.password)
	})
	t.Run("closed port", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_, port, _ := net.SplitHostPort(addr)
		_ = l.Close()
		ds := newDS(t, func(o *prometheus.Options) { o.URL = "http://" + addr })
		err = ds.CheckHealth(ctx)
		if !errors.Is(err, datasource.ErrUnavailable) {
			t.Fatalf("want ErrUnavailable, got %v", err)
		}
		hasNone(t, err.Error(), "127.0.0.1", port, "http", env.password)
	})
}

func TestQueryRange(t *testing.T) {
	ds := newDS(t)
	frames, err := ds.QueryRange(context.Background(), datasource.Query{
		Text: `sum by (handler) (rate(prometheus_http_requests_total[$__rate_interval]))`,
	}, lastMinutes(2), 5*time.Second)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if len(frames) < 1 {
		t.Fatal("want at least 1 frame")
	}
	for _, f := range frames {
		if f.Fields[0].Type != datasource.FieldTime {
			t.Errorf("first field type %q, want time", f.Fields[0].Type)
		}
		for _, v := range f.Fields[1].Values {
			if _, ok := v.(float64); v != nil && !ok {
				t.Errorf("value %v (%T) is neither float64 nor nil", v, v)
			}
		}
	}
	if _, err := ds.QueryRange(context.Background(), datasource.Query{Text: "sum("}, lastMinutes(2), 5*time.Second); !errors.Is(err, datasource.ErrInvalidQuery) {
		t.Fatalf("sum( : want ErrInvalidQuery, got %v", err)
	}
}

func TestQueryInstant(t *testing.T) {
	ds := newDS(t)
	ctx := context.Background()
	frames, err := ds.QueryInstant(ctx, datasource.Query{Text: "up"}, lastMinutes(1))
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(frames) != 1 || frames[0].Labels["job"] != "prometheus" || frames[0].Fields[1].Values[0] != float64(1) {
		t.Fatalf("up: unexpected frames %+v", frames)
	}
	frames, err = ds.QueryInstant(ctx, datasource.Query{Text: "1+1"}, lastMinutes(1))
	if err != nil {
		t.Fatalf("1+1: %v", err)
	}
	if len(frames) != 1 || frames[0].Rows() != 1 || frames[0].Fields[1].Values[0] != float64(2) {
		t.Fatalf("1+1: unexpected frames %+v", frames)
	}
}

func TestMacros(t *testing.T) {
	ds := newDS(t)
	text := `count_over_time(up[$__range]) + 0*count_over_time(up[$__rate_interval]) + 0*count_over_time(up[$__interval])`
	for _, m := range spec.Macros(spec.Prometheus) {
		if !strings.Contains(text, "$"+m.Name) {
			t.Errorf("macro $%s is not covered by the test query", m.Name)
		}
	}
	frames, err := ds.QueryInstant(context.Background(), datasource.Query{Text: text}, lastMinutes(5))
	if err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(frames))
	}
}

func TestVariables(t *testing.T) {
	ds := newDS(t)
	ctx := context.Background()
	query := func(text string, vars ...datasource.Variable) ([]datasource.Frame, error) {
		return ds.QueryInstant(ctx, datasource.Query{Text: text, Variables: vars}, lastMinutes(1))
	}
	expectFrames := func(t *testing.T, want int, text string, vars ...datasource.Variable) {
		t.Helper()
		frames, err := query(text, vars...)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if len(frames) != want {
			t.Fatalf("%s: want %d frames, got %d", text, want, len(frames))
		}
	}
	t.Run("selected value", func(t *testing.T) {
		expectFrames(t, 1, `up{job=~"$job"}`, datasource.Variable{Name: "job", Values: []string{"prometheus"}})
	})
	t.Run("all", func(t *testing.T) {
		expectFrames(t, 1, `up{job=~"$job"}`, datasource.Variable{Name: "job", All: true})
	})
	for _, hostile := range []string{`}) or vector(1) #`, `.*`, `a|b`} {
		t.Run("hostile "+hostile, func(t *testing.T) {
			expectFrames(t, 0, `up{job=~"$job"}`, datasource.Variable{Name: "job", Values: []string{hostile}})
		})
	}
	t.Run("quote in value", func(t *testing.T) {
		expectFrames(t, 0, `up{job="$job"}`, datasource.Variable{Name: "job", Values: []string{`pro"metheus`}})
	})
	t.Run("multiple values in equality", func(t *testing.T) {
		_, err := query(`up{job="$job"}`, datasource.Variable{Name: "job", Values: []string{"a", "b"}})
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("want ErrInvalidQuery, got %v", err)
		}
	})
	t.Run("injection in grouping", func(t *testing.T) {
		_, err := query(`sum by ($l) (up)`, datasource.Variable{Name: "l", Values: []string{"pod) or vector(1"}})
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("want ErrInvalidQuery, got %v", err)
		}
	})
}

func TestLimits(t *testing.T) {
	ctx := context.Background()
	t.Run("max series", func(t *testing.T) {
		ds := newDS(t, func(o *prometheus.Options) { o.Limits = datasource.Limits{MaxSeries: 1} })
		_, err := ds.QueryRange(ctx, datasource.Query{Text: requestsMetric}, lastMinutes(2), 15*time.Second)
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
	})
	t.Run("step too small", func(t *testing.T) {
		ds := newDS(t, func(o *prometheus.Options) { o.URL = "http://127.0.0.1:1" })
		_, err := ds.QueryRange(ctx, datasource.Query{Text: "up"}, lastMinutes(2), time.Millisecond)
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded before any request, got %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		ds := newDS(t)
		_, err := ds.QueryInstant(ctx, datasource.Query{Text: "up", Limits: datasource.Limits{Timeout: time.Nanosecond}}, lastMinutes(1))
		if !errors.Is(err, datasource.ErrTimeout) {
			t.Fatalf("want ErrTimeout, got %v", err)
		}
	})
	t.Run("looser request limits do not raise configured ones", func(t *testing.T) {
		ds := newDS(t, func(o *prometheus.Options) { o.Limits = datasource.Limits{MaxSeries: 1} })
		_, err := ds.QueryRange(ctx, datasource.Query{Text: requestsMetric, Limits: datasource.Limits{MaxSeries: 5000}}, lastMinutes(2), 15*time.Second)
		if !errors.Is(err, datasource.ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
	})
}

func TestLint(t *testing.T) {
	ds := newDS(t)
	ctx := context.Background()
	t.Run("valid", func(t *testing.T) {
		if diags := lint(ctx, t, ds, `sum(rate(prometheus_http_requests_total[5m]))`); len(diags) != 0 {
			t.Fatalf("want no diagnostics, got %+v", diags)
		}
	})
	t.Run("parse error", func(t *testing.T) {
		query := `sum(rate(up[5m])`
		diags := lint(ctx, t, ds, query)
		if len(diags) != 1 || diags[0].Severity != datasource.SeverityError {
			t.Fatalf("want exactly one error, got %+v", diags)
		}
		if d := diags[0]; d.Start != 16 || d.Start > d.End || d.End > len(query) {
			t.Errorf("bad range %d..%d for %q, want start at the unclosed paren (16)", d.Start, d.End, query)
		}
	})
	t.Run("unknown metric", func(t *testing.T) {
		query := `rate(does_not_exist_total[5m])`
		diags := lint(ctx, t, ds, query)
		if len(diags) != 1 || diags[0].Severity != datasource.SeverityError || !strings.Contains(diags[0].Message, "does_not_exist_total") {
			t.Fatalf("want one error naming the metric, got %+v", diags)
		}
		if got := query[diags[0].Start:diags[0].End]; got != "does_not_exist_total" {
			t.Fatalf("diagnostic covers %q", got)
		}
	})
	t.Run("macro", func(t *testing.T) {
		if diags := lint(ctx, t, ds, `rate(up[$__rate_interval])`); len(diags) != 0 {
			t.Fatalf("want no diagnostics, got %+v", diags)
		}
	})
	t.Run("variable", func(t *testing.T) {
		diags := lint(ctx, t, ds, `rate(up[$win])`)
		if len(diags) == 0 {
			t.Fatal("want warnings")
		}
		for _, d := range diags {
			if d.Severity != datasource.SeverityWarning {
				t.Errorf("want only warnings, got %+v", d)
			}
		}
	})
}

func TestDiscover(t *testing.T) {
	ds := newDS(t)
	ctx := context.Background()
	t.Run("metrics", func(t *testing.T) {
		cat, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverMetrics, Search: "http_requests"})
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(cat.Metrics, func(m datasource.Metric) bool { return m.Name == requestsMetric })
		if i < 0 {
			t.Fatalf("%s missing from %+v", requestsMetric, cat.Metrics)
		}
		if m := cat.Metrics[i]; m.Type != "counter" || m.Help == "" {
			t.Fatalf("unexpected metadata %+v", m)
		}
	})
	t.Run("labels", func(t *testing.T) {
		cat, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabels, Metric: requestsMetric})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(cat.Labels, "handler") || slices.Contains(cat.Labels, "__name__") {
			t.Fatalf("unexpected labels %v", cat.Labels)
		}
	})
	t.Run("label values", func(t *testing.T) {
		cat, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "handler"})
		if err != nil {
			t.Fatal(err)
		}
		if len(cat.Values) == 0 {
			t.Fatal("want handler values")
		}
	})
	t.Run("invalid label", func(t *testing.T) {
		_, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverLabelValues, Label: "bad-name"})
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("want ErrInvalidQuery, got %v", err)
		}
	})
	t.Run("unsupported kind", func(t *testing.T) {
		_, err := ds.Discover(ctx, datasource.DiscoverRequest{Kind: datasource.DiscoverTables})
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("want ErrInvalidQuery, got %v", err)
		}
	})
}

func TestVariableValues(t *testing.T) {
	ds := newDS(t)
	ctx := context.Background()
	r := lastMinutes(10)
	t.Run("selector and label", func(t *testing.T) {
		values, err := ds.VariableValues(ctx, datasource.Query{Text: "label_values(prometheus_http_requests_total, handler)"}, r)
		if err != nil {
			t.Fatal(err)
		}
		if len(values) == 0 || !slices.IsSorted(values) {
			t.Fatalf("want sorted non-empty values, got %v", values)
		}
	})
	t.Run("label only", func(t *testing.T) {
		values, err := ds.VariableValues(ctx, datasource.Query{Text: "label_values(job)"}, r)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(values, "prometheus") {
			t.Fatalf("want prometheus in %v", values)
		}
	})
	t.Run("unsupported form", func(t *testing.T) {
		_, err := ds.VariableValues(ctx, datasource.Query{Text: "query_result(up)"}, r)
		if !errors.Is(err, datasource.ErrInvalidQuery) {
			t.Fatalf("want ErrInvalidQuery, got %v", err)
		}
	})
}
