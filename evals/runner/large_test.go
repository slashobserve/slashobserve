package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/replay"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func largeAnalyzed(t *testing.T) map[string]analyzed {
	t.Helper()
	dashboards, err := loadCorpus(testCorpusRoot, largeDashboardSet())
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]analyzed, len(dashboards))
	for _, d := range dashboards {
		a, err := analyze(d)
		if err != nil {
			t.Fatal(err)
		}
		out[d.key] = a
	}
	return out
}

func TestTheDefaultSampleIsTheLargeDashboards(t *testing.T) {
	sample, err := loadSample(filepath.Join("..", "corpus-sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Sorted(maps.Keys(sample))
	want := slices.Sorted(maps.Keys(largeDashboardSet()))
	if !slices.Equal(got, want) || len(largeDashboards) != 8 {
		t.Fatalf("sample = %v, largeDashboards = %v", got, want)
	}
}

func TestLargeDashboardsLoadWithEnoughDataPanels(t *testing.T) {
	prometheusPicks := largeDashboards[:6]
	all := largeAnalyzed(t)
	for _, key := range largeDashboards {
		a, ok := all[key]
		if !ok {
			t.Fatalf("%s did not load from the corpus", key)
		}
		panels := len(a.dash.DataPanels())
		want := 18
		if slices.Contains(prometheusPicks, key) {
			want = 40
		}
		if panels < want {
			t.Errorf("%s has %d data panels, want at least %d", key, panels, want)
		}
	}
}

func TestSynthSeriesCoverEveryPrometheusSelectorOfTheLargeDashboards(t *testing.T) {
	useCorpus(t)
	out := t.TempDir()
	if _, err := writeSynth(out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(out, synthSeriesFile)) //nolint:gosec // a path inside the test's temporary directory
	if err != nil {
		t.Fatal(err)
	}
	var metrics []synthMetric
	if err := json.Unmarshal(body, &metrics); err != nil {
		t.Fatal(err)
	}
	served := map[string]bool{}
	for _, m := range metrics {
		served[m.Name] = true
	}
	for key, a := range largeAnalyzed(t) {
		for _, ref := range queryRefs(a.dash) {
			if ref.kind != spec.Prometheus {
				continue
			}
			sels, err := promSelectors(ref)
			if err != nil {
				continue
			}
			for _, s := range sels {
				if !excludedMetric(s.metric) && !served[s.metric] {
					t.Errorf("%s: no synthetic series for %s", key, s.metric)
				}
			}
		}
	}
}

func TestRebuildPromptListsRowsInOrderWithTheRealCounts(t *testing.T) {
	groups := []titleGroup{
		{Titles: []string{"a", "b"}},
		{Row: "R1", Titles: []string{"c", "d"}},
		{Row: "R2", Titles: []string{"e"}},
	}
	want := `Build a dashboard titled "Shop" with 5 panels in 2 rows. Above the rows: a; b. Row "R1": c; d. Row "R2": e.`
	if got := rebuildPrompt("Shop", groups); got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestRebuildPromptDropsTrailingTitlesToFitAndKeepsTheCounts(t *testing.T) {
	var titles []string
	for i := range 600 {
		titles = append(titles, fmt.Sprintf("Panel number %d", i))
	}
	groups := []titleGroup{{Titles: titles[:300]}, {Row: "Tail", Titles: titles[300:]}}
	prompt := rebuildPrompt("Big", groups)
	if n := utf8.RuneCountInString(prompt); n > maxPromptRunes || n < maxPromptRunes-40 {
		t.Fatalf("prompt has %d characters, want it just under %d", n, maxPromptRunes)
	}
	if !strings.Contains(prompt, "with 600 panels in 1 rows.") || strings.Contains(prompt, "Panel number 599") || !strings.Contains(prompt, "Panel number 0;") {
		t.Fatalf("prompt = %.200q", prompt)
	}
	if strings.Contains(prompt, `Row "Tail"`) {
		t.Fatal("a row whose titles were all dropped is still listed")
	}
}

func TestTitleGroupsKeepUniqueTitlesInDashboardOrder(t *testing.T) {
	a := largeAnalyzed(t)[demoShop]
	groups := titleGroups(a.dash)
	panels, rows := groupTotals(groups)
	if panels != len(a.titles) {
		t.Fatalf("groups hold %d titles, want the %d unique data titles", panels, len(a.titles))
	}
	if rows != len(groups)-1 && rows != len(groups) {
		t.Fatalf("%d rows in %d groups", rows, len(groups))
	}
	seen := map[string]bool{}
	for _, g := range groups {
		for _, title := range g.Titles {
			if seen[title] {
				t.Fatalf("title %q is listed twice", title)
			}
			seen[title] = true
		}
	}
}

func TestLargeLiveRebuildsNameTheirRealCountsAndFitThePrompt(t *testing.T) {
	live := planOf(t, suiteLive)
	for key, a := range largeAnalyzed(t) {
		sc := taskOf(t, live, taskRebuild, key).scenario
		panels, rows := groupTotals(titleGroups(a.dash))
		if want := fmt.Sprintf("with %d panels in %d rows.", panels, rows); !strings.Contains(sc.Prompt, want) {
			t.Errorf("%s: prompt does not contain %q", key, want)
		}
		if n := utf8.RuneCountInString(sc.Prompt); n > maxPromptRunes {
			t.Errorf("%s: prompt has %d characters", key, n)
		}
		ex := sc.Expect
		wantMin := (panels*8 + 9) / 10
		wantMax := min(200, panels+(panels+3)/4)
		if ex.MinPanels != wantMin || ex.MaxPanels != wantMax || ex.MaxSeconds != int(serverRunLimit.Seconds()) {
			t.Errorf("%s: expect min %d max %d seconds %d, want %d, %d, %d", key, ex.MinPanels, ex.MaxPanels, ex.MaxSeconds, wantMin, wantMax, int(serverRunLimit.Seconds()))
		}
		if ex.RebuildMetricShare != rebuildMetricShare || ex.RebuildMetricCap != rebuildMetricCap || !slices.Equal(sc.Suites, []string{suiteLive}) || !slices.Contains(sc.Tags, taskRebuild) {
			t.Errorf("%s: scenario = %+v", key, sc)
		}
	}
}

func TestReplayRebuildsExistOnlyForLargeDashboards(t *testing.T) {
	plan := planOf(t, suiteReplay)
	var got, want []string
	for _, tk := range plan.tasks {
		if strings.HasPrefix(tk.scenario.ID, "corpus-rebuild-") {
			got = append(got, tk.scenario.ID)
		}
	}
	for _, a := range largeAnalyzed(t) {
		want = append(want, plan.id(taskRebuild, a))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("replay rebuilds = %v, want %v", got, want)
	}
}

func TestTheNodeExporterFullRebuildFixtureSavesOnePanelPerTurn(t *testing.T) {
	a := largeAnalyzed(t)[nodeExporterFull]
	fixture, shape, err := rebuildFixture(a)
	if err != nil {
		t.Fatal(err)
	}
	if shape.panels != 125 || len(fixture.Turns) != shape.panels || !fixture.ResumeAfterSaves {
		t.Fatalf("fixture has %d turns for %d panels, resume_after_saves %v", len(fixture.Turns), shape.panels, fixture.ResumeAfterSaves)
	}
	for i, turn := range fixture.Turns {
		if len(turn.ToolCalls) != 1 {
			t.Fatalf("turn %d has %d calls", i, len(turn.ToolCalls))
		}
		call := turn.ToolCalls[0]
		if i == 0 {
			var create createInput
			if err := json.Unmarshal(call.Input, &create); err != nil || call.Name != toolCreateDashboard || create.PlannedPanels != shape.panels {
				t.Fatalf("first turn = %s planned %d, err %v", call.Name, create.PlannedPanels, err)
			}
			body, err := json.Marshal(create.Spec)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := spec.Parse(body); err != nil {
				t.Fatalf("the first save does not parse: %v", err)
			}
			continue
		}
		var save panelsInput
		if err := json.Unmarshal(call.Input, &save); err != nil || call.Name != toolUpdatePanels || len(save.Upsert) != 1 {
			t.Fatalf("turn %d = %s with %d upserts, err %v", i, call.Name, len(save.Upsert), err)
		}
		if save.Finish != (i == len(fixture.Turns)-1) {
			t.Fatalf("turn %d finish = %v", i, save.Finish)
		}
		if save.Upsert[0].GridPos.X != 0 || save.Upsert[0].GridPos.Y != 0 || save.Upsert[0].GridPos.W == 0 {
			t.Fatalf("turn %d gridPos = %+v, want only the size", i, save.Upsert[0].GridPos)
		}
	}
}

func TestRebuildFixturesHealEveryQueryAndLoadInTheReplayProvider(t *testing.T) {
	dir := t.TempDir()
	scenarios := map[string]Scenario{}
	for _, tk := range planOf(t, suiteReplay).tasks {
		if tk.fixture == nil || !strings.HasPrefix(tk.scenario.ID, "corpus-rebuild-") {
			continue
		}
		body, err := json.Marshal(tk.fixture)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, tk.scenario.ID+fixtureSuffix), body, 0o600); err != nil {
			t.Fatal(err)
		}
		scenarios[tk.fixture.Prompt] = tk.scenario
		for i, turn := range tk.fixture.Turns {
			if len(turn.ToolCalls[0].Input) > int(corpusTokenCap*replayBytesPerToken) {
				t.Errorf("%s turn %d is %d bytes, over the replay token cap", tk.scenario.ID, i, len(turn.ToolCalls[0].Input))
			}
			if strings.Contains(string(turn.ToolCalls[0].Input), `"${`) {
				t.Errorf("%s turn %d still references a variable", tk.scenario.ID, i)
			}
		}
		ex := tk.scenario.Expect
		if ex.MinPanels != ex.MaxPanels || ex.MinPanels != len(tk.fixture.Turns) || len(ex.Metrics) > 0 || len(ex.Tables) > 0 {
			t.Errorf("%s: expect = %+v for %d turns", tk.scenario.ID, ex, len(tk.fixture.Turns))
		}
	}
	if len(scenarios) != len(largeDashboards) {
		t.Fatalf("%d rebuild fixtures, want %d", len(scenarios), len(largeDashboards))
	}
	provider, err := replay.New(dir, corpusTokenCap, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	for prompt := range scenarios {
		session, err := provider.Start(context.Background(), llm.Request{Prompt: prompt, BuildSaves: 3})
		if err != nil {
			t.Fatalf("resume after three saves: %v", err)
		}
		turn, err := session.Next(context.Background(), nil)
		if err != nil || turn.Stop != llm.StopToolUse {
			t.Fatalf("resumed turn = %+v, %v", turn, err)
		}
	}
}

func promPanel(id, title, query string) spec.Panel {
	return spec.Panel{ID: id, Type: spec.Timeseries, Title: title, Datasource: "prom", Queries: []spec.Query{{Query: query}}}
}

func workDash(panels ...spec.Panel) analyzed {
	return analyzed{dash: &spec.Dashboard{Panels: panels}}
}

func rowOf(id string, children ...spec.Panel) spec.Panel {
	return spec.Panel{ID: id, Type: spec.Row, Title: "Row " + id, Panels: children}
}

func TestPickWorkTargetPrefersAPanelEveryHostServes(t *testing.T) {
	cases := []struct {
		name   string
		dash   analyzed
		wantID string
		wantOK bool
	}{
		{
			"the first panel that every host serves",
			workDash(promPanel("a", "Pressure", `rate(node_pressure_cpu_waiting_seconds_total[5m])`), promPanel("b", "Load", `node_load1`), promPanel("c", "Load 5", `node_load5`)),
			"b", true,
		},
		{
			"a panel that mixes a served and an unserved metric is not preferred",
			workDash(promPanel("a", "Mixed", `node_load1 / node_pressure_io_waiting_seconds_total`), promPanel("b", "CPU", `rate(node_cpu_seconds_total[5m])`)),
			"b", true,
		},
		{
			"the first titled panel with a query when none qualifies",
			workDash(promPanel("a", "", `node_load1`), promPanel("b", "Disk", `node_disk_io_now`), promPanel("c", "Net", `node_netstat_Tcp_CurrEstab`)),
			"b", true,
		},
		{"no titled data panel", workDash(promPanel("a", "", `up`)), "", false},
		{"no panel at all", workDash(), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entry, ok := pickWorkTarget(c.dash)
			if ok != c.wantOK || (ok && entry.Panel.ID != c.wantID) {
				t.Fatalf("pick = %v %v, want %q %v", ok, entry.Panel, c.wantID, c.wantOK)
			}
		})
	}
}

func TestPickWorkTargetOnTheNodeExporterFullSkipsThePressurePanel(t *testing.T) {
	a := largeAnalyzed(t)[nodeExporterFull]
	entry, ok := pickWorkTarget(a)
	if !ok || entry.Panel.ID == a.dash.DataPanels()[0].Panel.ID || !hostIndependent(a.dash, entry.Panel) {
		t.Fatalf("pick = %v %v, want a host-independent panel after the first data panel", ok, entry.Panel)
	}
}

func TestPickWorkRowTakesThreeToEightChildrenAndPrefersServedMetrics(t *testing.T) {
	served := func(prefix string, n int) []spec.Panel {
		var out []spec.Panel
		for i := range n {
			out = append(out, promPanel(fmt.Sprintf("%s%d", prefix, i), "Load", `node_load1`))
		}
		return out
	}
	unserved := promPanel("u", "Disk", `node_disk_io_now`)
	cases := []struct {
		name   string
		dash   analyzed
		wantID string
		wantOK bool
	}{
		{"too small and too big rows are passed over", workDash(rowOf("r1", served("a", 2)...), rowOf("r2", served("b", 9)...), rowOf("r3", served("c", 3)...)), "r3", true},
		{"a row of served panels beats an earlier row with another metric", workDash(rowOf("r1", append(served("a", 2), unserved)...), rowOf("r2", served("b", 4)...)), "r2", true},
		{"the first row of three to eight when none is served", workDash(rowOf("r1", unserved, unserved, unserved), rowOf("r2", served("b", 3)...)), "r2", true},
		{"a placeholder child makes a row unfit", workDash(rowOf("r1", served("a", 2)[0], served("a", 2)[1], spec.Panel{ID: "ph", Type: spec.Placeholder})), "", false},
		{"no rows", workDash(promPanel("a", "A", `node_load1`)), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row, ok := pickWorkRow(c.dash)
			if ok != c.wantOK || (ok && row.ID != c.wantID) {
				t.Fatalf("pick = %v %v, want %q %v", ok, row, c.wantID, c.wantOK)
			}
		})
	}
}

