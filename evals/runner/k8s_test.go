package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/grafana"
)

var k8sConversions sync.Map

type k8sConversion struct {
	result *grafana.Result
	err    error
}

func convertK8s(source []byte, mapping map[string]string) (*grafana.Result, error) {
	key := string(source) + "\x00" + fmt.Sprint(mapping)
	if cached, ok := k8sConversions.Load(key); ok {
		c := cached.(k8sConversion)
		return c.result, c.err
	}
	result, err := grafana.Convert(source, importOptions(k8sDatasources, mapping))
	k8sConversions.Store(key, k8sConversion{result: result, err: err})
	return result, err
}

var notKubernetes = []string{
	"kube-prometheus/alertmanager-overview.json",
	"kube-prometheus/grafana-overview.json",
	"kube-prometheus/prometheus.json",
	"kube-prometheus/prometheus-remote-write.json",
}

func TestK8sDashboardListMatchesTheCorpus(t *testing.T) {
	if len(k8sDashboards) != 29 {
		t.Fatalf("%d Kubernetes dashboards, want 29", len(k8sDashboards))
	}
	for _, key := range k8sDashboards {
		if _, err := os.Stat(filepath.Join(testCorpusRoot, key)); err != nil {
			t.Errorf("%s: %v", key, err)
		}
	}
	for _, source := range []string{"dotdc", "kube-prometheus"} {
		files, err := filepath.Glob(filepath.Join(testCorpusRoot, source, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			key := source + "/" + filepath.Base(file)
			if !slices.Contains(k8sDashboards, key) && !slices.Contains(notKubernetes, key) {
				t.Errorf("%s is in the corpus but neither a Kubernetes dashboard nor a known exception", key)
			}
		}
	}
	if set := k8sDashboardSet(); len(set) != 29 {
		t.Errorf("the dashboard set has %d entries, want 29", len(set))
	}
}

func TestK8sMappingResolvesEveryKubernetesDashboardToTheClusterPrometheus(t *testing.T) {
	useCorpus(t)
	for _, key := range k8sDashboards {
		source, err := readCorpus(key)
		if err != nil {
			t.Fatal(err)
		}
		bare, err := convertK8s(source, nil)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if bare.Ready() {
			t.Errorf("%s resolves without a mapping, so the mapping would prove nothing", key)
		}
		mapping, err := k8sMapping(source)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if len(mapping) == 0 {
			t.Errorf("%s: no Prometheus source to map", key)
		}
		for from, to := range mapping {
			if to != k8sDatasourceUID {
				t.Errorf("%s maps %s to %s, want %s", key, from, to, k8sDatasourceUID)
			}
		}
		mapped, err := convertK8s(source, mapping)
		if err != nil || !mapped.Ready() {
			t.Errorf("%s does not resolve with its mapping (err %v)", key, err)
		}
	}
}

func TestK8sTasksSeedWithTheClusterMappingAndTheK8sTag(t *testing.T) {
	sample := map[string]bool{"kube-prometheus/nodes.json": true, "dotdc/k8s-views-pods.json": true}
	tasks, err := k8sCorpusTasks(testCorpusRoot, sample, suiteLive)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("no tasks")
	}
	for _, sc := range tasks {
		if !slices.Contains(sc.Tags, tagK8s) || !slices.Contains(sc.Tags, tagCorpus) {
			t.Errorf("%s tags = %v, want corpus and k8s", sc.ID, sc.Tags)
		}
		if sc.Seed == nil {
			if sc.Kind != kindCreate {
				t.Errorf("%s has kind %s and no seed", sc.ID, sc.Kind)
			}
			continue
		}
		if !sample[sc.Seed.Corpus] {
			t.Errorf("%s seeds %s, outside the sample", sc.ID, sc.Seed.Corpus)
		}
		if len(sc.Seed.Mapping) == 0 {
			t.Errorf("%s has no mapping", sc.ID)
		}
		for _, to := range sc.Seed.Mapping {
			if to != k8sDatasourceUID {
				t.Errorf("%s maps to %s", sc.ID, to)
			}
		}
	}
}

