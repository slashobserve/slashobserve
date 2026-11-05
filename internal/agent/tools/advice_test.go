package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func queryPanel(id string, typ spec.PanelType, queries ...string) spec.Panel {
	p := spec.Panel{ID: id, Type: typ, Unit: "short", Description: "Described."}
	for _, q := range queries {
		p.Queries = append(p.Queries, spec.Query{Query: q, Legend: "{{pod}}"})
	}
	return p
}

func TestAdviceUncoveredFamilies(t *testing.T) {
	shop := []datasource.Family{
		{Name: "http_request_duration_seconds", Type: "histogram"},
		{Name: "http_requests", Type: "counter"},
		{Name: "go_goroutines", Type: "gauge"},
		{Name: "otel_traces", Type: "traces"},
		{Name: "up"},
	}
	tests := []struct {
		name      string
		panels    []spec.Panel
		inventory []datasource.Family
		want      []string
		wantTotal int
	}{
		{"no panel queries a family", nil, shop,
			[]string{"http_request_duration_seconds", "http_requests", "go_goroutines", "otel_traces", "up"}, 5},
		{"a histogram is covered by its buckets", []spec.Panel{queryPanel("p95", spec.Timeseries, `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))`)}, shop[:1],
			[]string{}, 0},
		{"a counter is covered by its _total series", []spec.Panel{queryPanel("rps", spec.Timeseries, `sum(rate(http_requests_total[5m]))`)}, shop[1:2],
			[]string{}, 0},
		{"a longer name sharing the prefix does not cover", []spec.Panel{queryPanel("errs", spec.Timeseries, `sum(rate(http_requests_errors_total[5m]))`, `go_goroutines_max`)}, shop[1:3],
			[]string{"http_requests", "go_goroutines"}, 2},
		{"a qualified ClickHouse table is covered", []spec.Panel{queryPanel("spans", spec.Table, "SELECT count() FROM otel.otel_traces WHERE $__timeFilter(Timestamp)")}, shop[3:4],
			[]string{}, 0},
		{"a panel inside a row counts", []spec.Panel{{ID: "r", Type: spec.Row, Panels: []spec.Panel{queryPanel("u", spec.Stat, `sum(up{job="shop"})`)}}}, shop[4:],
			[]string{}, 0},
		{"an empty inventory leaves nothing uncovered", []spec.Panel{queryPanel("u", spec.Stat, "up")}, []datasource.Family{},
			[]string{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adviceFor(&spec.Dashboard{Panels: tt.panels}, tt.inventory)
			if got.coverage == nil {
				t.Fatal("coverage is missing with an inventory")
			}
			if !slices.Equal(got.Uncovered, tt.want) || got.UncoveredTotal != tt.wantTotal {
				t.Fatalf("uncovered = %v (total %d), want %v (total %d)", got.Uncovered, got.UncoveredTotal, tt.want, tt.wantTotal)
			}
		})
	}
}

func TestAdviceCapsUncoveredAtFifty(t *testing.T) {
	inventory := make([]datasource.Family, 60)
	for i := range inventory {
		inventory[i] = datasource.Family{Name: fmt.Sprintf("m%02d", i)}
	}
	got := adviceFor(&spec.Dashboard{}, inventory)
	if len(got.Uncovered) != 50 || got.UncoveredTotal != 60 || got.Uncovered[49] != "m49" {
		t.Fatalf("uncovered = %d names ending %q, total %d; want 50 and 60", len(got.Uncovered), got.Uncovered[len(got.Uncovered)-1], got.UncoveredTotal)
	}
}