func replayTaskOf(t *testing.T, name, key string) task {
	t.Helper()
	tk := taskOf(t, planOf(t, suiteReplay), name, key)
	if tk.fixture == nil {
		t.Fatalf("%s has no fixture", tk.scenario.ID)
	}
	return tk
}

func TestEveryGeneratorYieldsAScenarioAndAReplayFixtureOnTheNodeExporterFull(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{taskChange, taskRetime, taskSuggest, taskRowedit} {
		liveTask := taskOf(t, planOf(t, suiteLive), name, nodeExporterFull)
		replayTask := replayTaskOf(t, name, nodeExporterFull)
		for _, sc := range []Scenario{liveTask.scenario, replayTask.scenario} {
			if err := validateScenario(sc); err != nil {
				t.Errorf("%s: %v", sc.ID, err)
			}
			if !slices.Contains(sc.Tags, name) || !slices.Contains(sc.Tags, tagCorpus) || sc.Seed == nil || sc.Seed.Corpus != nodeExporterFull {
				t.Errorf("%s: tags %v seed %+v", sc.ID, sc.Tags, sc.Seed)
			}
		}
		if liveTask.fixture != nil {
			t.Errorf("%s: the live plan carries a fixture", name)
		}
		body, err := json.Marshal(replayTask.fixture)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, replayTask.scenario.ID+fixtureSuffix), body, 0o600); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(replayTask.scenario.Prompt, " ["+replayTask.scenario.ID+"]") {
			t.Errorf("%s: replay prompt %q has no marker", name, replayTask.scenario.Prompt)
		}
	}
	if _, err := replay.New(dir, corpusTokenCap, discardLogger()); err != nil {
		t.Fatalf("the replay provider rejects the generated fixtures: %v", err)
	}
}