func TestK8sTasksRefuseWhatTheProfileCannotRun(t *testing.T) {
	cases := map[string]struct {
		sample map[string]bool
		suite  string
		want   string
	}{
		"replay suite":            {suite: suiteReplay, want: "live suite only"},
		"dashboard not for k8s":   {sample: map[string]bool{demoShop: true}, suite: suiteLive, want: "not one of the Kubernetes dashboards"},
		"no sample means all 29":  {suite: suiteLive},
		"one kubernetes selected": {sample: map[string]bool{"kube-prometheus/proxy.json": true}, suite: suiteLive},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tasks, err := k8sCorpusTasks(testCorpusRoot, tc.sample, tc.suite)
			switch {
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("err = %v, want %q", err, tc.want)
			case tc.want == "" && (err != nil || len(tasks) == 0):
				t.Fatalf("tasks = %d, err = %v", len(tasks), err)
			}
		})
	}
}

func TestDefaultProfileTasksStayFreeOfTheClusterMapping(t *testing.T) {
	for _, sc := range shippedSampleTasks(t, suiteLive) {
		if slices.Contains(sc.Tags, tagK8s) || (sc.Seed != nil && len(sc.Seed.Mapping) > 0) {
			t.Errorf("%s carries the k8s profile: tags %v seed %+v", sc.ID, sc.Tags, sc.Seed)
		}
	}
}

func TestShippedK8sSample(t *testing.T) {
	path := filepath.Join("..", "corpus-sample.yaml")
	sample, err := loadK8sSample(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sample) == 0 {
		t.Fatal("the k8s sample is empty")
	}
	if _, err := k8sCorpusTasks(testCorpusRoot, sample, suiteLive); err != nil {
		t.Fatalf("the shipped k8s sample does not match the corpus: %v", err)
	}
	dflt, err := loadSample(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dflt) != 8 {
		t.Errorf("the default sample has %d entries, want 8", len(dflt))
	}

	bad := map[string]string{
		"missing":          "dashboards: [a/b.json]\n",
		"empty":            "dashboards: [a/b.json]\nk8s: []\n",
		"not kubernetes":   "dashboards: [a/b.json]\nk8s: [slashobserve/demo-shop.json]\n",
		"duplicate":        "dashboards: [a/b.json]\nk8s: [dotdc/k8s-views-pods.json, dotdc/k8s-views-pods.json]\n",
		"escapes":          "dashboards: [a/b.json]\nk8s: [../dotdc/k8s-views-pods.json]\n",
		"unknown property": "dashboards: [a/b.json]\nk8s: [dotdc/k8s-views-pods.json]\nextra: 1\n",
	}
	for name, content := range bad {
		file := filepath.Join(t.TempDir(), "sample.yaml")
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadK8sSample(file); err == nil {
			t.Errorf("%s: loadK8sSample accepted the file", name)
		}
	}
}

func TestLoadK8sGaps(t *testing.T) {
	shipped, err := loadK8sGaps(filepath.Join("..", "k8s-gaps.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(shipped) == 0 {
		t.Error("the shipped gaps list is empty")
	}

	good := "gaps:\n  - dashboard: kube-prometheus/k8s-resources-multicluster.json\n    panel_title: CPU Utilisation\n    reason: one cluster only\n"
	cases := map[string]struct {
		content string
		want    int
		wantErr bool
	}{
		"one gap":         {content: good, want: 1},
		"unknown field":   {content: good + "    extra: 1\n", wantErr: true},
		"other dashboard": {content: strings.Replace(good, "kube-prometheus/k8s-resources-multicluster.json", demoShop, 1), wantErr: true},
		"no reason":       {content: strings.Replace(good, "one cluster only", "", 1), wantErr: true},
		"no title":        {content: strings.Replace(good, "CPU Utilisation", "", 1), wantErr: true},
		"listed twice":    {content: good + strings.TrimPrefix(good, "gaps:\n"), wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "gaps.yaml")
			if err := os.WriteFile(file, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			gaps, err := loadK8sGaps(file)
			if (err != nil) != tc.wantErr || len(gaps) != tc.want {
				t.Fatalf("gaps = %d, err = %v", len(gaps), err)
			}
		})
	}
}

