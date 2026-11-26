package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func callApps(t *testing.T, r *Registry, name, input string) (Result, error) {
	t.Helper()
	ctx := auth.WithPrincipal(reading(WithAppsHost(context.Background())), auth.Principal{Role: auth.RoleAdmin})
	return r.Call(ctx, name, []byte(input))
}

func seriesDS(typ string) *fakeDS {
	return &fakeDS{typ: typ, rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return []datasource.Frame{timeNumberFrame(map[string]string{"route": "/a"}, 1, 2, 3)}, nil
	}}
}

func decodeApp(t *testing.T, res Result) panelAppOut {
	t.Helper()
	if res.IsError || len(res.App) == 0 {
		t.Fatalf("res = %#v", res)
	}
	var out panelAppOut
	decoder := json.NewDecoder(bytes.NewReader(res.App))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestShowPanelDrawsAQueryWithTheDataAndASummary(t *testing.T) {
	r := newTestRegistry(t, seriesDS("prometheus"), nil)
	res, err := callApps(t, r, nameShowPanel, `{"datasource":"fake","query":"rate(up[5m])","from":"now-6h","title":"Up rate","type":"barchart"}`)
	if err != nil {
		t.Fatal(err)
	}
	app := decodeApp(t, res)
	if app.Title != "Up rate" || app.Panel.Type != "barchart" || app.Panel.Queries[0].Query != "rate(up[5m])" {
		t.Errorf("panel = %q %q %+v", app.Title, app.Panel.Type, app.Panel.Queries)
	}
	from, errFrom := time.Parse(time.RFC3339, app.From)
	to, errTo := time.Parse(time.RFC3339, app.To)
	if errFrom != nil || errTo != nil || to.Sub(from) < 6*time.Hour-time.Minute || to.Sub(from) > 6*time.Hour+time.Minute {
		t.Errorf("range = %q to %q", app.From, app.To)
	}
	var data struct {
		Frames []json.RawMessage `json:"frames"`
		Meta   struct {
			Kind string `json:"kind"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(app.Data, &data); err != nil || len(data.Frames) != 1 || data.Meta.Kind != "range" {
		t.Errorf("data = %s (%v)", app.Data, err)
	}
	if len(app.Datasources) != 1 || app.Datasources[0] != (panelAppDatasource{UID: "fake", Type: "prometheus"}) {
		t.Errorf("datasources = %+v", app.Datasources)
	}
	out, ok := res.Content.(shownPanelOut)
	if !ok || out.Frames != 1 || len(out.Series) != 1 || out.OpenURL != app.OpenURL {
		t.Fatalf("content = %#v", res.Content)
	}
	if !strings.HasPrefix(res.Step, "Showed a panel: Up rate: 1 series") {
		t.Errorf("step = %q", res.Step)
	}
}

func TestShowPanelLinksToTheExplorerThatFitsTheDatasource(t *testing.T) {
	tests := []struct {
		typ  string
		path string
	}{
		{"prometheus", "/explore/metrics/fake"},
		{"tempo", "/traces/fake"},
		{"jaeger", "/traces/fake"},
		{"loki", "/logs/fake"},
		{"clickhouse", "/logs/fake"},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			res, err := callApps(t, newTestRegistry(t, seriesDS(tt.typ), nil), nameShowPanel, `{"datasource":"fake","query":"up","from":"now-3h","to":"now-1h"}`)
			if err != nil {
				t.Fatal(err)
			}
			link, err := url.Parse(decodeApp(t, res).OpenURL)
			if err != nil {
				t.Fatal(err)
			}
			query := link.Query()
			if link.Path != tt.path || query.Get("q") != "up" || query.Get("from") != "now-3h" || query.Get("to") != "now-1h" {
				t.Errorf("link = %s", link)
			}
		})
	}
}

func TestShowPanelRunsUnderTheLimitsOfQueryRange(t *testing.T) {
	var seen datasource.Query
	var steps []time.Duration
	ds := &fakeDS{typ: "clickhouse", rangeFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange, step time.Duration) ([]datasource.Frame, error) {
		seen = q
		steps = append(steps, step)
		return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
	}}
	r := newTestRegistry(t, ds, nil)
	if res, err := callApps(t, r, nameShowPanel, `{"datasource":"fake","query":"SELECT 1","from":"now-6h"}`); err != nil || res.IsError {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if seen.Limits != queryproxy.DraftLimits {
		t.Errorf("the datasource saw limits %+v", seen.Limits)
	}
	want := queryproxy.StepFor(datasource.TimeRange{From: time.Unix(0, 0), To: time.Unix(6*3600, 0)}, queryproxy.AgentMaxDataPoints)
	if len(steps) != 1 || steps[0] != want {
		t.Errorf("step = %v, want %v", steps, want)
	}
}

func TestShowPanelRefusesWhatQueryRangeRefuses(t *testing.T) {
	r := newTestRegistry(t, seriesDS(""), nil)
	tests := []struct{ name, input string }{
		{"neither a query nor a panel", `{}`},
		{"both a query and a dashboard", `{"datasource":"fake","query":"up","dashboard_id":"00000000-0000-0000-0000-000000000001","panel":"p"}`},
		{"a datasource without a query", `{"datasource":"fake"}`},
		{"a dashboard without a panel", `{"dashboard_id":"00000000-0000-0000-0000-000000000001"}`},
		{"a type with a dashboard", `{"dashboard_id":"00000000-0000-0000-0000-000000000001","panel":"p","type":"stat"}`},
		{"a title with a dashboard", `{"dashboard_id":"00000000-0000-0000-0000-000000000001","panel":"p","title":"t"}`},
		{"a panel without a dashboard", `{"panel":"p"}`},
		{"a panel with a query", `{"datasource":"fake","query":"up","panel":"p"}`},
		{"a dashboard id that is not a uuid", `{"dashboard_id":"x","panel":"p"}`},
		{"variables with a dashboard", `{"dashboard_id":"00000000-0000-0000-0000-000000000001","panel":"p","variables":{"a":["b"]}}`},
		{"unknown type", `{"datasource":"fake","query":"up","type":"geomap"}`},
		{"data points too high", `{"datasource":"fake","query":"up","max_data_points":1001}`},
		{"range over 31 days", `{"datasource":"fake","query":"up","from":"now-32d"}`},
		{"from after to", `{"datasource":"fake","query":"up","from":"now","to":"now-1h"}`},
		{"unknown datasource", `{"datasource":"nope","query":"up"}`},
		{"long title", `{"datasource":"fake","query":"up","title":"` + strings.Repeat("a", maxShowTitleChars+1) + `"}`},
		{"bad variable name", `{"datasource":"fake","query":"up","variables":{"1bad":["x"]}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := callApps(t, r, nameShowPanel, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError || len(res.App) != 0 {
				t.Fatalf("expected a tool error, got %#v", res)
			}
		})
	}
}

func TestShowPanelReportsADatasourceFailureWithoutApp(t *testing.T) {
	ds := &fakeDS{rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "parse error")
	}}
	res, err := callApps(t, newTestRegistry(t, ds, nil), nameShowPanel, `{"datasource":"fake","query":"broken("}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != string(queryproxy.CodeQueryInvalid) || len(res.App) != 0 {
		t.Fatalf("code = %q app = %s", code, res.App)
	}
}

func oversizeDS() *fakeDS {
	return &fakeDS{rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		var frames []datasource.Frame
		for i := range 40 {
			frames = append(frames, timeNumberFrame(map[string]string{"series": strings.Repeat(string(rune('a'+i%26)), 60<<10)}, 1, 2))
		}
		return frames, nil
	}}
}

func TestShowPanelAnswersAnAppsHostWithTheSummaryAndANoteWhenTheDataIsTooLargeToDraw(t *testing.T) {
	res, err := callApps(t, newTestRegistry(t, oversizeDS(), nil), nameShowPanel, `{"datasource":"fake","query":"up"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.App) != 0 {
		t.Fatalf("res = %#v app = %d bytes", res, len(res.App))
	}
	out, ok := res.Content.(shownPanelOut)
	if !ok || out.Frames != 40 || out.OpenURL == "" {
		t.Fatalf("content = %#v", res.Content)
	}
	if !strings.Contains(res.Step, "The chart is not drawn") || !strings.Contains(res.Step, "query_range") {
		t.Errorf("step = %q", res.Step)
	}
}

func TestShowPanelGivesAClientWithoutAppsTheSummaryWhateverTheSizeOfTheData(t *testing.T) {
	res, err := call(t, newTestRegistry(t, oversizeDS(), nil), nameShowPanel, `{"datasource":"fake","query":"up"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.App) != 0 {
		t.Fatalf("res = %#v app = %d bytes", res, len(res.App))
	}
	if strings.Contains(res.Step, "not drawn") {
		t.Errorf("step = %q", res.Step)
	}
	if out, ok := res.Content.(shownPanelOut); !ok || out.Frames != 40 {
		t.Fatalf("content = %#v", res.Content)
	}
}

func TestShowPanelBuildsNoChartDataForAClientWithoutApps(t *testing.T) {
	res, err := call(t, newTestRegistry(t, seriesDS("prometheus"), nil), nameShowPanel, `{"datasource":"fake","query":"up"}`)
	if err != nil || res.IsError || len(res.App) != 0 {
		t.Fatalf("res = %#v err = %v", res, err)
	}
	if out, ok := res.Content.(shownPanelOut); !ok || out.Frames != 1 || out.OpenURL == "" {
		t.Fatalf("content = %#v", res.Content)
	}
}

func TestShowPanelLinksToTheQueryTheVariablesGaveWhenThereAreVariables(t *testing.T) {
	ds := &fakeDS{typ: "prometheus", rangeFn: func(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
		text := q.Text
		for _, v := range q.Variables {
			text = strings.ReplaceAll(text, "$"+v.Name, strings.Join(v.Values, "|"))
		}
		q.Expanded(text)
		return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
	}}
	res, err := callApps(t, newTestRegistry(t, ds, nil), nameShowPanel, `{"datasource":"fake","query":"up{job=\"$job\"}","variables":{"job":["api"]}}`)
	if err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(decodeApp(t, res).OpenURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := link.Query().Get("q"); strings.Contains(got, "$job") || !strings.Contains(got, `job="api"`) {
		t.Errorf("q = %q", got)
	}
}

func TestShowPanelLinksToTheQueryAsGivenWithoutVariables(t *testing.T) {
	r := newTestRegistry(t, seriesDS("prometheus"), nil)
	res, err := callApps(t, r, nameShowPanel, `{"datasource":"fake","query":"rate(up[$__rate_interval])"}`)
	if err != nil {
		t.Fatal(err)
	}
	link, err := url.Parse(decodeApp(t, res).OpenURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := link.Query().Get("q"); got != "rate(up[$__rate_interval])" {
		t.Errorf("q = %q", got)
	}
}

func TestShowPanelIsAViewerToolAndNoRunOffersIt(t *testing.T) {
	for _, tool := range New(Deps{}).List() {
		if tool.Name != nameShowPanel {
			continue
		}
		if tool.MinRole != "viewer" || !tool.ReadOnly || !tool.MCPOnly || tool.OfferedIn(ModeCreate) || tool.OfferedIn(ModeInvestigate) {
			t.Fatalf("tool = %+v", tool)
		}
		return
	}
	t.Fatal("show_panel is not registered")
}

func TestShowPanelRefusesAnOverlongTimeExpression(t *testing.T) {
	if _, ok := checkTimeExprs("now-1h", "now"); !ok {
		t.Fatal("a short expression was refused")
	}
	if res, ok := checkTimeExprs(strings.Repeat("a", maxTimeExprBytes+1), "now"); ok || !res.IsError {
		t.Fatalf("res = %#v ok = %v", res, ok)
	}
}

func savedDashboard() *spec.Dashboard {
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "Shop",
		Time:        spec.TimeRange{From: "now-6h", To: "now"},
		Timezone:    "UTC",
		Variables:   []spec.Variable{{Name: "ds", Type: spec.DatasourceVariable, DatasourceType: spec.Prometheus}},
		Panels: []spec.Panel{
			{ID: "adhoc", Type: spec.Timeseries, Title: "Saved under the ad hoc id", Datasource: "${ds}", TimeShift: "1h", Queries: []spec.Query{{Query: "up"}}},
			{ID: "note", Type: spec.Text, Title: "Notes"},
		},
	}
}

func showSaved(t *testing.T, input string) (Result, error) {
	t.Helper()
	r := newTestRegistry(t, seriesDS("prometheus"), nil)
	var in showPanelInput
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithPrincipal(reading(WithAppsHost(context.Background())), auth.Principal{Role: auth.RoleAdmin})
	shown, res, ok, err := r.panelOfDashboard(ctx, "00000000-0000-0000-0000-000000000001", savedDashboard(), in, 200)
	if err != nil || !ok {
		return res, err
	}
	return r.drawPanel(ctx, shown)
}

func TestShowPanelDrawsASavedPanelWhoseIDIsAdhocAsADashboardPanel(t *testing.T) {
	res, err := showSaved(t, `{"panel":"adhoc"}`)
	if err != nil {
		t.Fatal(err)
	}
	app := decodeApp(t, res)
	if app.Title != "Saved under the ad hoc id" || strings.Contains(app.OpenURL, "/explore") || !strings.Contains(app.OpenURL, "viewPanel=adhoc") {
		t.Errorf("title = %q link = %s", app.Title, app.OpenURL)
	}
}

func TestShowPanelListsTheDatasourceAVariableSelectsInTheSavedPanel(t *testing.T) {
	res, err := showSaved(t, `{"panel":"adhoc"}`)
	if err != nil {
		t.Fatal(err)
	}
	app := decodeApp(t, res)
	if len(app.Datasources) != 1 || app.Datasources[0].UID != fakeUID || app.Datasources[0].Type != "prometheus" {
		t.Fatalf("datasources = %+v", app.Datasources)
	}
	if app.Variables["ds"][0] != fakeUID {
		t.Errorf("variables = %v", app.Variables)
	}
}

func TestShowPanelReportsTheRangeItQueriedAfterTheShiftOfThePanel(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	res, err := showSaved(t, `{"panel":"adhoc","from":"now-2h","to":"now"}`)
	if err != nil {
		t.Fatal(err)
	}
	app := decodeApp(t, res)
	from, errFrom := time.Parse(time.RFC3339, app.From)
	to, errTo := time.Parse(time.RFC3339, app.To)
	if errFrom != nil || errTo != nil {
		t.Fatalf("from %q to %q", app.From, app.To)
	}
	if got := to.Sub(from); got < 2*time.Hour-time.Minute || got > 2*time.Hour+time.Minute {
		t.Errorf("queried %v, want about two hours", got)
	}
	if time.Since(to) < 59*time.Minute {
		t.Errorf("to %v does not carry the one hour shift (now %v)", to, now)
	}
}

func TestShowPanelRefusesATextPanelWithALinkToTheDashboard(t *testing.T) {
	res, err := showSaved(t, `{"panel":"note"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != codeInvalidRequest {
		t.Fatalf("code = %q", code)
	}
	if text := fmt.Sprint(res.Content); !strings.Contains(text, "text panel") || !strings.Contains(text, "/d/") {
		t.Errorf("refusal = %s", text)
	}
	res, err = showSaved(t, `{"panel":"missing"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != string(queryproxy.CodePanelNotFound) {
		t.Errorf("code = %q", code)
	}
}

func TestShowPanelRefusesARangeOverFourHundredDaysOfASavedPanel(t *testing.T) {
	res, err := showSaved(t, `{"panel":"adhoc","from":"now-401d"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != codeInvalidRequest {
		t.Fatalf("code = %q", code)
	}
}

func TestTheChartTypesOfTheSchemaAreThoseOfTheViteRendererList(t *testing.T) {
	var schema struct {
		Properties struct {
			Type struct {
				Enum []string `json:"enum"`
			} `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schemaShowPanel), &schema); err != nil {
		t.Fatal(err)
	}
	if got := schema.Properties.Type.Enum; !slices.Equal(got, chartTypeNames()) {
		t.Errorf("schema enum = %v, chart types = %v", got, chartTypeNames())
	}
	manifests, err := filepath.Glob("../../../web/src/panels/*/manifest.ts")
	if err != nil || len(manifests) == 0 {
		t.Fatalf("manifests = %v (%v)", manifests, err)
	}
	typeLine := regexp.MustCompile(`(?m)^\s*type: "([^"]+)",`)
	var built []string
	for _, path := range manifests {
		raw, err := os.ReadFile(path) //nolint:gosec // paths come from a glob of the repo
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`(?m)^\s*mcpApp: true,`).Match(raw) {
			continue
		}
		match := typeLine.FindSubmatch(raw)
		if match == nil {
			t.Fatalf("%s sets mcpApp without a type", path)
		}
		built = append(built, string(match[1]))
	}
	slices.Sort(built)
	want := slices.Sorted(slices.Values(chartTypeNames()))
	if !slices.Equal(built, want) {
		t.Errorf("manifests with mcpApp = %v, chart types = %v", built, want)
	}
}

func TestShowPanelDrawsAPanelOfASavedDashboard(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	caller := rig.as(WithAppsHost(rig.ctx))
	res, _ := caller.run(t, nameShowPanel, fmt.Sprintf(`{"dashboard_id":%q,"panel":"p1","from":"now-2h"}`, id))
	app := decodeApp(t, res)
	if app.Title != "One" || app.Panel.Type != "stat" || app.From == "" || app.To == "" {
		t.Errorf("app = %q %q %q to %q", app.Title, app.Panel.Type, app.From, app.To)
	}
	link, err := url.Parse(app.OpenURL)
	if err != nil || !strings.HasPrefix(link.Path, "/d/"+id) || link.Query().Get("viewPanel") != "p1" || link.Query().Get("from") != "now-2h" {
		t.Errorf("link = %s (%v)", app.OpenURL, err)
	}
	if len(app.Variables["svc"]) == 0 {
		t.Errorf("variables = %v", app.Variables)
	}
	for panel, want := range map[string]string{"nope": string(queryproxy.CodePanelNotFound), "r1": codeInvalidRequest} {
		res, _ := caller.run(t, nameShowPanel, fmt.Sprintf(`{"dashboard_id":%q,"panel":%q}`, id, panel))
		if code := errorCode(t, res); code != want {
			t.Errorf("%s: code = %q, want %q", panel, code, want)
		}
	}
	res, _ = caller.run(t, nameShowPanel, `{"dashboard_id":"00000000-0000-0000-0000-000000000001","panel":"p1"}`)
	if code := errorCode(t, res); code != codeNotFound {
		t.Errorf("unknown dashboard: code = %q", code)
	}
}

func TestTheRecordedResultTheWebTestsRenderMatchesThePayloadOfTheTool(t *testing.T) {
	raw, err := os.ReadFile("../../../web/src/mcp-panel/fixtures/show-panel-result.json")
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Content           []map[string]string `json:"content"`
		StructuredContent panelAppOut         `json:"structuredContent"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&recorded); err != nil {
		t.Fatal(err)
	}
	app := recorded.StructuredContent
	if app.Title == "" || app.Panel.ID == "" || len(app.Data) == 0 || app.OpenURL == "" || len(app.Datasources) == 0 || app.From == "" {
		t.Errorf("recorded payload = %+v", app)
	}
	var summary shownPanelOut
	if len(recorded.Content) != 1 || json.Unmarshal([]byte(recorded.Content[0]["text"]), &summary) != nil || summary.OpenURL != app.OpenURL {
		t.Errorf("recorded text = %v", recorded.Content)
	}
}