func TestChangeRetimeSuggestAndRoweditExpectWhatTheyPromise(t *testing.T) {
	live := planOf(t, suiteLive)
	change := taskOf(t, live, taskChange, nodeExporterFull).scenario
	if change.Prompt != `Show the panel "{panel_title}" ({panel_id}) as a stat panel.` || change.Seed.Target != "p20" {
		t.Errorf("change = %q on %q", change.Prompt, change.Seed.Target)
	}
	if ex := change.Expect; !ex.OnlyTargetChanged || !ex.TargetReturnsData || ex.TargetType != "stat" || ex.MaxSeconds != 300 {
		t.Errorf("change expect = %+v", ex)
	}
	retime := taskOf(t, live, taskRetime, nodeExporterFull).scenario
	if ex := retime.Expect; retime.Prompt != "Default this dashboard to the last 6 hours." || !slices.Equal(ex.TimeRangeFrom, []string{"now-6h"}) || !slices.Equal(ex.ChangedPaths, []string{"/time"}) ||
		ex.MaxSaveOutputTokens != 2000 || ex.MaxSeconds != 90 || ex.MaxModelCalls != 5 {
		t.Errorf("retime = %q %+v", retime.Prompt, ex)
	}
	suggest := taskOf(t, live, taskSuggest, nodeExporterFull).scenario
	if suggest.Kind != kindSuggest || suggest.Prompt != "Suggest a clearer title for this panel." || suggest.Seed.Target != change.Seed.Target {
		t.Errorf("suggest = %+v", suggest)
	}
	rowedit := taskOf(t, live, taskRowedit, nodeExporterFull).scenario
	if !strings.Contains(rowedit.Prompt, `"{row_title}"`) || rowedit.Expect.ChangedPanels != 4 || len(rowedit.Expect.ChangedPaths) != 4 || rowedit.Expect.MaxSeconds != 600 {
		t.Errorf("rowedit = %q %+v", rowedit.Prompt, rowedit.Expect)
	}
	for _, path := range rowedit.Expect.ChangedPaths {
		if !strings.HasPrefix(path, "/panels/"+rowedit.Seed.Row+"/panels/") || !strings.HasSuffix(path, "/description") {
			t.Errorf("live changed path %q", path)
		}
	}
	for _, path := range taskOf(t, planOf(t, suiteReplay), taskRowedit, nodeExporterFull).scenario.Expect.ChangedPaths {
		if strings.HasSuffix(path, "/description") {
			t.Errorf("replay changed path %q should cover the whole child", path)
		}
	}
}