func TestCompareK8sSeparatesFailuresFromWarnings(t *testing.T) {
	gap := k8sGap{Dashboard: "dotdc/k8s-views-pods.json", PanelTitle: "Volumes", Reason: "no persistent volumes on kind"}
	results := func(unverified ...string) []k8sResult {
		return []k8sResult{
			{dashboard: "dotdc/k8s-views-pods.json", panels: 100, unverified: unverified},
			{dashboard: "kube-prometheus/proxy.json", panels: 100},
		}
	}
	cases := map[string]struct {
		results     []k8sResult
		gaps        []k8sGap
		wantProblem string
		wantWarning string
	}{
		"everything verifies":      {results: results()},
		"listed gap is unverified": {results: results("Volumes"), gaps: []k8sGap{gap}},
		"unlisted failure":         {results: results("Volumes"), wantProblem: `panel "Volumes" is unverified and not listed`},
		"stale entry":              {results: results(), gaps: []k8sGap{gap}, wantWarning: `panel "Volumes" is listed as a gap but verifies`},
		"entry for another title":  {results: results("Memory"), gaps: []k8sGap{gap}, wantProblem: `panel "Memory" is unverified and not listed`, wantWarning: `panel "Volumes" is listed as a gap but verifies`},
		"entry for another dashboard": {
			results:     []k8sResult{{dashboard: "kube-prometheus/proxy.json", panels: 100}},
			gaps:        []k8sGap{gap},
			wantWarning: `panel "Volumes" is listed as a gap but verifies`,
		},
		"over the 15% cap": {
			results:     []k8sResult{{dashboard: gap.Dashboard, panels: 6, unverified: []string{"Volumes"}}},
			gaps:        []k8sGap{gap},
			wantProblem: "more than 15%",
		},
		"within the 15% cap": {
			results: []k8sResult{{dashboard: gap.Dashboard, panels: 7, unverified: []string{"Volumes"}}},
			gaps:    []k8sGap{gap},
		},
		"a gap that verifies still counts against the cap": {
			results:     []k8sResult{{dashboard: gap.Dashboard, panels: 6}},
			gaps:        []k8sGap{gap},
			wantProblem: "more than 15%",
			wantWarning: "listed as a gap but verifies",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			problems, warnings := compareK8s(tc.results, tc.gaps)
			for kind, got := range map[string]struct {
				found []string
				want  string
			}{"problems": {problems, tc.wantProblem}, "warnings": {warnings, tc.wantWarning}} {
				joined := strings.Join(got.found, "\n")
				switch {
				case got.want == "" && joined != "":
					t.Errorf("unexpected %s: %s", kind, joined)
				case got.want != "" && !strings.Contains(joined, got.want):
					t.Errorf("%s = %q, want %q", kind, joined, got.want)
				}
			}
		})
	}
}

type clusterImports struct {
	mu       sync.Mutex
	titles   map[string][]string
	imported []string
}

func (f *clusterImports) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/imports", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			JSON    string            `json:"json"`
			Mapping map[string]string `json:"mapping"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		result, err := convertK8s([]byte(body.JSON), body.Mapping)
		if err != nil {
			t.Error(err)
			return
		}
		if !result.Ready() {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"unresolved datasources","code":"unresolved_datasources"}`))
			return
		}
		raw, err := json.Marshal(result.Spec)
		if err != nil {
			t.Error(err)
			return
		}
		f.mu.Lock()
		f.imported = append(f.imported, result.Spec.Title)
		wanted := f.titles[result.Spec.Title]
		f.mu.Unlock()
		var panels []map[string]string
		for _, entry := range result.Spec.AllPanels() {
			if slices.Contains(wanted, entry.Panel.Title) {
				panels = append(panels, map[string]string{"panel_id": entry.Panel.ID})
			}
		}
		payload, err := json.Marshal(map[string]any{
			"dashboard": map[string]any{"id": "dash-1"},
			"version":   map[string]any{"version": 1, "spec": json.RawMessage(raw), "unverified": map[string]any{"panels": panels, "variables": []any{}}},
			"import":    map[string]any{"id": "imp-1"},
		})
		if err != nil {
			t.Error(err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(payload)
	})
	return mux
}

