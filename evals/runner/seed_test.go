package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const demoShop = "slashobserve/demo-shop.json"

func useCorpus(t *testing.T) {
	t.Helper()
	previous := corpusDir
	corpusDir = filepath.Join("..", "..", "pkg", "grafana", "testdata", "corpus")
	t.Cleanup(func() { corpusDir = previous })
}

func useBreaker(t *testing.T, breaker func([]byte) ([]byte, error)) {
	t.Helper()
	previous := panelBreaker
	panelBreaker = breaker
	t.Cleanup(func() { panelBreaker = previous })
}

func renameCPUMetric(source []byte) ([]byte, error) {
	return bytes.Replace(source, []byte("container_cpu_usage_seconds_total"), []byte("container_cpu_usage_seconds_total_broken_by_eval"), 1), nil
}

type fakeImports struct {
	mu         sync.Mutex
	busyFirst  int
	unverified []string
	variables  []string
	posted     [][]byte
	mappings   []map[string]string
	active     int
	maxActive  int
}

func (f *fakeImports) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/datasources", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"datasources":[{"uid":"prom","type":"prometheus"},{"uid":"ch","type":"clickhouse"}]}`)
	})
	mux.HandleFunc("POST /api/v1/imports", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.active++
		f.maxActive = max(f.maxActive, f.active)
		busy := f.busyFirst > 0
		if busy {
			f.busyFirst--
		}
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.active--
			f.mu.Unlock()
		}()
		time.Sleep(5 * time.Millisecond)
		if busy {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"too many imports in flight","code":"too_many_imports"}`)
			return
		}
		var body struct {
			JSON    string            `json:"json"`
			Mapping map[string]string `json:"mapping"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		f.mu.Lock()
		f.posted = append(f.posted, []byte(body.JSON))
		f.mappings = append(f.mappings, body.Mapping)
		unverified, variables := f.unverified, f.variables
		f.mu.Unlock()
		result, err := grafana.Convert([]byte(body.JSON), grafana.Options{Datasources: []grafana.Datasource{{UID: "prom", Type: "prometheus"}, {UID: "ch", Type: "clickhouse"}}, Mapping: body.Mapping})
		if err != nil {
			t.Error(err)
			return
		}
		raw, err := json.Marshal(result.Spec)
		if err != nil {
			t.Error(err)
			return
		}
		panels := make([]map[string]string, len(unverified))
		for i, id := range unverified {
			panels[i] = map[string]string{"panel_id": id}
		}
		names := make([]map[string]string, len(variables))
		for i, name := range variables {
			names[i] = map[string]string{"name": name}
		}
		payload, err := json.Marshal(map[string]any{
			"dashboard": map[string]any{"id": "dash-1"},
			"version":   map[string]any{"version": 1, "spec": json.RawMessage(raw), "unverified": map[string]any{"panels": panels, "variables": names}},
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

func startImports(t *testing.T, f *fakeImports) *client {
	t.Helper()
	useCorpus(t)
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, secretToken)
	if err != nil {
		t.Fatal(err)
	}
	c.backoff = time.Millisecond
	return c
}

func TestSeedBreaksOnePanelAndNamesItTheTarget(t *testing.T) {
	cases := map[string]struct {
		unverified   []string
		wantVerified bool
	}{
		"server could not verify the broken panel": {unverified: []string{"p3"}},
		"server verified it":                       {wantVerified: true},
		"another panel is unverified":              {unverified: []string{"p4"}, wantVerified: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeImports{unverified: tc.unverified}
			c := startImports(t, f)
			useBreaker(t, renameCPUMetric)
			got, err := seedScenario(context.Background(), c, Scenario{Seed: &Seed{Corpus: demoShop, BreakPanel: true}})
			if err != nil {
				t.Fatal(err)
			}
			want := seeded{dashboardID: "dash-1", version: 1, targetID: "p3", targetTitle: "CPU busy", targetVerified: tc.wantVerified}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("seeded %+v, want %+v", got, want)
			}
			if len(f.posted) != 1 || !bytes.Contains(f.posted[0], []byte("_broken_by_eval")) {
				t.Fatalf("the broken copy was not the one imported (%d imports)", len(f.posted))
			}
		})
	}
}