func TestReplayGeneratorFixturesApplyAndChangeWhatTheyPromise(t *testing.T) {
	a := largeAnalyzed(t)[nodeExporterFull]
	change := replayTaskOf(t, taskChange, nodeExporterFull)
	var save panelsInput
	if err := json.Unmarshal(change.fixture.Turns[0].ToolCalls[0].Input, &save); err != nil {
		t.Fatal(err)
	}
	if len(change.fixture.Turns) != 1 || len(save.Upsert) != 1 || save.Upsert[0].Type != spec.Stat || !save.Finish || save.Upsert[0].Queries[0].Query != healthyPromQuery {
		t.Fatalf("change fixture = %+v", save)
	}
	if strings.Contains(change.fixture.Prompt, "{panel") {
		t.Fatalf("change fixture prompt = %q", change.fixture.Prompt)
	}
	rowedit := replayTaskOf(t, taskRowedit, nodeExporterFull)
	if len(rowedit.fixture.Turns) != rowedit.scenario.Expect.ChangedPanels || strings.Contains(rowedit.fixture.Prompt, "{row_title}") {
		t.Fatalf("rowedit fixture has %d turns, prompt %q", len(rowedit.fixture.Turns), rowedit.fixture.Prompt)
	}
	current := a.dash
	for i, turn := range rowedit.fixture.Turns {
		var patch spec.PanelPatch
		if err := json.Unmarshal(turn.ToolCalls[0].Input, &patch); err != nil {
			t.Fatal(err)
		}
		next, err := spec.ApplyPanelPatch(current, patch)
		if err != nil {
			t.Fatal(err)
		}
		current = next
		var one panelsInput
		if err := json.Unmarshal(turn.ToolCalls[0].Input, &one); err != nil {
			t.Fatal(err)
		}
		if one.Finish != (i == len(rowedit.fixture.Turns)-1) {
			t.Fatalf("turn %d finish = %v", i, one.Finish)
		}
	}
	changed := map[string]bool{}
	for _, c := range spec.Diff(a.dash, current) {
		id, ok := panelOfPath(c.Path)
		if !ok || !underAny(c.Path, rowedit.scenario.Expect.ChangedPaths) {
			t.Fatalf("change at %s is outside %v", c.Path, rowedit.scenario.Expect.ChangedPaths)
		}
		changed[id] = true
	}
	if len(changed) != rowedit.scenario.Expect.ChangedPanels {
		t.Fatalf("%d panels changed, want %d", len(changed), rowedit.scenario.Expect.ChangedPanels)
	}
	retime := replayTaskOf(t, taskRetime, nodeExporterFull)
	var patch spec.PanelPatch
	if err := json.Unmarshal(retime.fixture.Turns[0].ToolCalls[0].Input, &patch); err != nil {
		t.Fatal(err)
	}
	saved, err := spec.ApplyPanelPatch(a.dash, patch)
	if err != nil || saved.Time.From != "now-6h" {
		t.Fatalf("retime saved %v, err %v", saved.Time, err)
	}
	for _, c := range spec.Diff(a.dash, saved) {
		if !underAny(c.Path, retime.scenario.Expect.ChangedPaths) {
			t.Fatalf("retime changed %s", c.Path)
		}
	}
}

