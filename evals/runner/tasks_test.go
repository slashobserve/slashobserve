package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nodeExporterFull    = "rfmoz/node-exporter-full.json"
	nodeExporterFullOld = "rfmoz/node-exporter-full-old.json"
	smallImport         = "altinity/test_timeFilterMs_and_timeSeriesMs.json"
)

var (
	testCorpusRoot    = filepath.Join("..", "..", "pkg", "grafana", "testdata", "corpus")
	cachedPlans       sync.Map
	cachedSampleTasks sync.Map
	metricNameRe      = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNameRe       = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

func planOf(t *testing.T, suite string) *planner {
	t.Helper()
	if cached, ok := cachedPlans.Load(suite); ok {
		return cached.(*planner)
	}
	p, err := planCorpus(testCorpusRoot, nil, suite)
	if err != nil {
		t.Fatal(err)
	}
	cachedPlans.Store(suite, p)
	return p
}

func shippedSampleTasks(t *testing.T, suite string) []Scenario {
	t.Helper()
	if cached, ok := cachedSampleTasks.Load(suite); ok {
		return cached.([]Scenario)
	}
	shipped, err := loadSample(filepath.Join("..", "corpus-sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := corpusTasks(testCorpusRoot, shipped, suite)
	if err != nil {
		t.Fatalf("the shipped sample does not match the corpus: %v", err)
	}
	cachedSampleTasks.Store(suite, tasks)
	return tasks
}

type synthOutput struct {
	summary synthSummary
	body    []byte
}

var generatedSynth = sync.OnceValues(func() (synthOutput, error) {
	out, err := os.MkdirTemp("", "evals-synth-")
	if err != nil {
		return synthOutput{}, err
	}
	defer func() { _ = os.RemoveAll(out) }()
	summary, err := writeSynth(out)
	if err != nil {
		return synthOutput{}, err
	}
	body, err := os.ReadFile(filepath.Join(out, synthSeriesFile)) //nolint:gosec // a path inside the temporary directory made above
	return synthOutput{summary: summary, body: body}, err
})

func synthOf(t *testing.T) synthOutput {
	t.Helper()
	useCorpus(t)
	got, err := generatedSynth()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func taskOf(t *testing.T, p *planner, taskName, key string) task {
	t.Helper()
	dir, file := filepath.Split(key)
	id := corpusIDPrefix + "-" + taskName + "-" + slug(strings.TrimSuffix(dir, "/")) + "-" + slug(strings.TrimSuffix(file, ".json"))
	for _, tk := range p.tasks {
		if tk.scenario.ID == id {
			return tk
		}
	}
	t.Fatalf("no %s task %s for %s", taskName, id, key)
	return task{}
}

func useFixtureDirs(t *testing.T) {
	t.Helper()
	useCorpus(t)
	previousE2E, previousEvals := e2eFixtureDir, evalsFixtureDir
	e2eFixtureDir = filepath.Join("..", "..", "e2e", "fixtures", "llm")
	evalsFixtureDir = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { e2eFixtureDir, evalsFixtureDir = previousE2E, previousEvals })
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRenameSubjectBreaksTheFirstMetricOrTable(t *testing.T) {
	tests := []struct {
		name, field, text, want string
		ok                      bool
	}{
		{"bare metric", fieldExpr, `up`, `up_broken_by_eval`, true},
		{"rate with matchers", fieldExpr, `sum(rate(http_requests_total{job="x"}[5m])) by (le)`, `sum(rate(http_requests_total_broken_by_eval{job="x"}[5m])) by (le)`, true},
		{"aggregation with a grouping clause first", fieldExpr, `sum by (instance) (node_load1)`, `sum by (instance) (node_load1_broken_by_eval)`, true},
		{"grafana interval variable", fieldExpr, `rate(node_cpu_seconds_total{mode="idle"}[$__rate_interval])`, `rate(node_cpu_seconds_total_broken_by_eval{mode="idle"}[$__rate_interval])`, true},
		{"quantile over buckets", fieldExpr, `histogram_quantile(0.95, sum by (le) (rate(x_bucket[5m])))`, `histogram_quantile(0.95, sum by (le) (rate(x_bucket_broken_by_eval[5m])))`, true},
		{"variable scalar argument", fieldExpr, `topk($n, foo)`, `topk($n, foo_broken_by_eval)`, true},
		{"string arguments are skipped", fieldExpr, `label_replace(foo, "a", "bar", "c", "d")`, `label_replace(foo_broken_by_eval, "a", "bar", "c", "d")`, true},
		{"vector matching clauses are skipped", fieldExpr, `sum without (a, b) (foo{c="d"}) / on (x) group_left bar`, `sum without (a, b) (foo_broken_by_eval{c="d"}) / on (x) group_left bar`, true},
		{"variable metric name", fieldExpr, `$metric{a="b"}`, ``, false},
		{"only numbers", fieldExpr, `1 + 2`, ``, false},
		{"table", fieldRawSQL, `SELECT 1 FROM otel_logs WHERE x`, `SELECT 1 FROM otel_logs_broken_by_eval WHERE x`, true},
		{"database qualified table", fieldRawSQL, `SELECT * FROM default.otel_logs`, `SELECT * FROM default.otel_logs_broken_by_eval`, true},
		{"variable database", fieldRawSQL, `SELECT * FROM ${database}.otel_logs`, `SELECT * FROM ${database}.otel_logs_broken_by_eval`, true},
		{"variable table", fieldRawSQL, `SELECT * FROM ${query_log} WHERE x`, `SELECT * FROM ${query_log}_broken_by_eval WHERE x`, true},
		{"a literal table wins over a variable one", fieldRawSQL, `SELECT * FROM $t a JOIN (SELECT * FROM real_t) b`, `SELECT * FROM $t a JOIN (SELECT * FROM real_t_broken_by_eval) b`, true},
		{"sql without a table", fieldRawSQL, `SELECT 1`, ``, false},
		{"query field holding sql", fieldQuery, `select x from t`, `select x from t_broken_by_eval`, true},
		{"query field holding sql without a table", fieldQuery, `SELECT (${type})`, ``, false},
		{"query field holding promql", fieldQuery, `up`, `up_broken_by_eval`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := renameSubject(tt.field, tt.text)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("renameSubject(%s, %q) = %q, %t; want %q, %t", tt.field, tt.text, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBreakPanelChangesExactlyOnePanelOfNodeExporterFull(t *testing.T) {
	useCorpus(t)
	source, err := readCorpus(nodeExporterFull)
	if err != nil {
		t.Fatal(err)
	}
	broken, err := breakPanel(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(broken, []byte(brokenSuffix)) {
		t.Fatal("the broken dashboard does not carry the suffix")
	}
	pristine, err := convertSource(source, stackOptions())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := convertSource(broken, stackOptions())
	if err != nil {
		t.Fatal(err)
	}
	id, err := changedPanel(spec.Diff(pristine, changed))
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := changed.FindDataPanel(id)
	if !ok {
		t.Fatalf("the changed panel %s is not a data panel", id)
	}
	if !strings.Contains(entry.Panel.Queries[0].Query, brokenSuffix) && !slices.ContainsFunc(entry.Panel.Queries, func(q spec.Query) bool { return strings.Contains(q.Query, brokenSuffix) }) {
		t.Fatalf("panel %s has no broken query", id)
	}
	if got := panelCount(pristine); got < 100 {
		t.Fatalf("the pristine dashboard has %d panels, want the full dashboard", got)
	}
}

func panelCount(d *spec.Dashboard) int { return len(d.AllPanels()) }

func panelMetrics(t *testing.T, panel *spec.Panel) []string {
	t.Helper()
	var metrics []string
	for _, q := range panel.Queries {
		sels, err := exprSelectors(q.Query)
		if err != nil {
			t.Fatalf("panel %s: %v", panel.ID, err)
		}
		for _, s := range sels {
			metrics = append(metrics, s.metric)
		}
	}
	return metrics
}

func TestNodeExporterFullTasksTargetOnlyMetricsEveryHostServes(t *testing.T) {
	useCorpus(t)
	source, err := readCorpus(nodeExporterFull)
	if err != nil {
		t.Fatal(err)
	}
	broken, err := breakPanel(source)
	if err != nil {
		t.Fatal(err)
	}
	pristine, err := convertSource(source, stackOptions())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := convertSource(broken, stackOptions())
	if err != nil {
		t.Fatal(err)
	}
	id, err := changedPanel(spec.Diff(pristine, changed))
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := pristine.FindDataPanel(id)
	if !ok {
		t.Fatalf("panel %s is not a data panel", id)
	}
	metrics := panelMetrics(t, entry.Panel)
	if len(metrics) == 0 {
		t.Fatalf("panel %s names no metric", id)
	}
	for _, m := range metrics {
		if !hostIndependentMetrics[m] {
			t.Errorf("the fix target %q (%s) reads %s, which depends on the host kernel", entry.Panel.Title, id, m)
		}
	}

	dashboards, err := loadCorpus(corpusDir, map[string]bool{nodeExporterFull: true})
	if err != nil {
		t.Fatal(err)
	}
	a, err := analyze(dashboards[0])
	if err != nil {
		t.Fatal(err)
	}
	if a.subject == nil || !a.subject.servedByEveryHost() {
		t.Errorf("the add task would name %+v, which depends on the host kernel", a.subject)
	}
}

func TestBreakPanelPrefersAMetricEveryHostServes(t *testing.T) {
	dashboard := func(exprs ...string) []byte {
		panels := make([]string, len(exprs))
		for i, expr := range exprs {
			panels[i] = fmt.Sprintf(`{"id":%d,"type":"stat","title":"P%d","gridPos":{"x":%d,"y":0,"w":6,"h":4},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"expr":%q,"refId":"A","datasource":{"type":"prometheus","uid":"prom"}}]}`, i+1, i+1, i*6, expr)
		}
		return []byte(`{"title":"t","schemaVersion":39,"panels":[` + strings.Join(panels, ",") + `]}`)
	}
	cases := map[string]struct {
		exprs []string
		want  string
	}{
		"a core metric after a host dependent one": {[]string{`rate(node_pressure_cpu_waiting_seconds_total[5m])`, `node_load1`}, "node_load1_broken_by_eval"},
		"a core metric inside a call":              {[]string{`up`, `100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m]))`}, "node_cpu_seconds_total_broken_by_eval"},
		"the first core metric wins":               {[]string{`node_memory_MemTotal_bytes`, `node_load1`}, "node_memory_MemTotal_bytes_broken_by_eval"},
		"no core metric keeps the first panel":     {[]string{`rate(node_pressure_cpu_waiting_seconds_total[5m])`, `node_hwmon_temp_celsius`}, "node_pressure_cpu_waiting_seconds_total_broken_by_eval"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			broken, err := breakPanel(dashboard(tc.exprs...))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(broken, []byte(tc.want)) || bytes.Count(broken, []byte(brokenSuffix)) != 1 {
				t.Fatalf("want only %s broken: %s", tc.want, broken)
			}
		})
	}
}

func TestSubjectPrefersAMetricEveryHostServes(t *testing.T) {
	var a analyzed
	a.offerSubject(subject{kind: spec.Prometheus, name: "node_pressure_cpu_waiting_seconds_total"})
	a.offerSubject(subject{kind: spec.Prometheus, name: "node_hwmon_temp_celsius"})
	if a.subject.name != "node_pressure_cpu_waiting_seconds_total" {
		t.Fatalf("subject %+v: the first host dependent metric stays when nothing better exists", a.subject)
	}
	a.offerSubject(subject{kind: spec.Prometheus, name: "node_load1"})
	a.offerSubject(subject{kind: spec.Prometheus, name: "node_cpu_seconds_total"})
	if a.subject.name != "node_load1" {
		t.Fatalf("subject %+v, want the first core metric", a.subject)
	}
	var tables analyzed
	tables.offerSubject(subject{kind: spec.ClickHouse, name: "otel_logs"})
	tables.offerSubject(subject{kind: spec.Prometheus, name: "node_load1"})
	if tables.subject.name != "otel_logs" {
		t.Fatalf("subject %+v: a table subject must not be replaced", tables.subject)
	}
}

func TestBreakPanelRefusesWhatItCannotBreak(t *testing.T) {
	useCorpus(t)
	if _, err := breakPanel([]byte(`not json`)); err == nil {
		t.Fatal("breakPanel accepted invalid JSON")
	}
	source, err := readCorpus("altinity/interval_variables_dashboard.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := breakPanel(source); err == nil {
		t.Fatal("breakPanel broke a dashboard whose only tables are plugin macros")
	}
}

func TestBreakPanelIsTheSeedBreaker(t *testing.T) {
	if panelBreaker == nil {
		t.Fatal("no panel breaker is wired in")
	}
	source := []byte(`{"title":"t","schemaVersion":39,"panels":[{"id":1,"type":"stat","title":"A","gridPos":{"x":0,"y":0,"w":6,"h":4},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"expr":"up","refId":"A","datasource":{"type":"prometheus","uid":"prom"}}]}]}`)
	broken, err := panelBreaker(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(broken, []byte("up_broken_by_eval")) {
		t.Fatalf("the seed breaker did not rename the metric: %s", broken)
	}
}

func TestParserSafeAndSelectors(t *testing.T) {
	tests := []struct {
		name, query string
		metrics     []string
		fails       bool
	}{
		{"rate interval and variable matchers", `rate(node_cpu_seconds_total{instance="$node",job=~"$job"}[$__rate_interval])`, []string{"node_cpu_seconds_total"}, false},
		{"braced variable with a format", `sum(foo{a=~"${x:regex}"})`, []string{"foo"}, false},
		{"subquery step variable", `max_over_time(foo[1h:$__interval])`, []string{"foo"}, false},
		{"bare variable argument", `topk($n, bar)`, []string{"bar"}, false},
		{"two selectors", `a / on(x) b`, []string{"a", "b"}, false},
		{"syntax error", `sum(`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sels, err := exprSelectors(tt.query)
			if (err != nil) != tt.fails {
				t.Fatalf("exprSelectors(%q) error = %v, want failure %t", tt.query, err, tt.fails)
			}
			var got []string
			for _, s := range sels {
				got = append(got, s.metric)
			}
			if !slices.Equal(got, tt.metrics) {
				t.Fatalf("metrics = %v, want %v", got, tt.metrics)
			}
		})
	}
}

func TestVariableSelectorsReadLabelValues(t *testing.T) {
	sels, err := variableSelectors(`label_values(node_uname_info{job="$job"}, nodename)`)
	if err != nil || len(sels) != 1 || sels[0].metric != "node_uname_info" || sels[0].label != "nodename" {
		t.Fatalf("variableSelectors = %+v, %v", sels, err)
	}
	if sels, err := variableSelectors(`label_values(job)`); err != nil || len(sels) != 0 {
		t.Fatalf("a label-only query gave %+v, %v", sels, err)
	}
	if _, err := variableSelectors(`query_result(up)`); err == nil {
		t.Fatal("a query_result variable query was accepted")
	}
}

func TestChTablesNamesPlainTablesOnly(t *testing.T) {
	got := chTables("SELECT 1 FROM ${database}.otel_logs a WHERE x IN (SELECT 1 FROM default.otel_traces) AND y IN (SELECT 1 FROM ${var})")
	if want := []string{"otel_logs", "otel_traces"}; !slices.Equal(got, want) {
		t.Fatalf("chTables = %v, want %v", got, want)
	}
}

func TestChTablesSkipsSystemDatabases(t *testing.T) {
	got := chTables("SELECT database FROM system.tables WHERE name IN ('otel_logs') UNION ALL SELECT 1 FROM INFORMATION_SCHEMA.columns UNION ALL SELECT 1 FROM otel_logs")
	if want := []string{"otel_logs"}; !slices.Equal(got, want) {
		t.Fatalf("chTables = %v, want %v", got, want)
	}
}

func TestOtelExplorerRebuildTasksExpectOnlyTheirOwnTable(t *testing.T) {
	useCorpus(t)
	want := map[string]string{
		"clickhouse-datasource/otel-logs-explorer.json":   "otel_logs",
		"clickhouse-datasource/otel-traces-explorer.json": "otel_traces",
	}
	dashboards, err := loadCorpus(corpusDir, map[string]bool{"clickhouse-datasource/otel-logs-explorer.json": true, "clickhouse-datasource/otel-traces-explorer.json": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboards) != len(want) {
		t.Fatalf("loaded %d dashboards, want %d", len(dashboards), len(want))
	}
	for _, d := range dashboards {
		a, err := analyze(d)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(a.tables, []string{want[d.key]}) {
			t.Errorf("%s tables = %v, want [%s]", d.key, a.tables, want[d.key])
		}
	}
}

func TestSeriesBookChoosesLabelValuesAndSeries(t *testing.T) {
	book := &seriesBook{metrics: map[string]*metricBook{}}
	match := labels.MustNewMatcher
	book.add(selector{metric: "foo_total", matchers: []*labels.Matcher{
		match(labels.MatchEqual, "mode", "idle"),
		match(labels.MatchRegexp, "device", "sda|sdb"),
		match(labels.MatchRegexp, "instance", ".+"),
		match(labels.MatchEqual, "job", variableStandIn),
		match(labels.MatchNotEqual, "region", "x"),
		match(labels.MatchEqual, "gone", ""),
	}, label: "extra"})
	book.add(selector{metric: "up"})
	book.add(selector{metric: "http_requests_total"})
	book.add(selector{metric: "http_request_duration_seconds_bucket"})
	book.add(selector{metric: "lat_bucket", matchers: []*labels.Matcher{match(labels.MatchEqual, "le", "0.3")}})
	book.add(selector{metric: "plain_metric"})
	metrics := map[string]synthMetric{}
	for _, m := range book.list() {
		metrics[m.Name] = m
	}
	for _, excluded := range []string{"up", "http_requests_total", "http_request_duration_seconds_bucket"} {
		if _, ok := metrics[excluded]; ok {
			t.Fatalf("%s must not be generated", excluded)
		}
	}
	foo := metrics["foo_total"]
	if foo.Kind != synthKindCounter || len(foo.Labels) != 2 {
		t.Fatalf("foo_total = %+v", foo)
	}
	first := foo.Labels[0]
	for label, want := range map[string]string{"mode": "idle", "device": "sda", "instance": synthLabelValue, "job": synthLabelValue, "region": synthLabelValue, "extra": synthLabelValue} {
		if first[label] != want {
			t.Errorf("label %s = %q, want %q", label, first[label], want)
		}
	}
	if _, ok := first["gone"]; ok {
		t.Error("a label matched against the empty string must not be generated")
	}
	if foo.Labels[1]["device"] != "sda-2" || foo.Labels[1]["job"] != synthLabelValue || foo.Labels[1]["mode"] != "idle" {
		t.Errorf("the second series should differ in one non-job label: %v", foo.Labels[1])
	}
	lat := metrics["lat_bucket"]
	if lat.Kind != synthKindCounter || len(lat.Labels) != 1*len(bucketBounds)+1 {
		t.Fatalf("lat_bucket has %d series, want %d", len(lat.Labels), len(bucketBounds)+1)
	}
	les := map[string]bool{}
	for _, set := range lat.Labels {
		les[set[labelLE]] = true
	}
	if !les["0.3"] || !les["+Inf"] {
		t.Errorf("le values = %v, want the literal 0.3 and +Inf", les)
	}
	if plain := metrics["plain_metric"]; plain.Kind != synthKindGauge || len(plain.Labels) != 1 || len(plain.Labels[0]) != 0 {
		t.Errorf("a metric without labels should be one series: %+v", plain)
	}
}

func TestGenerateSynthCoversTheCorpusAndLeavesTheShopAlone(t *testing.T) {
	synth := synthOf(t)
	summary := synth.summary
	dec := json.NewDecoder(bytes.NewReader(synth.body))
	dec.DisallowUnknownFields()
	var metrics []synthMetric
	if err := dec.Decode(&metrics); err != nil {
		t.Fatal(err)
	}
	byName := map[string]synthMetric{}
	series := 0
	for _, m := range metrics {
		if !metricNameRe.MatchString(m.Name) || (m.Kind != synthKindCounter && m.Kind != synthKindGauge) || len(m.Labels) == 0 {
			t.Fatalf("invalid metric %+v", m)
		}
		if _, dup := byName[m.Name]; dup {
			t.Fatalf("duplicate metric %s", m.Name)
		}
		byName[m.Name] = m
		series += len(m.Labels)
		for _, set := range m.Labels {
			for label := range set {
				if !labelNameRe.MatchString(label) || strings.HasPrefix(label, "__") {
					t.Fatalf("%s has invalid label %q", m.Name, label)
				}
			}
		}
		if strings.HasSuffix(m.Name, bucketSuffix) && !slices.ContainsFunc(m.Labels, func(set map[string]string) bool { return set[labelLE] == "+Inf" }) {
			t.Fatalf("%s has no +Inf bucket", m.Name)
		}
	}
	if summary.Metrics != len(metrics) || summary.Series != series {
		t.Fatalf("summary %+v does not match %d metrics and %d series", summary, len(metrics), series)
	}
	if cpu := byName["node_cpu_seconds_total"]; cpu.Kind != synthKindCounter {
		t.Fatalf("node_cpu_seconds_total = %+v, want a counter", cpu)
	}
	if load := byName["node_load1"]; load.Kind != synthKindGauge {
		t.Fatalf("node_load1 = %+v, want a gauge", load)
	}
	if info, ok := byName["node_uname_info"]; !ok || info.Labels[0][labelJob] == "" {
		t.Fatalf("node_uname_info = %+v, want a job label", info)
	}
	for name := range byName {
		if name == "up" || name == "http_requests_total" || strings.HasPrefix(name, "http_request_duration_seconds") {
			t.Fatalf("%s must be left to the shop", name)
		}
	}
	if len(byName) < 100 {
		t.Fatalf("only %d metrics generated", len(byName))
	}
}

func TestCorpusTasksPerSuite(t *testing.T) {
	counts := func(p *planner) map[string]int {
		out := map[string]int{}
		for _, tk := range p.tasks {
			out[strings.Split(tk.scenario.ID, "-")[1]]++
		}
		return out
	}
	replayPlan, livePlan := planOf(t, suiteReplay), planOf(t, suiteLive)
	replayCounts, liveCounts := counts(replayPlan), counts(livePlan)
	if replayCounts[taskRebuild] != len(largeDashboards) {
		t.Fatalf("replay has %d rebuild tasks, want one for each of the %d large dashboards", replayCounts[taskRebuild], len(largeDashboards))
	}
	if liveCounts[taskPaste] != 101 || replayCounts[taskPaste] != 101 {
		t.Fatalf("paste tasks = %d live, %d replay; want one for each of the 101 dashboards", liveCounts[taskPaste], replayCounts[taskPaste])
	}
	if liveCounts[taskRebuild] != 101 {
		t.Fatalf("rebuild tasks = %d, want 101", liveCounts[taskRebuild])
	}
	if replayCounts[taskFix] != liveCounts[taskFix] || replayCounts[taskAdd] != liveCounts[taskAdd] || liveCounts[taskFix] < 90 || liveCounts[taskAdd] < 90 {
		t.Fatalf("fix and add tasks = %v replay, %v live", replayCounts, liveCounts)
	}
	fits := 0
	for _, tk := range livePlan.tasks {
		if tk.scenario.Kind == kindChatImport && tk.scenario.Expect.FailureCode == "" {
			fits++
		}
	}
	if fits != 3 {
		t.Fatalf("%d paste tasks fit the prompt cap, want 3", fits)
	}
	ids := map[string]bool{}
	for _, plan := range []*planner{replayPlan, livePlan} {
		for _, tk := range plan.tasks {
			sc := tk.scenario
			if !idPattern.MatchString(sc.ID) || !strings.HasPrefix(sc.ID, corpusIDPrefix+"-") {
				t.Fatalf("task id %q is not a valid corpus id", sc.ID)
			}
			if !slices.Contains(sc.Suites, plan.suite) {
				t.Fatalf("%s is in suites %v but was planned for %s", sc.ID, sc.Suites, plan.suite)
			}
			if plan.suite == suiteLive {
				if ids[sc.ID] {
					t.Fatalf("duplicate id %s", sc.ID)
				}
				ids[sc.ID] = true
			}
		}
	}
}

func TestCorpusTasksReturnsScenariosForASuite(t *testing.T) {
	scenarios, err := corpusTasks(testCorpusRoot, map[string]bool{demoShop: true}, suiteLive)
	if err != nil {
		t.Fatal(err)
	}
	var kindsSeen []string
	for _, sc := range scenarios {
		kindsSeen = append(kindsSeen, sc.Kind)
		if sc.Seed != nil && sc.Seed.Corpus != demoShop {
			t.Fatalf("%s seeds %s, want only the sampled dashboard", sc.ID, sc.Seed.Corpus)
		}
	}
	slices.Sort(kindsSeen)
	if want := []string{kindCreate, kindFixUnverified, kindRevise, kindRevise, kindRevise, kindRevise, kindSizeGuard, kindSuggest}; !slices.Equal(kindsSeen, want) {
		t.Fatalf("kinds = %v, want %v", kindsSeen, want)
	}
	if _, err := corpusTasks(testCorpusRoot, map[string]bool{"nope/missing.json": true}, suiteLive); err == nil {
		t.Fatal("a sample entry outside the corpus was accepted")
	}
	if _, err := corpusTasks(testCorpusRoot, nil, "both"); err == nil {
		t.Fatal("an unknown suite was accepted")
	}
}

func TestFixTaskForTheDemoShop(t *testing.T) {
	replayTask := taskOf(t, planOf(t, suiteReplay), taskFix, demoShop)
	liveTask := taskOf(t, planOf(t, suiteLive), taskFix, demoShop)
	if liveTask.scenario.Prompt != `Fix the unverified panel "{panel_title}" ({panel_id}).` {
		t.Fatalf("live prompt = %q", liveTask.scenario.Prompt)
	}
	if want := liveTask.scenario.Prompt + " [corpus-fix-slashobserve-demo-shop]"; replayTask.scenario.Prompt != want {
		t.Fatalf("replay prompt = %q, want %q", replayTask.scenario.Prompt, want)
	}
	sc := replayTask.scenario
	if sc.Kind != kindFixUnverified || sc.Seed == nil || !sc.Seed.BreakPanel || sc.Seed.Corpus != demoShop {
		t.Fatalf("scenario = %+v", sc)
	}
	ex := sc.Expect
	if !ex.OnlyTargetChanged || !ex.TargetReturnsData || !ex.UnverifiedCleared || !ex.KeepsPanels || ex.FailureCode != "" || ex.MaxSaveOutputTokens != fixSaveTokenBudget {
		t.Fatalf("expect = %+v, want the four fix checks, the save token budget and no failure", ex)
	}
	if !slices.Equal(sc.Suites, []string{suiteReplay, suiteLive}) {
		t.Fatalf("suites = %v", sc.Suites)
	}
	if replayTask.fixture == nil || liveTask.fixture != nil {
		t.Fatal("only the replay plan carries fixtures")
	}
	if !strings.HasPrefix(replayTask.fixture.Prompt, `Fix the unverified panel "`) || !strings.HasSuffix(replayTask.fixture.Prompt, " [corpus-fix-slashobserve-demo-shop]") || strings.Contains(replayTask.fixture.Prompt, "{panel") {
		t.Fatalf("fixture prompt = %q", replayTask.fixture.Prompt)
	}
}

func TestAddTaskAndRebuildTaskForTheDemoShop(t *testing.T) {
	add := taskOf(t, planOf(t, suiteReplay), taskAdd, demoShop).scenario
	if add.Kind != kindRevise || !add.Expect.KeepsPanels || !strings.HasPrefix(add.Prompt, "Add a panel showing ") || !strings.HasSuffix(add.Prompt, " [corpus-add-slashobserve-demo-shop]") {
		t.Fatalf("add scenario = %+v", add)
	}
	rebuild := taskOf(t, planOf(t, suiteLive), taskRebuild, demoShop).scenario
	if rebuild.Kind != kindCreate || rebuild.Seed != nil || !slices.Equal(rebuild.Suites, []string{suiteLive}) {
		t.Fatalf("rebuild scenario = %+v", rebuild)
	}
	if rebuild.Category != categoryPrometheus && rebuild.Category != categoryMixed && rebuild.Category != categoryClickHouse {
		t.Fatalf("category = %q", rebuild.Category)
	}
	if !strings.Contains(rebuild.Prompt, `"Slash`) && !strings.HasPrefix(rebuild.Prompt, "Build a dashboard titled ") {
		t.Fatalf("rebuild prompt = %q", rebuild.Prompt)
	}
	if len(rebuild.Expect.Metrics) == 0 || rebuild.Expect.RebuildMetricShare != 0.25 || rebuild.Expect.RebuildMetricCap != 20 || rebuild.Expect.MinPanels < 1 || !slices.Contains(rebuild.Tags, taskRebuild) {
		t.Fatalf("rebuild expect = %+v", rebuild.Expect)
	}
}

func TestRebuildPromptStaysUnderThePromptCap(t *testing.T) {
	for _, tk := range planOf(t, suiteLive).tasks {
		if tk.scenario.Kind == kindCreate && utf8.RuneCountInString(tk.scenario.Prompt) > maxPromptRunes {
			t.Fatalf("%s prompt has %d characters", tk.scenario.ID, utf8.RuneCountInString(tk.scenario.Prompt))
		}
	}
}

func TestExpectedFailureCodes(t *testing.T) {
	replayPlan, livePlan := planOf(t, suiteReplay), planOf(t, suiteLive)
	tests := []struct {
		name         string
		plan         *planner
		taskName     string
		key          string
		wantFailure  string
		wantFixtures bool
	}{
		{"a fix on a dashboard over 128 KiB succeeds in replay", replayPlan, taskFix, nodeExporterFullOld, "", true},
		{"an add on a dashboard over 128 KiB succeeds in live", livePlan, taskAdd, nodeExporterFullOld, "", false},
		{"a fix on a large dashboard succeeds in replay", replayPlan, taskFix, nodeExporterFull, "", true},
		{"the same fix has no fixture live", livePlan, taskFix, nodeExporterFull, "", false},
		{"adding to a large dashboard succeeds in replay", replayPlan, taskAdd, nodeExporterFull, "", true},
		{"a small dashboard succeeds in replay", replayPlan, taskFix, demoShop, "", true},
		{"a paste over the prompt cap is rejected", replayPlan, taskPaste, nodeExporterFull, failurePromptTooLong, false},
		{"a paste that fits is imported", replayPlan, taskPaste, smallImport, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := taskOf(t, tt.plan, tt.taskName, tt.key)
			if tk.scenario.Expect.FailureCode != tt.wantFailure {
				t.Fatalf("failure_code = %q, want %q", tk.scenario.Expect.FailureCode, tt.wantFailure)
			}
			if (tk.fixture != nil) != tt.wantFixtures {
				t.Fatalf("fixture present = %t, want %t", tk.fixture != nil, tt.wantFixtures)
			}
			if tk.scenario.Expect.FailureCode != "" && (tk.scenario.Expect.OnlyTargetChanged || tk.scenario.Expect.KeepsPanels) {
				t.Fatalf("a task expected to fail must skip the other checks: %+v", tk.scenario.Expect)
			}
		})
	}
}

func TestPasteTasksOverTheGuardAreSizeGuardAndTheOthersAreImports(t *testing.T) {
	useCorpus(t)
	for _, plan := range []*planner{planOf(t, suiteReplay), planOf(t, suiteLive)} {
		for _, tk := range plan.tasks {
			sc := tk.scenario
			if !strings.HasPrefix(sc.ID, corpusIDPrefix+"-paste-") {
				continue
			}
			rejected := sc.Expect.FailureCode == failurePromptTooLong
			if rejected != (sc.Kind == kindSizeGuard) || (sc.Kind != kindSizeGuard && sc.Kind != kindChatImport) {
				t.Fatalf("%s is kind %s with failure code %q", sc.ID, sc.Kind, sc.Expect.FailureCode)
			}
			if err := validateScenario(sc); err != nil {
				t.Fatalf("%s: %v", sc.ID, err)
			}
		}
	}
	big := taskOf(t, planOf(t, suiteReplay), taskPaste, nodeExporterFull).scenario
	if big.Kind != kindSizeGuard || big.Expect.FailureCode != failurePromptTooLong {
		t.Fatalf("a paste over the cap = kind %s, failure code %q", big.Kind, big.Expect.FailureCode)
	}
	small := taskOf(t, planOf(t, suiteReplay), taskPaste, smallImport).scenario
	if small.Kind != kindChatImport || small.Expect.FailureCode != "" {
		t.Fatalf("a paste that fits = kind %s, failure code %q", small.Kind, small.Expect.FailureCode)
	}
}

func TestPasteTaskPromptMatchesTheRunnersPrompt(t *testing.T) {
	useCorpus(t)
	tk := taskOf(t, planOf(t, suiteReplay), taskPaste, smallImport)
	want, err := chatImportPrompt(tk.scenario)
	if err != nil {
		t.Fatal(err)
	}
	if tk.fixture.Prompt != want {
		t.Fatalf("fixture prompt differs from the prompt the runner sends")
	}
	if n := utf8.RuneCountInString(want); n > maxPromptRunes {
		t.Fatalf("prompt has %d characters", n)
	}
	call := tk.fixture.Turns[0].ToolCalls[0]
	var input importInput
	if err := json.Unmarshal(call.Input, &input); err != nil || call.Name != toolImportGrafana || !strings.Contains(want, input.JSON) {
		t.Fatalf("import call = %s %s, %v", call.Name, call.Input, err)
	}
}

func TestEstimatedTokensRoundUpAgainstTheCap(t *testing.T) {
	tests := []struct{ bytes, want int }{{0, 0}, {1, 1}, {3200, 1000}, {51200, 16000}, {51201, 16001}}
	for _, tt := range tests {
		if got := estimatedTokens(tt.bytes); got != tt.want {
			t.Errorf("estimatedTokens(%d) = %d, want %d", tt.bytes, got, tt.want)
		}
	}
}

func TestTaskIDsStayWithinTheIDPattern(t *testing.T) {
	p := &planner{}
	short := p.id(taskFix, analyzed{corpusDashboard: corpusDashboard{key: nodeExporterFull, source: "rfmoz", name: "node-exporter-full"}})
	if short != "corpus-fix-rfmoz-node-exporter-full" {
		t.Fatalf("short id = %q", short)
	}
	long := corpusDashboard{key: "clickhouse-datasource/otel-traces-explorer-with-a-very-long-name.json", source: "clickhouse-datasource", name: "otel-traces-explorer-with-a-very-long-name"}
	first, second := p.id(taskRebuild, analyzed{corpusDashboard: long}), p.id(taskRebuild, analyzed{corpusDashboard: long})
	if first != second || len(first) != maxIDLength || !idPattern.MatchString(first) {
		t.Fatalf("long id = %q (%d)", first, len(first))
	}
	other := long
	other.key = "clickhouse-datasource/otel-traces-explorer-with-a-very-long-name-2.json"
	if p.id(taskRebuild, analyzed{corpusDashboard: other}) == first {
		t.Fatal("two long names produced the same id")
	}
	if got := slug("Node_Exporter  Full.v2"); got != "node-exporter-full-v2" {
		t.Fatalf("slug = %q", got)
	}
}

func TestLoadSample(t *testing.T) {
	shipped, err := loadSample(filepath.Join("..", "corpus-sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(shipped) != 8 || !shipped[nodeExporterFull] || !shipped[demoShop] {
		t.Fatalf("shipped sample = %v", shipped)
	}
	fixed := map[string]bool{}
	for _, sc := range shippedSampleTasks(t, suiteLive) {
		if sc.Kind == kindFixUnverified {
			fixed[sc.Seed.Corpus] = true
		}
	}
	for key := range shipped {
		if !fixed[key] {
			t.Errorf("%s has no fix task in the live sample", key)
		}
	}

	bad := map[string]string{
		"unknown field": "dashboards: [a/b.json]\nextra: 1\n",
		"empty":         "dashboards: []\n",
		"duplicate":     "dashboards: [a/b.json, a/b.json]\n",
		"escapes":       "dashboards: [../a/b.json]\n",
		"no source":     "dashboards: [b.json]\n",
		"not json":      "dashboards: [a/b.yaml]\n",
	}
	for name, content := range bad {
		path := filepath.Join(t.TempDir(), "sample.yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSample(path); err == nil {
			t.Errorf("%s: loadSample accepted the file", name)
		}
	}
}

func TestPrepareWritesReplayFixturesAndSeriesThatBehaveAsExpected(t *testing.T) {
	useFixtureDirs(t)
	out := t.TempDir()
	var report bytes.Buffer
	previous := prepareOutput
	prepareOutput = &report
	t.Cleanup(func() { prepareOutput = previous })
	llmDir, synthDir := filepath.Join(out, "llm"), filepath.Join(out, "synth")
	if err := prepareReplay(llmDir, planOf(t, suiteReplay), corpusTokenCap); err != nil {
		t.Fatal(err)
	}
	if err := generateSynth(synthDir); err != nil {
		t.Fatal(err)
	}
	provider, err := replay.New(llmDir, corpusTokenCap, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, tk := range planOf(t, suiteReplay).tasks {
		if tk.fixture == nil {
			continue
		}
		checked++
		session, err := provider.Start(context.Background(), llm.Request{Prompt: tk.fixture.Prompt})
		if err != nil {
			t.Fatalf("%s: %v", tk.scenario.ID, err)
		}
		turn, err := session.Next(context.Background(), nil)
		if err != nil {
			t.Fatalf("%s: %v", tk.scenario.ID, err)
		}
		if turn.Stop != llm.StopToolUse || len(turn.ToolCalls) != 1 {
			t.Fatalf("%s: stop = %v with %d output tokens and %d tool calls", tk.scenario.ID, turn.Stop, turn.Usage.OutputTokens, len(turn.ToolCalls))
		}
	}
	if checked < 190 {
		t.Fatalf("checked %d fixtures", checked)
	}
	files, err := filepath.Glob(filepath.Join(llmDir, "*"+fixtureSuffix))
	if err != nil {
		t.Fatal(err)
	}
	handWritten, err := filepath.Glob(filepath.Join(e2eFixtureDir, "*"+fixtureSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != checked+1+len(handWritten) {
		t.Fatalf("%d fixture files, want %d generated, the renewal fixture and %d copied", len(files), checked, len(handWritten))
	}
	wantLine := fmt.Sprintf("replay fixtures: %d generated, %d copied", checked+1, len(handWritten))
	if !strings.Contains(report.String(), wantLine) || !strings.Contains(report.String(), "synthetic series: ") {
		t.Fatalf("report = %q, want it to contain %q", report.String(), wantLine)
	}
	if _, err := os.Stat(filepath.Join(synthDir, synthSeriesFile)); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareReplacesStaleFixtures(t *testing.T) {
	useFixtureDirs(t)
	out := t.TempDir()
	stale := filepath.Join(out, "llm", "stale.json")
	if err := writeShared(stale, []byte(`{"prompt":"stale","turns":[{"text":"x"}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := writeReplay(filepath.Join(out, "llm"), planOf(t, suiteReplay), corpusTokenCap); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("the stale fixture survived: %v", err)
	}
}

func TestReplayFixturesPatchOnlyWhatTheTaskAllows(t *testing.T) {
	useCorpus(t)
	checkedFix, checkedAdd := 0, 0
	pristines := map[string]*spec.Dashboard{}
	for _, tk := range planOf(t, suiteReplay).tasks {
		isFix, isAdd := strings.HasPrefix(tk.scenario.ID, "corpus-fix-"), strings.HasPrefix(tk.scenario.ID, "corpus-add-")
		if tk.fixture == nil || tk.fixture.Turns[0].ToolCalls[0].Name != toolUpdatePanels || (!isFix && !isAdd) {
			continue
		}
		var patch spec.PanelPatch
		if err := json.Unmarshal(tk.fixture.Turns[0].ToolCalls[0].Input, &patch); err != nil {
			t.Fatalf("%s: %v", tk.scenario.ID, err)
		}
		source, err := readCorpus(tk.scenario.Seed.Corpus)
		if err != nil {
			t.Fatal(err)
		}
		pristine, ok := pristines[tk.scenario.Seed.Corpus]
		if !ok {
			if pristine, err = convertSource(source, stackOptions()); err != nil {
				t.Fatal(err)
			}
			pristines[tk.scenario.Seed.Corpus] = pristine
		}
		if tk.scenario.Kind == kindFixUnverified {
			checkedFix++
			assertOnlyTargetChanged(t, tk, source, pristine, patch)
			continue
		}
		checkedAdd++
		saved, err := spec.ApplyPanelPatch(pristine, patch)
		if err != nil {
			t.Fatalf("%s: the patch does not apply: %v", tk.scenario.ID, err)
		}
		changes := spec.Diff(pristine, saved)
		if len(changes) == 0 {
			t.Fatalf("%s: the added panel is not in the saved spec", tk.scenario.ID)
		}
		for _, change := range changes {
			if !strings.HasPrefix(change.Path, "/panels/"+addedPanelID) {
				t.Fatalf("%s: change at %s is not the added panel", tk.scenario.ID, change.Path)
			}
		}
	}
	if checkedFix < 90 || checkedAdd < 90 {
		t.Fatalf("checked %d fix and %d add fixtures", checkedFix, checkedAdd)
	}
}

func assertOnlyTargetChanged(t *testing.T, tk task, source []byte, pristine *spec.Dashboard, patch spec.PanelPatch) {
	t.Helper()
	broken, err := breakDashboard(source, pristine)
	if err != nil {
		t.Fatalf("%s: %v", tk.scenario.ID, err)
	}
	saved, err := spec.ApplyPanelPatch(broken.stored, patch)
	if err != nil {
		t.Fatalf("%s: the patch does not apply: %v", tk.scenario.ID, err)
	}
	changes := spec.Diff(broken.stored, saved)
	if len(changes) == 0 {
		t.Fatalf("%s: the fixture changes nothing", tk.scenario.ID)
	}
	for _, change := range changes {
		if id, ok := panelOfPath(change.Path); !ok || id != broken.target {
			t.Fatalf("%s: change at %s is outside the target panel %s", tk.scenario.ID, change.Path, broken.target)
		}
	}
}

func TestGeneratedFixSavesStayUnderTheSaveTokenBudget(t *testing.T) {
	fixes := 0
	for _, tk := range planOf(t, suiteReplay).tasks {
		if tk.scenario.Kind != kindFixUnverified {
			continue
		}
		fixes++
		if tk.scenario.Expect.MaxSaveOutputTokens != fixSaveTokenBudget {
			t.Fatalf("%s: max_save_output_tokens = %d, want %d", tk.scenario.ID, tk.scenario.Expect.MaxSaveOutputTokens, fixSaveTokenBudget)
		}
		if tokens := estimatedTokens(tk.fixture.inputBytes()); tokens > fixSaveTokenBudget {
			t.Fatalf("%s: the fix save is %d bytes, %d estimated tokens, over %d", tk.scenario.ID, tk.fixture.inputBytes(), tokens, fixSaveTokenBudget)
		}
	}
	if fixes < 90 {
		t.Fatalf("checked %d fix fixtures", fixes)
	}
}

func TestHealTargetReplacesQueriesWithOnesAYoungStackServes(t *testing.T) {
	dash := &spec.Dashboard{Variables: []spec.Variable{{Name: "datasource", Type: spec.DatasourceVariable, DatasourceType: spec.Prometheus}}}
	panel := &spec.Panel{
		ID: "p1", Type: spec.Stat, Datasource: "${datasource}", Interval: "30m", MaxDataPoints: 100,
		Queries: []spec.Query{
			{Ref: "A", Query: "redis_uptime_in_seconds", Legend: "{{instance}}", Interval: "1800s", IntervalFactor: 2},
			{Ref: "B", Query: "SELECT 1", Datasource: "ch", Interval: "1h"},
			{Ref: "C", Query: " "},
		},
	}
	if err := healTarget(dash, panel); err != nil {
		t.Fatal(err)
	}
	want := []spec.Query{
		{Ref: "A", Query: healthyPromQuery, Instant: true},
		{Ref: "B", Query: healthyCHQuery, Datasource: "ch"},
		{Ref: "C", Query: " "},
	}
	if !reflect.DeepEqual(panel.Queries, want) || panel.Interval != "" {
		t.Fatalf("queries %+v, panel interval %q, want %+v", panel.Queries, panel.Interval, want)
	}

	added := addedPanel(dash, spec.Prometheus)
	if len(added.Queries) != 1 || !added.Queries[0].Instant || added.Queries[0].Query != healthyPromQuery {
		t.Fatalf("added panel queries %+v", added.Queries)
	}
}