func TestSeedWithoutABreakImportsThePristineSourceAndTargetsTheFirstDataPanel(t *testing.T) {
	f := &fakeImports{}
	c := startImports(t, f)
	useBreaker(t, func([]byte) ([]byte, error) { return nil, errors.New("must not break") })
	mapping := map[string]string{"prometheus": "prom"}
	got, err := seedScenario(context.Background(), c, Scenario{Seed: &Seed{Corpus: demoShop, Mapping: mapping}})
	if err != nil {
		t.Fatal(err)
	}
	if got.targetID != "p2" || got.targetTitle != "Error ratio" || !got.targetVerified {
		t.Fatalf("seeded %+v, want the first data panel, verified", got)
	}
	source, err := os.ReadFile(filepath.Join(corpusDir, demoShop))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.posted[0], source) || f.mappings[0]["prometheus"] != "prom" {
		t.Fatal("the pristine source and the scenario's mapping must reach the import")
	}
}

func TestSeedFailures(t *testing.T) {
	cases := map[string]struct {
		seed    *Seed
		breaker func([]byte) ([]byte, error)
		want    string
	}{
		"no seed":      {seed: nil, want: "no seed"},
		"missing file": {seed: &Seed{Corpus: "nobody/none.json"}, want: "no such file"},
		"escaping path": {
			seed: &Seed{Corpus: "../corpus/slashobserve/demo-shop.json"},
			want: "must stay inside the corpus",
		},
		"breaker fails": {
			seed:    &Seed{Corpus: demoShop, BreakPanel: true},
			breaker: func([]byte) ([]byte, error) { return nil, errors.New("no query to break") },
			want:    "no query to break",
		},
		"break changes nothing": {
			seed:    &Seed{Corpus: demoShop, BreakPanel: true},
			breaker: func(source []byte) ([]byte, error) { return source, nil },
			want:    "changed nothing",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := startImports(t, &fakeImports{})
			if tc.breaker != nil {
				useBreaker(t, tc.breaker)
			}
			_, err := seedScenario(context.Background(), c, Scenario{Seed: tc.seed})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSeedRetriesWhileTheServerHasTooManyImports(t *testing.T) {
	f := &fakeImports{busyFirst: 2}
	c := startImports(t, f)
	got, err := seedScenario(context.Background(), c, Scenario{Seed: &Seed{Corpus: demoShop}})
	if err != nil {
		t.Fatal(err)
	}
	if got.dashboardID != "dash-1" || f.busyFirst != 0 || len(f.posted) != 1 {
		t.Fatalf("seeded %+v after %d busy answers left and %d imports", got, f.busyFirst, len(f.posted))
	}
}

func TestSeedGivesUpWhenTheContextEnds(t *testing.T) {
	f := &fakeImports{busyFirst: 1 << 20}
	c := startImports(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	_, err := seedScenario(ctx, c, Scenario{Seed: &Seed{Corpus: demoShop}})
	if !errors.Is(err, context.DeadlineExceeded) && (err == nil || !strings.Contains(err.Error(), "too_many_imports")) {
		t.Fatalf("error %v, want the deadline or the last too_many_imports answer", err)
	}
}

func TestImportsAreSerialisedPerClient(t *testing.T) {
	f := &fakeImports{}
	c := startImports(t, f)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := seedScenario(context.Background(), c, Scenario{Seed: &Seed{Corpus: demoShop}}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if f.maxActive != 1 || len(f.posted) != 4 {
		t.Fatalf("%d imports in flight at once over %d imports, want 1 at a time", f.maxActive, len(f.posted))
	}
}

func TestChangedPanelResolvesRowNestedAndTopLevelPaths(t *testing.T) {
	cases := map[string]struct {
		paths   []string
		want    string
		wantErr string
	}{
		"panel in a row":        {paths: []string{"/panels/1/panels/3/queries/A/expr"}, want: "3"},
		"top-level panel":       {paths: []string{"/panels/7/queries/A/expr"}, want: "7"},
		"added panel":           {paths: []string{"/panels/9"}, want: "9"},
		"several fields":        {paths: []string{"/panels/1/panels/3/queries/A/expr", "/panels/1/panels/3/title"}, want: "3"},
		"escaped id":            {paths: []string{"/panels/a~1b/title"}, want: "a/b"},
		"two panels":            {paths: []string{"/panels/1/panels/3/title", "/panels/1/panels/4/title"}, wantErr: "want one"},
		"outside the panels":    {paths: []string{"/variables/service/query"}, wantErr: "not inside one panel"},
		"dashboard level":       {paths: []string{"/title"}, wantErr: "not inside one panel"},
		"no changes":            {wantErr: "changed nothing"},
		"panels list reordered": {paths: []string{"/panels"}, wantErr: "not inside one panel"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			changes := make([]spec.Change, len(tc.paths))
			for i, p := range tc.paths {
				changes[i] = spec.Change{Op: spec.OpReplace, Path: p}
			}
			got, err := changedPanel(changes)
			switch {
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error %v, want %q", err, tc.wantErr)
			case tc.wantErr == "" && (err != nil || got != tc.want):
				t.Fatalf("got %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestSpecDiffPathsOfARealBreakPointAtTheNestedPanel(t *testing.T) {
	useCorpus(t)
	source, err := readCorpus(demoShop)
	if err != nil {
		t.Fatal(err)
	}
	broken, err := renameCPUMetric(source)
	if err != nil {
		t.Fatal(err)
	}
	opts := importOptions(map[string]string{"prom": "prometheus", "ch": "clickhouse"}, nil)
	before, err := convertSource(source, opts)
	if err != nil {
		t.Fatal(err)
	}
	after, err := convertSource(broken, opts)
	if err != nil {
		t.Fatal(err)
	}
	changes := spec.Diff(before, after)
	if len(changes) != 1 || !strings.HasPrefix(changes[0].Path, "/panels/p1/panels/p3/queries/") {
		t.Fatalf("changes %+v, want one change under /panels/p1/panels/p3/queries/", changes)
	}
}

func TestFillPrompt(t *testing.T) {
	useCorpus(t)
	target := seeded{targetID: "p3", targetTitle: "CPU busy"}
	long := strings.Repeat("x", maxPromptRunes)
	cases := map[string]struct {
		sc      Scenario
		want    string
		wantErr string
	}{
		"placeholders": {
			sc:   Scenario{Kind: kindFixUnverified, Prompt: `Fix the unverified panel "{panel_title}" ({panel_id}).`},
			want: `Fix the unverified panel "CPU busy" (p3).`,
		},
		"no placeholders": {sc: Scenario{Kind: kindRevise, Prompt: "Add a panel."}, want: "Add a panel."},
		"over the cap once filled": {
			sc:      Scenario{Kind: kindSuggest, Prompt: long[:maxPromptRunes-3] + "{panel_title}"},
			wantErr: "after filling",
		},
		"chat import has no cap": {
			sc:   Scenario{Kind: kindChatImport, Prompt: "Import this Grafana dashboard:"},
			want: "Import this Grafana dashboard:",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := fillPrompt(tc.sc, target)
			switch {
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error %v, want %q", err, tc.wantErr)
			case tc.wantErr == "" && (err != nil || got != tc.want):
				t.Fatalf("got %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestChatImportPromptCarriesTheMinifiedSource(t *testing.T) {
	useCorpus(t)
	sc := Scenario{Kind: kindChatImport, Prompt: "Import this Grafana dashboard:", Seed: &Seed{Corpus: demoShop}}
	got, err := fillPrompt(sc, seeded{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := readCorpus(demoShop)
	if err != nil {
		t.Fatal(err)
	}
	var minified bytes.Buffer
	if err := json.Compact(&minified, source); err != nil {
		t.Fatal(err)
	}
	if got != "Import this Grafana dashboard: "+minified.String() || strings.ContainsAny(got, "\n\t") {
		t.Fatalf("prompt of %d bytes is not the prefix and the minified JSON", len(got))
	}
}

func writeScenarios(t *testing.T, body string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadScenarios(dir)
	return dir, err
}

func TestScenarioKindsDefaultsAndValidation(t *testing.T) {
	const seed = "seed:\n  corpus: slashobserve/demo-shop.json\n"
	const live, create = "suites: [live]\n", "kind: create\nsuites: [live]\n"
	cases := map[string]struct {
		body    string
		wantErr string
	}{
		"revise needs no datasources": {
			body: "id: revise-demo\nprompt: Add a panel.\nkind: revise\n" + live + seed + "expect:\n  keeps_panels: true\n",
		},
		"suggest with placeholders": {
			body: "id: suggest-demo\nprompt: 'Improve {panel_title} ({panel_id}).'\nkind: suggest\nsuites: [live, replay]\n" + seed,
		},
		"fix with a fault and values": {
			body: "id: fix-demo\nprompt: Fix it.\nkind: fix_unverified\n" + live + seed + "  break_panel: true\nfault: {pod: checkout-1, type: errors, pct: 30, soak_seconds: 120}\nexpect:\n  only_target_changed: true\n  failure_code: call_timeout\n  max_save_output_tokens: 2000\n  panel_values:\n    - {query_contains: http_requests_total, series: {pod: checkout-1}, min: 0.2, max: 0.4}\n",
		},
		"chat import may exceed the cap": {
			body: "id: chat-demo\nprompt: " + strings.Repeat("x", maxPromptRunes+1) + "\nkind: chat_import\n" + live,
		},
		"create still caps the prompt": {
			body:    "id: create-demo\nprompt: " + strings.Repeat("x", maxPromptRunes+1) + "\n" + create + "category: prometheus\nexpect:\n  datasources: [prometheus]\n",
			wantErr: "longer than 4000",
		},
		"create still needs datasources": {
			body:    "id: create-demo\nprompt: Show it.\n" + create + "category: prometheus\n",
			wantErr: "expect.datasources",
		},
		"create still needs a category": {
			body:    "id: create-demo\nprompt: Show it.\n" + create,
			wantErr: "category",
		},
		"unknown kind": {
			body:    "id: odd-demo\nprompt: Do it.\nkind: delete\n" + live,
			wantErr: `kind "delete"`,
		},
		"revise without a seed": {
			body:    "id: revise-demo\nprompt: Add a panel.\nkind: revise\n" + live,
			wantErr: "seed is required for kind revise",
		},
		"target expectation without a seed": {
			body:    "id: chat-demo\nprompt: Import.\nkind: chat_import\n" + live + "expect:\n  keeps_panels: true\n",
			wantErr: "seed is required",
		},
		"escaping seed path": {
			body:    "id: revise-demo\nprompt: Add.\nkind: revise\n" + live + "seed:\n  corpus: ../x.json\n",
			wantErr: "seed.corpus",
		},
		"missing kind": {
			body:    "id: create-demo\nprompt: Show.\ncategory: prometheus\n" + live + "expect:\n  datasources: [prometheus]\n",
			wantErr: "kind is required",
		},
		"missing suites": {
			body:    "id: create-demo\nprompt: Show.\ncategory: prometheus\nkind: create\nexpect:\n  datasources: [prometheus]\n",
			wantErr: "suites is required",
		},
		"any declared suite name": {
			body: "id: create-demo\nprompt: Show.\ncategory: prometheus\nkind: create\nsuites: [live, nightly-2]\nexpect:\n  datasources: [prometheus]\n",
		},
		"malformed suite": {
			body:    "id: create-demo\nprompt: Show.\ncategory: prometheus\nkind: create\nsuites: [Nightly]\nexpect:\n  datasources: [prometheus]\n",
			wantErr: `suites entry "Nightly" must match`,
		},
		"duplicate suite": {
			body:    "id: create-demo\nprompt: Show.\ncategory: prometheus\nkind: create\nsuites: [live, live]\nexpect:\n  datasources: [prometheus]\n",
			wantErr: "suites contain",
		},
		"fault percentage out of range": {
			body:    "id: revise-demo\nprompt: Add.\nkind: revise\n" + live + seed + "fault: {pod: checkout-0, type: errors, pct: 101}\n",
			wantErr: "fault.pct 101",
		},
		"fault type unknown": {
			body:    "id: revise-demo\nprompt: Add.\nkind: revise\n" + live + seed + "fault: {pod: checkout-0, type: slow, pct: 10}\n",
			wantErr: `fault.type "slow"`,
		},
		"panel value band inverted": {
			body:    "id: revise-demo\nprompt: Add.\nkind: revise\n" + live + seed + "expect:\n  panel_values:\n    - {query_contains: up, min: 2, max: 1}\n",
			wantErr: "finite min <= max",
		},
		"share above one": {
			body:    "id: create-demo\nprompt: Show.\n" + create + "category: prometheus\nexpect:\n  datasources: [prometheus]\n  rebuild_metric_share: 1.5\n",
			wantErr: "rebuild_metric_share",
		},
		"unknown key still rejected": {
			body:    "id: revise-demo\nprompt: Add.\nkind: revise\n" + live + seed + "expect:\n  max_calls: 3\n",
			wantErr: "max_calls",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := writeScenarios(t, tc.body)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("error %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func loadOne(t *testing.T, body string) Scenario {
	t.Helper()
	dir, err := writeScenarios(t, body)
	if err != nil {
		t.Fatal(err)
	}
	all, err := loadScenarios(dir)
	if err != nil {
		t.Fatal(err)
	}
	return all[0]
}

func TestScenarioDefaults(t *testing.T) {
	create := loadOne(t, "id: create-demo\nprompt: Show.\ncategory: prometheus\nkind: create\nsuites: [live]\nexpect:\n  datasources: [prometheus]\n")
	if create.Expect.MaxPanels != defaultMaxPanels || create.Expect.MinPanels != defaultMinPanels || create.Expect.MaxSeconds != defaultMaxSeconds {
		t.Fatalf("a create scenario got expect %+v, want the default panel and time limits", create.Expect)
	}

	smoke := loadOne(t, "id: smoke-demo\nprompt: Show.\ncategory: prometheus\nkind: create\nsuites: [live]\ntags: [replay-smoke]\nexpect:\n  datasources: [prometheus]\n")
	if got := strings.Join(smoke.Suites, ","); got != "live" {
		t.Fatalf("a replay-smoke scenario is in suites %q, want only the declared live", got)
	}

	revise := loadOne(t, "id: revise-demo\nprompt: Add.\nkind: revise\nsuites: [live]\nseed:\n  corpus: slashobserve/demo-shop.json\n")
	if revise.Expect.MaxPanels != 0 || revise.Expect.MinPanels != defaultMinPanels || revise.Expect.MaxSeconds != defaultMaxSeconds {
		t.Fatalf("a revise scenario got expect %+v, want max_panels left at 0", revise.Expect)
	}
}

type shopCalls struct {
	mu    sync.Mutex
	calls []string
}

func (s *shopCalls) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *shopCalls) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func startShop(t *testing.T, failPost bool) (*shopFaults, *shopCalls) {
	t.Helper()
	calls := &shopCalls{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.record(r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery)
		if r.Method == http.MethodPost && failPost {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	faults := newShopFaults(map[string]string{"checkout-1": srv.URL})
	faults.soakUnit = time.Millisecond
	return faults, calls
}

var errorFault = Fault{Pod: "checkout-1", Type: "errors", Pct: 30, SoakSeconds: 5}

func TestFaultIsSetSoakedRunAndCleared(t *testing.T) {
	faults, calls := startShop(t, false)
	ran := false
	err := faults.withFault(context.Background(), errorFault, func(context.Context) error {
		ran = true
		if got := calls.snapshot(); len(got) != 1 || got[0] != "POST /fault?pct=30&type=errors" {
			t.Errorf("calls before the task %v, want only the fault POST", got)
		}
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("ran %t, error %v", ran, err)
	}
	if got := calls.snapshot(); len(got) != 2 || got[1] != "DELETE /fault?" {
		t.Fatalf("calls %v, want the fault POST then a DELETE", got)
	}
}

func TestFaultIsClearedWhenTheTaskFailsOrPanics(t *testing.T) {
	boom := errors.New("task failed")
	faults, calls := startShop(t, false)
	if err := faults.withFault(context.Background(), errorFault, func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("error %v, want the task's error", err)
	}
	if got := calls.snapshot(); len(got) != 2 || got[1] != "DELETE /fault?" {
		t.Fatalf("calls after a failed task %v", got)
	}

	faults, calls = startShop(t, false)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic was swallowed")
			}
		}()
		_ = faults.withFault(context.Background(), errorFault, func(context.Context) error { panic("task panicked") })
	}()
	if got := calls.snapshot(); len(got) != 2 || got[1] != "DELETE /fault?" {
		t.Fatalf("calls after a panic %v", got)
	}
}

func TestFaultIsClearedAfterTheRunIsCancelled(t *testing.T) {
	t.Run("during the soak", func(t *testing.T) {
		faults, calls := startShop(t, false)
		faults.soakUnit = time.Hour
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			for len(calls.snapshot()) == 0 {
				time.Sleep(time.Millisecond)
			}
			cancel()
		}()
		err := faults.withFault(ctx, errorFault, func(context.Context) error {
			t.Error("the task ran after the cancel")
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error %v, want context.Canceled", err)
		}
		if got := calls.snapshot(); len(got) != 2 || got[1] != "DELETE /fault?" {
			t.Fatalf("calls %v, want the DELETE to go out on a context detached from the cancelled run", got)
		}
	})
	t.Run("during the task", func(t *testing.T) {
		faults, calls := startShop(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		err := faults.withFault(ctx, errorFault, func(ctx context.Context) error {
			cancel()
			return ctx.Err()
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error %v, want context.Canceled", err)
		}
		if got := calls.snapshot(); len(got) != 2 || got[1] != "DELETE /fault?" {
			t.Fatalf("calls %v, want the DELETE after the cancel", got)
		}
	})
}

func TestFaultThatCannotBeSetIsStillCleared(t *testing.T) {
	faults, calls := startShop(t, true)
	err := faults.withFault(context.Background(), errorFault, func(context.Context) error {
		t.Error("the task ran without its fault")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "set the errors fault on checkout-1") {
		t.Fatalf("error %v, want the failed POST", err)
	}
	if got := calls.snapshot(); len(got) != 2 || got[1] != "DELETE /fault?" {
		t.Fatalf("calls %v, want a DELETE in case the POST half applied", got)
	}
}

func TestFaultNeedsAShopForThePod(t *testing.T) {
	faults, _ := startShop(t, false)
	err := faults.withFault(context.Background(), Fault{Pod: "checkout-9", Type: "errors", Pct: 5}, func(context.Context) error { return nil })
	if err == nil || !strings.Contains(err.Error(), `no --shop-url for pod "checkout-9"`) {
		t.Fatalf("error %v", err)
	}
}

func TestFaultsRunOneAtATime(t *testing.T) {
	faults, _ := startShop(t, false)
	var active, peak atomic.Int32
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			_ = faults.withFault(context.Background(), errorFault, func(context.Context) error {
				peak.Store(max(peak.Load(), active.Add(1)))
				time.Sleep(10 * time.Millisecond)
				active.Add(-1)
				return nil
			})
		})
	}
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("%d faulted tasks at once, want 1", peak.Load())
	}
}

func TestShopURLsDefaultFromThePortsAndTakeOverrides(t *testing.T) {
	env := map[string]string{"SLASHOBSERVE_EVALS_SHOP_PORT": "31000"}
	getenv := func(name string) string { return env[name] }

	var flagged shopURLs
	for _, v := range []string{"checkout-1=http://127.0.0.1:32000/", "checkout-2=http://shop2:8080"} {
		if err := flagged.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	got := resolveShopURLs(flagged, getenv)
	want := map[string]string{
		"checkout-0": "http://127.0.0.1:31000",
		"checkout-1": "http://127.0.0.1:32000",
		"checkout-2": "http://shop2:8080",
	}
	for pod, url := range want {
		if got[pod] != url {
			t.Errorf("%s resolves to %q, want %q", pod, got[pod], url)
		}
	}

	defaults := resolveShopURLs(nil, func(string) string { return "" })
	if defaults["checkout-0"] != "http://127.0.0.1:28081" || defaults["checkout-1"] != "http://127.0.0.1:28082" {
		t.Fatalf("defaults %v, want the evals stack ports", defaults)
	}
}

func TestShopURLFlagRejectsMalformedValues(t *testing.T) {
	for _, v := range []string{"checkout-0", "=http://x:1", "checkout-0=ftp://x", "checkout-0=http://", "checkout-0=localhost:8081"} {
		var flagged shopURLs
		if err := flagged.Set(v); err == nil {
			t.Errorf("%q was accepted", v)
		}
	}
}

func TestClientEndpointsCarryTheDocumentedBodies(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		seen[key] = body
		mu.Unlock()
		switch key {
		case "POST /api/v1/dashboards/d1/revisions":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"revision":{"id":"r1","dashboard_id":"d1","status":"queued"}}`)
		case "POST /api/v1/dashboards/d1/suggestions":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"suggestion":{"id":"s1","status":"queued"}}`)
		case "GET /api/v1/dashboards/d1/suggestions/s1":
			_, _ = io.WriteString(w, `{"suggestion":{"id":"s1","status":"succeeded","panel":{"id":"3"},"failure_code":null}}`)
		case "GET /api/v1/dashboards/d1/versions/2":
			_, _ = io.WriteString(w, `{"version":{"version":2,"spec":{"specVersion":"1"},"unverified":{"panels":[{"panel_id":"3","issues":[]}],"variables":[{"name":"v","issues":[]}]}}}`)
		case "GET /api/v1/dashboards/d1/import":
			_, _ = io.WriteString(w, `{"id":"i1","dashboard_id":"d1","version":1,"title":"Shop"}`)
		default:
			t.Errorf("unexpected call %s", key)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := newClient(srv.URL, secretToken)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	rev, err := c.createRevision(ctx, "d1", 2, "3", "Fix it.")
	if err != nil || rev.ID != "r1" || rev.DashboardID == nil || *rev.DashboardID != "d1" {
		t.Fatalf("revision %+v, %v", rev, err)
	}
	if body := seen["POST /api/v1/dashboards/d1/revisions"]; body["prompt"] != "Fix it." || body["base_version"] != float64(2) || body["panel_id"] != "3" {
		t.Fatalf("revision body %v", body)
	}
	if _, err := c.createRevision(ctx, "d1", 2, "", "Add a panel."); err != nil {
		t.Fatal(err)
	}
	if _, has := seen["POST /api/v1/dashboards/d1/revisions"]["panel_id"]; has {
		t.Fatal("an empty target must send no panel_id")
	}

	sug, err := c.createSuggestion(ctx, "d1", 2, "3", "Better.")
	if err != nil || sug.ID != "s1" {
		t.Fatalf("suggestion %+v, %v", sug, err)
	}
	if body := seen["POST /api/v1/dashboards/d1/suggestions"]; body["version"] != float64(2) || body["panel_id"] != "3" || body["prompt"] != "Better." {
		t.Fatalf("suggestion body %v", body)
	}
	done, err := c.getSuggestion(ctx, "d1", "s1")
	if err != nil || !done.finished() || string(done.Panel) != `{"id":"3"}` || done.FailureCode != nil {
		t.Fatalf("suggestion %+v, %v", done, err)
	}

	version, err := c.getVersion(ctx, "d1", 2)
	if err != nil || version.Version != 2 || !version.Unverified.hasPanel("3") || version.Unverified.hasPanel("4") || len(version.Unverified.Variables) != 1 {
		t.Fatalf("version %+v, %v", version, err)
	}
	record, err := c.dashboardImport(ctx, "d1")
	if err != nil || record.ID != "i1" || record.Title != "Shop" {
		t.Fatalf("import record %+v, %v", record, err)
	}
}