func TestEveryLargeDashboardGetsEveryGeneratorItHasAVerifiedTargetFor(t *testing.T) {
	all := largeAnalyzed(t)
	for _, suite := range []string{suiteLive, suiteReplay} {
		p := planOf(t, suite)
		for _, key := range largeDashboards {
			a := all[key]
			_, hasTarget := pickWorkTarget(a)
			_, hasRow := pickWorkRow(a)
			_, hasRange := pickRetimeRange(a.dash)
			wanted := map[string]bool{taskFix: true, taskAdd: true, taskPaste: true, taskRebuild: true, taskChange: hasTarget, taskRetime: hasRange, taskSuggest: hasTarget, taskRowedit: hasRow}
			for name, want := range wanted {
				id := p.id(name, a)
				if got := slices.ContainsFunc(p.tasks, func(tk task) bool { return tk.scenario.ID == id }); got != want {
					t.Errorf("%s suite: task %s present = %v, want %v", suite, id, got, want)
				}
			}
		}
	}
}

func TestWorkTargetsAreVerifiedAtImport(t *testing.T) {
	all := largeAnalyzed(t)
	for _, key := range largeDashboards {
		a := all[key]
		if entry, ok := pickWorkTarget(a); ok && !verifiedAtImport(key, entry.Panel.ID) {
			t.Errorf("%s: target %s is unverified at import", key, entry.Panel.ID)
		}
		if row, ok := pickWorkRow(a); ok {
			for _, child := range row.Panels {
				if !verifiedAtImport(key, child.ID) {
					t.Errorf("%s: row %s has the unverified child %s", key, row.ID, child.ID)
				}
			}
		}
	}
	if _, ok := pickWorkTarget(all["victoriametrics/vmalert.json"]); ok {
		t.Error("vmalert has no panel verified at import, yet it got a target")
	}
	if _, ok := pickWorkRow(all["victoriametrics/vmalert.json"]); ok {
		t.Error("vmalert has no row verified at import, yet it got a row")
	}
	if _, ok := pickWorkTarget(all[nodeExporterFull]); !ok {
		t.Error("the node exporter dashboard lost its target")
	}
}

