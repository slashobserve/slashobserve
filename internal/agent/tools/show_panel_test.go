package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

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
	res, err := call(t, r, nameShowPanel, `{"datasource":"fake","query":"rate(up[5m])","from":"now-6h","title":"Up rate","type":"barchart"}`)
	if err != nil {
		t.Fatal(err)
	}
	app := decodeApp(t, res)
	if app.Title != "Up rate" || app.Panel.Type != "barchart" || app.Panel.Queries[0].Query != "rate(up[5m])" {
		t.Errorf("panel = %q %q %+v", app.Title, app.Panel.Type, app.Panel.Queries)
	}
	if app.From != "now-6h" || app.To != "now" {
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
			res, err := call(t, newTestRegistry(t, seriesDS(tt.typ), nil), nameShowPanel, `{"datasource":"fake","query":"up","from":"now-3h","to":"now-1h"}`)
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
	if res, err := call(t, r, nameShowPanel, `{"datasource":"fake","query":"SELECT 1","from":"now-6h"}`); err != nil || res.IsError {
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
			res, err := call(t, r, nameShowPanel, tt.input)
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
	res, err := call(t, newTestRegistry(t, ds, nil), nameShowPanel, `{"datasource":"fake","query":"broken("}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != string(queryproxy.CodeQueryInvalid) || len(res.App) != 0 {
		t.Fatalf("code = %q app = %s", code, res.App)
	}
}

func TestShowPanelRefusesDataOverTheSizeAChartCanBeSentIn(t *testing.T) {
	ds := &fakeDS{rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
		var frames []datasource.Frame
		for i := range 40 {
			frames = append(frames, timeNumberFrame(map[string]string{"series": strings.Repeat(string(rune('a'+i%26)), 60<<10)}, 1, 2))
		}
		return frames, nil
	}}
	res, err := call(t, newTestRegistry(t, ds, nil), nameShowPanel, `{"datasource":"fake","query":"up"}`)
	if err != nil {
		t.Fatal(err)
	}
	if code := errorCode(t, res); code != string(queryproxy.CodeLimitExceeded) || len(res.App) != 0 {
		t.Fatalf("code = %q app = %d bytes", code, len(res.App))
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

func TestResolveSavedRangeAppliesTheDashboardRules(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	rng, res, ok := resolveSavedRange("now-90d", "now", now, specTimeOptions())
	if !ok || rng.To.Sub(rng.From) != 90*24*time.Hour {
		t.Fatalf("rng = %+v res = %#v", rng, res)
	}
	for _, bad := range [][2]string{{"now", "now-1h"}, {"yesterday", "now"}, {strings.Repeat("a", maxTimeExprBytes+1), "now"}} {
		if _, res, ok := resolveSavedRange(bad[0], bad[1], now, specTimeOptions()); ok || !res.IsError {
			t.Errorf("%v: ok = %v res = %#v", bad, ok, res)
		}
	}
}

func specTimeOptions() spec.TimeOptions { return spec.TimeOptions{} }

func TestShowPanelDrawsAPanelOfASavedDashboard(t *testing.T) {
	rig := newPanelsRig(t)
	id := rig.seedStats(t)
	caller := rig.as(rig.ctx)
	res, _ := caller.run(t, nameShowPanel, fmt.Sprintf(`{"dashboard_id":%q,"panel":"p1","from":"now-2h"}`, id))
	app := decodeApp(t, res)
	if app.Title != "One" || app.Panel.Type != "stat" || app.From != "now-2h" || app.To != "now" {
		t.Errorf("app = %q %q %q to %q", app.Title, app.Panel.Type, app.From, app.To)
	}
	link, err := url.Parse(app.OpenURL)
	if err != nil || !strings.HasPrefix(link.Path, "/d/"+id) || link.Query().Get("viewPanel") != "p1" || link.Query().Get("from") != "now-2h" {
		t.Errorf("link = %s (%v)", app.OpenURL, err)
	}
	if len(app.Variables["svc"]) == 0 {
		t.Errorf("variables = %v", app.Variables)
	}
	for _, missing := range []string{"nope", "r1"} {
		res, _ := caller.run(t, nameShowPanel, fmt.Sprintf(`{"dashboard_id":%q,"panel":%q}`, id, missing))
		if code := errorCode(t, res); code != string(queryproxy.CodePanelNotFound) {
			t.Errorf("%s: code = %q", missing, code)
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
