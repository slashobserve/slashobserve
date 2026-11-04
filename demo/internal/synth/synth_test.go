package synth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const seriesJSON = `[
  {"name":"node_cpu_seconds_total","kind":"counter","labels":[{"cpu":"0","mode":"idle"},{"cpu":"1","mode":"idle"}]},
  {"name":"node_load1","kind":"gauge","labels":[{"instance":"synthetic"}]},
  {"name":"node_boots","kind":"gauge","labels":[]}
]`

var sampleLine = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{([a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*",?)+\})? -?[0-9.e+]+$`)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func scrape(t *testing.T, handler http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return rec.Code, string(body)
}

func newHandler(t *testing.T, raw string) (http.Handler, *clock) {
	t.Helper()
	metrics, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1_700_000_000, 0)
	c := &clock{now: start}
	return Handler(metrics, start, c.Now), c
}

func sampleValue(t *testing.T, body, series string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if rest, ok := strings.CutPrefix(line, series+" "); ok {
			value, err := strconv.ParseFloat(rest, 64)
			if err != nil {
				t.Fatalf("value of %s: %v", series, err)
			}
			return value
		}
	}
	t.Fatalf("series %s not in output:\n%s", series, body)
	return 0
}

func TestMetricsOutputIsTextExposition(t *testing.T) {
	handler, c := newHandler(t, seriesJSON)
	c.now = c.now.Add(10 * time.Second)
	code, body := scrape(t, handler, "/metrics")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var samples int
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if strings.HasPrefix(line, "# TYPE ") {
			continue
		}
		if !sampleLine.MatchString(line) {
			t.Fatalf("line is not a sample: %q", line)
		}
		samples++
	}
	if samples != 4 {
		t.Fatalf("got %d samples, want 4 (two counter series, one labelled gauge, one unlabelled gauge)", samples)
	}
	for _, want := range []string{
		"# TYPE node_cpu_seconds_total counter\n",
		"# TYPE node_load1 gauge\n",
		`node_cpu_seconds_total{cpu="0",mode="idle"} 10` + "\n",
		`node_load1{instance="synthetic"} 1` + "\n",
		"node_boots 1\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("output missing %q:\n%s", want, body)
		}
	}
}

func TestCounterAdvancesWithElapsedTimeAndGaugeStaysFixed(t *testing.T) {
	handler, c := newHandler(t, seriesJSON)
	const counter = `node_cpu_seconds_total{cpu="0",mode="idle"}`
	const gauge = `node_load1{instance="synthetic"}`

	c.now = c.now.Add(15 * time.Second)
	_, first := scrape(t, handler, "/metrics")
	c.now = c.now.Add(15 * time.Second)
	_, second := scrape(t, handler, "/metrics")

	if got := sampleValue(t, first, counter); got != 15 {
		t.Errorf("counter after 15s = %v, want 15", got)
	}
	if got := sampleValue(t, second, counter); got != 30 {
		t.Errorf("counter after 30s = %v, want 30", got)
	}
	if a, b := sampleValue(t, first, gauge), sampleValue(t, second, gauge); a != b {
		t.Errorf("gauge changed between scrapes: %v then %v", a, b)
	}
}

func TestLabelValuesAreEscaped(t *testing.T) {
	handler, _ := newHandler(t, `[{"name":"m","kind":"gauge","labels":[{"path":"a\\b\"c\nd"}]}]`)
	_, body := scrape(t, handler, "/metrics")
	want := `m{path="a\\b\"c\nd"} 1` + "\n"
	if !strings.Contains(body, want) {
		t.Fatalf("output %q does not contain %q", body, want)
	}
}

func TestHealthzAnswers200(t *testing.T) {
	handler, _ := newHandler(t, seriesJSON)
	if code, _ := scrape(t, handler, "/healthz"); code != http.StatusOK {
		t.Fatalf("healthz status %d", code)
	}
}

func TestParseRejectsInvalidSeries(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"not a list", `{"name":"m","kind":"gauge"}`},
		{"unknown field", `[{"name":"m","kind":"gauge","help":"x"}]`},
		{"unknown kind", `[{"name":"m","kind":"histogram"}]`},
		{"missing kind", `[{"name":"m"}]`},
		{"bad metric name", `[{"name":"9m","kind":"gauge"}]`},
		{"bad label name", `[{"name":"m","kind":"gauge","labels":[{"a-b":"x"}]}]`},
		{"reserved label name", `[{"name":"m","kind":"gauge","labels":[{"__name__":"x"}]}]`},
		{"duplicate name", `[{"name":"m","kind":"gauge"},{"name":"m","kind":"counter"}]`},
		{"trailing data", `[] []`},
		{"empty input", ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.raw)); err == nil {
				t.Fatalf("Parse(%s) succeeded, want an error", tc.raw)
			}
		})
	}
}

func TestParseAcceptsEmptyList(t *testing.T) {
	metrics, err := Parse([]byte(`[]`))
	if err != nil || len(metrics) != 0 {
		t.Fatalf("Parse([]) = %v, %v", metrics, err)
	}
}

func TestRunRequiresReadableSeriesFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.json")
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"unset", nil},
		{"blank", map[string]string{"SYNTH_FILE": "  "}},
		{"missing file", map[string]string{"SYNTH_FILE": missing}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) { v, ok := tc.env[key]; return v, ok }
			if err := Run(context.Background(), lookup); err == nil {
				t.Fatal("Run succeeded, want an error")
			}
		})
	}
}

func TestRunServesMetricsUntilCancelled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "series.json")
	if err := os.WriteFile(path, []byte(seriesJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"SYNTH_FILE": path, "SHOP_ADDR": "127.0.0.1:0"}
	lookup := func(key string) (string, bool) { v, ok := env[key]; return v, ok }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, lookup) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