func TestPickRetimeRangeStaysWithinTheStacksHistory(t *testing.T) {
	cases := []struct {
		name   string
		from   string
		factor int
		want   string
	}{
		{"one step per point on a day default", "now-24h", 1, "now-6h"},
		{"the dashboard already defaults to six hours", "now-6h", 1, "now-3h"},
		{"a factor of 2 on a three hour default", "now-3h", 2, "now-1h"},
		{"a factor of 10 on a half hour default", "now-30m", 10, "now-15m"},
		{"a factor of 10 on a quarter hour default", "now-15m", 10, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dash := &spec.Dashboard{Time: spec.TimeRange{From: c.from, To: "now"}, Panels: []spec.Panel{{ID: "p1", Type: spec.Timeseries, Title: "A", Queries: []spec.Query{{Query: "up", IntervalFactor: c.factor}}}}}
			got, ok := pickRetimeRange(dash)
			if got.from != c.want || ok != (c.want != "") {
				t.Errorf("pickRetimeRange = %q, %v; want %q", got.from, ok, c.want)
			}
		})
	}
}

func TestVerifiedAtImportReadsTheMeasuredGaps(t *testing.T) {
	for key, ids := range unverifiedAtImport {
		if !slices.Contains(largeDashboards, key) || len(strings.Fields(ids)) == 0 {
			t.Errorf("%s is not a large dashboard with gaps", key)
		}
	}
	if verifiedAtImport("cilium/cilium-dashboard.json", "p76") || !verifiedAtImport("cilium/cilium-dashboard.json", "p1") || !verifiedAtImport("rfmoz/node-exporter-full.json", "p76") {
		t.Error("verifiedAtImport disagrees with the measured gaps")
	}
}