func TestAdviceWithoutAnInventoryHasNoCoverage(t *testing.T) {
	got := adviceFor(&spec.Dashboard{Panels: []spec.Panel{queryPanel("u", spec.Stat, "up")}}, nil)
	body, err := json.Marshal(savedOut{Panels: []panelOut{}, advice: got})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "uncovered") {
		t.Fatalf("result = %s, want no uncovered fields without an inventory", body)
	}
	covered := adviceFor(&spec.Dashboard{}, []datasource.Family{})
	body, err = json.Marshal(savedOut{Panels: []panelOut{}, advice: covered})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"uncovered":[],"uncovered_total":0`) {
		t.Fatalf("result = %s, want an empty uncovered list", body)
	}
}

func TestAdviceNotes(t *testing.T) {
	noLegend := func(p spec.Panel) spec.Panel {
		p.Queries[0].Legend = ""
		return p
	}
	noUnit := func(p spec.Panel) spec.Panel {
		p.Unit = ""
		return p
	}
	noDescription := func(p spec.Panel) spec.Panel {
		p.Description = ""
		return p
	}
	tests := []struct {
		name  string
		panel spec.Panel
		want  []string
	}{
		{"a complete timeseries", queryPanel("ts", spec.Timeseries, "up"), nil},
		{"a timeseries query without legend", noLegend(queryPanel("ts", spec.Timeseries, "up")), []string{"panel ts has a query with no legend; set legend when it returns several series"}},
		{"a barchart query without legend", noLegend(queryPanel("bars", spec.BarChart, "up")), []string{"panel bars has a query with no legend; set legend when it returns several series"}},
		{"a table needs no legend", noLegend(queryPanel("tbl", spec.Table, "up")), nil},
		{"a timeseries without unit", noUnit(queryPanel("ts", spec.Timeseries, "up")), []string{"panel ts is numeric and has no unit"}},
		{"a gauge without unit", noUnit(queryPanel("g", spec.Gauge, "up")), []string{"panel g is numeric and has no unit"}},
		{"a bar gauge without unit", noUnit(queryPanel("bg", spec.BarGauge, "up")), []string{"panel bg is numeric and has no unit"}},
		{"a table needs no unit", noUnit(queryPanel("tbl", spec.Table, "up")), nil},
		{"a stat without unit or description", noDescription(noUnit(queryPanel("s", spec.Stat, "up"))), []string{"panel s is numeric and has no unit", "stat panel s has no description"}},
		{"a gauge needs no description", noDescription(queryPanel("g", spec.Gauge, "up")), nil},
		{"a timeseries missing all three", noDescription(noUnit(noLegend(queryPanel("ts", spec.Timeseries, "up")))), []string{"panel ts has a query with no legend; set legend when it returns several series", "panel ts is numeric and has no unit"}},
		{"a row has no notes", spec.Panel{ID: "r", Type: spec.Row}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adviceFor(&spec.Dashboard{Panels: []spec.Panel{tt.panel}}, nil)
			if !slices.Equal(got.Notes, tt.want) {
				t.Fatalf("notes = %q, want %q", got.Notes, tt.want)
			}
		})
	}
}

func TestAdviceCapsNotesAtTwenty(t *testing.T) {
	var panels []spec.Panel
	for i := range 15 {
		p := queryPanel(fmt.Sprintf("s%d", i), spec.Stat, "up")
		p.Unit, p.Description = "", ""
		panels = append(panels, p)
	}
	if got := adviceFor(&spec.Dashboard{Panels: panels}, nil).Notes; len(got) != 20 || got[19] != "stat panel s9 has no description" {
		t.Fatalf("notes = %d ending %q, want 20", len(got), got[len(got)-1])
	}
}

func TestAdviceInventoryIsKeptPerRequestOrCaller(t *testing.T) {
	families := []datasource.Family{{Name: "up"}}
	user := func(id string) context.Context {
		return auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindUser, ID: id, Role: auth.RoleEditor})
	}
	runA := WithRun(user("advice-store-user"), Run{Mode: ModeCreate, RequestID: "advice-store-run-a"})
	runB := WithRun(user("advice-store-user"), Run{Mode: ModeCreate, RequestID: "advice-store-run-b"})
	inventories.put(runA, families)
	if got, ok := inventories.get(runA); !ok || !reflect.DeepEqual(got, families) {
		t.Fatalf("run a inventory = %v, %v", got, ok)
	}
	for name, ctx := range map[string]context.Context{
		"another run of the same requester": runB,
		"the requester outside the run":     user("advice-store-user"),
		"a run with no request id":          WithRun(user("advice-store-user"), Run{Mode: ModeCreate}),
		"a caller with no id":               user(""),
	} {
		if got, ok := inventories.get(ctx); ok {
			t.Errorf("%s sees inventory %v", name, got)
		}
	}
	inventories.put(user("advice-store-mcp"), nil)
	if got, ok := inventories.get(user("advice-store-mcp")); !ok || got == nil || len(got) != 0 {
		t.Fatalf("an empty scope must still record an inventory: %v, %v", got, ok)
	}
}

func TestAdviceStoreForgetsTheOldestInventory(t *testing.T) {
	run := func(i int) context.Context {
		return WithRun(context.Background(), Run{Mode: ModeCreate, RequestID: fmt.Sprintf("advice-evict-%d", i)})
	}
	for i := range maxStoredInventories + 1 {
		inventories.put(run(i), []datasource.Family{{Name: "up"}})
	}
	if _, ok := inventories.get(run(0)); ok {
		t.Fatal("the oldest inventory is still kept past the store size")
	}
	if _, ok := inventories.get(run(maxStoredInventories)); !ok {
		t.Fatal("the newest inventory is gone")
	}
}

func newAdviceRig(t *testing.T, families []datasource.Family) *Registry {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	frames := func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
		return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
	}
	ds := &fakeDS{
		rangeFn: func(ctx context.Context, q datasource.Query, tr datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
			return frames(ctx, q, tr)
		},
		instantFn:  frames,
		discoverFn: inventoryFake(families, false).discoverFn,
	}
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(database.DB)
	service := dashboards.New(database.DB, runner, access, logger)
	return New(Deps{Datasources: datasources, Runner: runner, Dashboards: service, Folders: access, Logger: logger})
}

func TestSaveResultsNameTheFamiliesNoPanelQueries(t *testing.T) {
	r := newAdviceRig(t, []datasource.Family{
		{Name: "http_requests_total", Type: "counter"},
		{Name: "http_request_duration_seconds", Type: "histogram"},
		{Name: "up"},
	})
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Kind: auth.KindServiceAccount, ID: "a1d71ce0-0000-4000-8000-000000000542", Name: "ci", Role: auth.RoleEditor})
	call := func(tool, input string) Result {
		t.Helper()
		res, err := r.Call(ctx, tool, []byte(input))
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		return res
	}
	if res := call(nameListMetrics, `{"datasource":"fake","scope":"{job=\"shop\"}"}`); res.IsError {
		t.Fatalf("list_metrics = %#v", res.Content)
	}

	created := savedVersion(t, call(nameCreateDashboard, `{"spec":{"specVersion":"0","title":"Shop","time":{"from":"now-1h","to":"now"},"panels":[
		{"id":"rps","type":"timeseries","title":"Requests","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"fake","queries":[{"query":"sum(rate(http_requests_total[5m]))"}]}]}}`))
	if !slices.Equal(created.Uncovered, []string{"http_request_duration_seconds", "up"}) || created.UncoveredTotal != 2 {
		t.Fatalf("create uncovered = %v (total %d)", created.Uncovered, created.UncoveredTotal)
	}
	wantNotes := []string{"panel rps has a query with no legend; set legend when it returns several series", "panel rps is numeric and has no unit"}
	if !slices.Equal(created.Notes, wantNotes) {
		t.Fatalf("create notes = %q, want %q", created.Notes, wantNotes)
	}

	patched := savedVersion(t, call(nameUpdatePanels, fmt.Sprintf(`{"dashboard_id":%q,"parent_version":1,"summary":"Add the rest","upsert":[
		{"id":"p95","type":"timeseries","title":"p95","gridPos":{"x":12,"y":0,"w":12,"h":8},"datasource":"fake","unit":"s","queries":[{"query":"histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))","legend":"p95"}]},
		{"id":"targets","type":"stat","title":"Targets up","description":"Scrape targets that answer.","gridPos":{"x":0,"y":8,"w":6,"h":4},"datasource":"fake","unit":"short","queries":[{"query":"sum(up)"}]}]}`, created.DashboardID)))
	if patched.Uncovered == nil || len(patched.Uncovered) != 0 || patched.UncoveredTotal != 0 {
		t.Fatalf("update_panels uncovered = %v (total %d), want an empty list", patched.Uncovered, patched.UncoveredTotal)
	}
	if !slices.Equal(patched.Notes, wantNotes) {
		t.Fatalf("update_panels notes = %q, want only the first panel's", patched.Notes)
	}
}

func TestBuildSavesNameUncoveredFamiliesUntilTheFinish(t *testing.T) {
	rig := newPanelsRig(t)
	run := rig.createRun(t)
	inventories.put(run.ctx, []datasource.Family{{Name: "http_requests_total", Type: "counter"}, {Name: "node_load1"}})

	res, _ := run.run(t, nameCreateDashboard, buildCreateJSON(suggestStat("rps", "Requests", "sum(rate(http_requests_total[5m]))", 0), `,"planned_panels":2`))
	first, ok := res.Content.(buildOut)
	if res.IsError || !ok {
		t.Fatalf("first save = %#v, want a build save", res.Content)
	}
	if first.coverage == nil || !slices.Equal(first.Uncovered, []string{"node_load1"}) || first.UncoveredTotal != 1 {
		t.Fatalf("first save advice = %+v, want node_load1 uncovered", first.advice)
	}
	if want := []string{"stat panel rps has no description", "panel rps is numeric and has no unit"}; !slices.Equal(slices.Sorted(slices.Values(first.Notes)), slices.Sorted(slices.Values(want))) {
		t.Fatalf("first save notes = %q, want %q", first.Notes, want)
	}

	res, _ = run.run(t, nameUpdatePanels, `{"summary":"Add load","upsert":[`+suggestStat("load", "Load", "node_load1", 4)+`],"finish":true}`)
	finished := savedVersion(t, res)
	if finished.coverage == nil || finished.Uncovered == nil || len(finished.Uncovered) != 0 || finished.UncoveredTotal != 0 {
		t.Fatalf("finish advice = %+v, want an empty uncovered list", finished.advice)
	}
}

func TestBuildAdviceShrinksTheChangedPanelsToStayUnderTheBudget(t *testing.T) {
	ctx := WithRun(context.Background(), Run{Mode: ModeCreate, Build: true, RequestID: "build-advice-budget", Attempt: 1})
	families := make([]datasource.Family, 80)
	for i := range families {
		families[i] = datasource.Family{Name: fmt.Sprintf("%s_%02d", strings.Repeat("f", 120), i)}
	}
	inventories.put(ctx, families)
	shown := make([]panelOut, 300)
	for i := range shown {
		shown[i] = panelOut{ID: fmt.Sprintf("p%03d", i), Title: strings.Repeat("t", 100), Type: spec.Stat}
	}
	var panels []spec.Panel
	for i := range 30 {
		panels = append(panels, spec.Panel{ID: fmt.Sprintf("s%02d", i), Type: spec.Stat})
	}

	out := buildAdvice(ctx, buildOut{ChangedPanels: shown, Message: "Saved."}, &spec.Dashboard{Panels: panels})
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > listBudget {
		t.Fatalf("build save result is %d bytes, over the %d byte budget", len(body), listBudget)
	}
	if len(out.Uncovered) != maxUncovered || out.UncoveredTotal != 80 || len(out.Notes) != maxNotes {
		t.Fatalf("advice = %d uncovered (total %d), %d notes; want all of it kept", len(out.Uncovered), out.UncoveredTotal, len(out.Notes))
	}
	if len(out.ChangedPanels) == 0 || len(out.ChangedPanels) == len(shown) {
		t.Fatalf("changed panels = %d, want some of the %d kept", len(out.ChangedPanels), len(shown))
	}
}