func TestVerifyK8sImportsEveryDashboardThroughTheMapping(t *testing.T) {
	useCorpus(t)
	f := &clusterImports{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, secretToken)
	if err != nil {
		t.Fatal(err)
	}
	c.backoff = time.Millisecond

	problems, warnings, err := verifyK8s(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 || len(warnings) != 0 {
		t.Fatalf("findings on a server that verifies everything: problems %v, warnings %v", problems, warnings)
	}
	if len(f.imported) != 29 {
		t.Fatalf("%d imports, want 29", len(f.imported))
	}
}

func TestVerifyK8sReportsAnUnlistedUnverifiedPanel(t *testing.T) {
	useCorpus(t)
	source, err := readCorpus("kube-prometheus/proxy.json")
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := k8sMapping(source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := convertK8s(source, mapping)
	if err != nil {
		t.Fatal(err)
	}
	var title string
	for _, entry := range result.Spec.DataPanels() {
		title = entry.Panel.Title
		break
	}
	f := &clusterImports{titles: map[string][]string{result.Spec.Title: {title}}}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, secretToken)
	if err != nil {
		t.Fatal(err)
	}
	c.backoff = time.Millisecond

	problems, warnings, err := verifyK8s(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) == 0 || !strings.Contains(problems[0], "kube-prometheus/proxy.json") || len(warnings) != 0 {
		t.Fatalf("problems = %v, warnings = %v, want the proxy dashboard's unverified panel only", problems, warnings)
	}
}

func TestShopURLsFollowTheProfile(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	cases := map[string]struct {
		profile string
		flagged shopURLs
		env     map[string]string
		want    map[string]string
	}{
		"default profile keeps the evals stack ports": {
			want: map[string]string{"checkout-0": "http://127.0.0.1:28081", "checkout-1": "http://127.0.0.1:28082"},
		},
		"k8s profile points at the cluster ports": {
			profile: profileK8s,
			want:    map[string]string{"checkout-0": "http://127.0.0.1:38081", "checkout-1": "http://127.0.0.1:38082"},
		},
		"k8s ports follow the demo variables": {
			profile: profileK8s,
			env:     map[string]string{"SLASHOBSERVE_DEMO_K8S_SHOP_PORT": "1111", "SLASHOBSERVE_DEMO_K8S_SHOP1_PORT": "2222", "SLASHOBSERVE_EVALS_SHOP_PORT": "9"},
			want:    map[string]string{"checkout-0": "http://127.0.0.1:1111", "checkout-1": "http://127.0.0.1:2222"},
		},
		"a flag wins under k8s": {
			profile: profileK8s,
			flagged: shopURLs{"checkout-1": "http://example.test:5"},
			want:    map[string]string{"checkout-0": "http://127.0.0.1:38081", "checkout-1": "http://example.test:5"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := shopURLsFor(tc.profile, tc.flagged, env(tc.env))
			if len(got) != len(tc.want) {
				t.Fatalf("urls = %v, want %v", got, tc.want)
			}
			for pod, url := range tc.want {
				if got[pod] != url {
					t.Errorf("%s = %s, want %s", pod, got[pod], url)
				}
			}
		})
	}
}

func TestParseFlagsProfile(t *testing.T) {
	cases := map[string]struct {
		args    []string
		want    string
		wantErr string
	}{
		"default":                {nil, profileDefault, ""},
		"k8s with live":          {[]string{"--profile", "k8s", "--suite", "live"}, profileK8s, ""},
		"k8s without a suite":    {[]string{"--profile", "k8s"}, profileK8s, ""},
		"unknown profile":        {[]string{"--profile", "gke"}, "", "--profile must be default or k8s"},
		"k8s with replay":        {[]string{"--profile", "k8s", "--suite", "replay"}, "", "--profile k8s runs the live suite only"},
		"default with replay":    {[]string{"--suite", "replay"}, profileDefault, ""},
		"verify without a suite": {[]string{"--verify-k8s"}, profileDefault, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			o, err := parseFlags(tc.args, io.Discard)
			switch {
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error %v, want %q", err, tc.wantErr)
			case tc.wantErr == "" && (err != nil || o.profile != tc.want):
				t.Fatalf("profile %q, error %v, want %q", o.profile, err, tc.want)
			}
		})
	}
}