func TestOtherDashboardsGetNoneOfTheLargeOnlyGenerators(t *testing.T) {
	for _, tk := range planOf(t, suiteLive).tasks {
		sc := tk.scenario
		large := sc.Seed != nil && slices.Contains(largeDashboards, sc.Seed.Corpus)
		for _, name := range []string{taskChange, taskRetime, taskSuggest, taskRowedit} {
			if slices.Contains(sc.Tags, name) && !large {
				t.Errorf("%s is %s on a dashboard outside largeDashboards", sc.ID, name)
			}
		}
	}
}

func TestNewExpectationsAreValidated(t *testing.T) {
	base := func() Scenario {
		return Scenario{ID: "check-new-fields", Prompt: "Show it.", Kind: kindRevise, Suites: []string{suiteLive}, Seed: &Seed{Corpus: nodeExporterFull, Target: "p1"}, Expect: Expect{MinPanels: 1, MaxSeconds: 60}}
	}
	cases := []struct {
		name    string
		mutate  func(*Scenario)
		wantErr string
	}{
		{"a stat target type", func(sc *Scenario) { sc.Expect.TargetType = "stat" }, ""},
		{"one changed panel", func(sc *Scenario) { sc.Expect.ChangedPanels = 1 }, ""},
		{"an unknown target type", func(sc *Scenario) { sc.Expect.TargetType = "gizmo" }, "data panel type"},
		{"a row is no target type", func(sc *Scenario) { sc.Expect.TargetType = "row" }, "data panel type"},
		{"a negative changed panel count", func(sc *Scenario) { sc.Expect.ChangedPanels = -1 }, "changed_panels"},
		{"a target with break_panel", func(sc *Scenario) { sc.Seed.BreakPanel = true }, "seed.target"},
		{"target_type needs a seed", func(sc *Scenario) { sc.Seed, sc.Kind, sc.Expect.TargetType = nil, kindCreate, "stat" }, "seed is required"},
		{"the row placeholder needs a seed", func(sc *Scenario) { sc.Seed, sc.Kind, sc.Prompt = nil, kindCreate, `Edit "{row_title}".` }, "seed is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sc := base()
			c.mutate(&sc)
			applyDefaults(&sc)
			err := validateScenario(sc)
			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want %q", err, c.wantErr)
			}
		})
	}
}

func rowDash(descriptions ...string) *spec.Dashboard {
	var children []spec.Panel
	for i, d := range descriptions {
		child := promPanel(fmt.Sprintf("c%d", i), "Child", `node_load1`)
		child.Description = d
		children = append(children, child)
	}
	return &spec.Dashboard{Panels: []spec.Panel{rowOf("r", children...), promPanel("top", "Top", `node_load1`)}}
}

func TestCheckChangedPanelsCountsPanelsWithAnyDiffPath(t *testing.T) {
	parent := rowDash("", "", "")
	cases := []struct {
		name  string
		saved *spec.Dashboard
		want  int
		pass  bool
	}{
		{"two of three children", rowDash("x", "x", ""), 2, true},
		{"all three children", rowDash("x", "x", "x"), 3, true},
		{"two changed where three are expected", rowDash("x", "x", ""), 3, false},
		{"a panel outside the row counts as well", func() *spec.Dashboard {
			d := rowDash("x", "x", "x")
			d.Panels[1].Title = "Renamed"
			return d
		}(), 3, false},
		{"nothing changed", rowDash("", "", ""), 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkChangedPanels(Expect{ChangedPanels: c.want}, observation{dashboard: c.saved, parent: parent})
			if got.Passed != c.pass {
				t.Fatalf("check = %+v", got)
			}
		})
	}
	if got := checkChangedPanels(Expect{ChangedPanels: 1}, observation{parent: parent}); got.Passed {
		t.Fatal("a missing spec passed")
	}
	if got := checkChangedPanels(Expect{ChangedPanels: 1}, observation{dashboard: parent}); got.Passed {
		t.Fatal("a missing parent passed")
	}
}