func TestCorpusTasksFollowTheProfile(t *testing.T) {
	useCorpus(t)
	calls := 0
	useCorpusTools(t, corpusToolkit{
		tasks:  func(string, map[string]bool, string) ([]Scenario, error) { calls++; return nil, nil },
		sample: func(string) (map[string]bool, error) { return nil, nil },
	})

	sampled, err := withCorpusTasks(nil, options{suite: suiteLive, corpus: corpusSample, profile: profileK8s, sampleFile: "../corpus-sample.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	all, err := withCorpusTasks(nil, options{suite: suiteLive, corpus: corpusAll, profile: profileK8s})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("the k8s profile used the default tools %d times", calls)
	}
	seeds := func(tasks []Scenario) int {
		set := map[string]bool{}
		for _, sc := range tasks {
			if !slices.Contains(sc.Tags, tagK8s) {
				t.Errorf("%s is not tagged k8s", sc.ID)
			}
			if sc.Seed != nil {
				set[sc.Seed.Corpus] = true
			}
		}
		return len(set)
	}
	sample, err := loadK8sSample("../corpus-sample.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := seeds(sampled); got == 0 || got > len(sample) {
		t.Errorf("the sample seeds %d dashboards, listed %d", got, len(sample))
	}
	if got := seeds(all); got != len(k8sDashboards) {
		t.Errorf("corpus all seeds %d dashboards, want %d", got, len(k8sDashboards))
	}

	if _, err := withCorpusTasks(nil, options{suite: suiteLive, corpus: corpusAll}); err != nil || calls != 1 {
		t.Errorf("the default profile must go through the corpus tools: calls %d, err %v", calls, err)
	}
}

func writeGaps(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gaps.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyK8sCommandNeedsNoScenariosOrProvider(t *testing.T) {
	useCorpus(t)
	f := &clusterImports{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	args := []string{"--verify-k8s", "--server", srv.URL, "--token", secretToken, "--scenarios", filepath.Join(t.TempDir(), "missing"), "--pricing", "missing.yaml"}

	var stdout, stderr strings.Builder
	o, err := parseFlags(args, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	o.k8sGapsFile = writeGaps(t, "gaps: []\n")
	if code := executeVerifyK8s(o, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if len(f.imported) != len(k8sDashboards) {
		t.Fatalf("%d imports, want %d", len(f.imported), len(k8sDashboards))
	}
}

func TestVerifyK8sCommandExitCodes(t *testing.T) {
	useCorpus(t)
	source, err := readCorpus("kube-prometheus/proxy.json")
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := k8sMapping(source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := convertK8s(source, mapping)
	if err != nil {
		t.Fatal(err)
	}
	title := result.Spec.DataPanels()[0].Panel.Title
	f := &clusterImports{titles: map[string][]string{result.Spec.Title: {title}}}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)

	listed := fmt.Sprintf("gaps:\n  - dashboard: kube-prometheus/proxy.json\n    panel_title: %q\n    reason: not scraped\n", title)
	staleToo := listed + "  - dashboard: dotdc/k8s-views-pods.json\n    panel_title: Volumes\n    reason: no volumes on kind\n"
	cases := map[string]struct {
		server string
		gaps   string
		want   int
		stdout string
	}{
		"an unlisted unverified panel exits 1":         {srv.URL, "gaps: []\n", 1, "kube-prometheus/proxy.json"},
		"a listed gap that verifies warns and exits 0": {srv.URL, staleToo, 0, `warning: dotdc/k8s-views-pods.json: panel "Volumes" is listed as a gap but verifies`},
		"a malformed gaps file exits 2":                {srv.URL, "gaps: [", 2, ""},
		"an unreachable server exits 2":                {"http://127.0.0.1:1", "gaps: []\n", 2, ""},
		"a missing server flag exits 2":                {"", "gaps: []\n", 2, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			o := options{server: tc.server, token: secretToken, k8sGapsFile: writeGaps(t, tc.gaps)}
			if code := executeVerifyK8s(o, &stdout, &stderr); code != tc.want {
				t.Fatalf("exit %d, want %d (stdout %q, stderr %q)", code, tc.want, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.stdout) {
				t.Errorf("stdout %q does not contain %q", stdout.String(), tc.stdout)
			}
			if strings.Contains(stderr.String(), secretToken) {
				t.Error("the token leaked to stderr")
			}
		})
	}
}