func TestCheckTargetType(t *testing.T) {
	d := &spec.Dashboard{Panels: []spec.Panel{promPanel("t", "T", `up`)}}
	d.Panels[0].Type = spec.Stat
	if got := checkTargetType(Expect{TargetType: "stat"}, observation{dashboard: d, target: "t"}); !got.Passed {
		t.Fatalf("stat target failed: %+v", got)
	}
	if got := checkTargetType(Expect{TargetType: "timeseries"}, observation{dashboard: d, target: "t"}); got.Passed {
		t.Fatal("a stat target passed as a time series")
	}
	if got := checkTargetType(Expect{TargetType: "stat"}, observation{dashboard: d, target: "gone"}); got.Passed {
		t.Fatal("a missing target passed")
	}
}

func TestReviseChecksRunTheTargetChecksOnlyWhenSet(t *testing.T) {
	names := func(ex Expect) []string {
		var out []string
		for _, c := range reviseChecks(ex, observation{}) {
			out = append(out, c.Name)
		}
		return out
	}
	plain := names(Expect{})
	if !slices.Contains(plain, "new_panel_data") || slices.Contains(plain, "only_target_changed") || slices.Contains(plain, "target_type") {
		t.Fatalf("plain revise checks = %v", plain)
	}
	change := names(Expect{OnlyTargetChanged: true, TargetReturnsData: true, TargetType: "stat"})
	for _, want := range []string{"only_target_changed", "target_data", "target_type"} {
		if !slices.Contains(change, want) {
			t.Errorf("change checks %v miss %s", change, want)
		}
	}
	if slices.Contains(change, "new_panel_data") {
		t.Errorf("change checks %v still demand a new panel", change)
	}
	rowedit := names(Expect{ChangedPaths: []string{"/panels/r"}, ChangedPanels: 3})
	if !slices.Contains(rowedit, "changed_panels") || !slices.Contains(rowedit, "changed_paths") || slices.Contains(rowedit, "new_panel_data") {
		t.Errorf("rowedit checks = %v", rowedit)
	}
}

func TestSeedTargetAndRowFollowTheScenario(t *testing.T) {
	c := startImports(t, &fakeImports{})
	got, err := seedScenario(context.Background(), c, Scenario{Seed: &Seed{Corpus: demoShop, Target: "p3", Row: "p1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.targetID != "p3" || got.rowTitle == "" {
		t.Fatalf("seeded %+v, want target p3 and the title of row p1", got)
	}
	prompt, err := fillPrompt(Scenario{Prompt: `Edit "{row_title}" and {panel_id}.`}, got)
	if err != nil || strings.Contains(prompt, "{") {
		t.Fatalf("prompt = %q, err %v", prompt, err)
	}
}

func TestSeedFailsWhenTheChosenTargetOrARowChildWasUnverified(t *testing.T) {
	cases := []struct {
		name       string
		seed       Seed
		unverified []string
		want       string
	}{
		{"target", Seed{Corpus: demoShop, Target: "p3"}, []string{"p3"}, "the target panel p3 was unverified at import"},
		{"row child", Seed{Corpus: demoShop, Row: "p1"}, []string{"p2"}, "was unverified at import"},
		{"unrelated panel", Seed{Corpus: demoShop, Target: "p3"}, []string{"p4"}, ""},
		{"not a data panel", Seed{Corpus: demoShop, Target: "p1"}, nil, "not a data panel"},
		{"not a row", Seed{Corpus: demoShop, Row: "p3"}, nil, "not a row"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := startImports(t, &fakeImports{unverified: tc.unverified})
			_, err := seedScenario(context.Background(), c, Scenario{Seed: &tc.seed})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
