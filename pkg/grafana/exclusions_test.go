package grafana

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDecisionsAreUniqueSentences(t *testing.T) {
	ids, reasons := map[string]bool{}, map[string]bool{}
	for _, d := range decisions {
		if d.id == "" || d.reason == "" {
			t.Errorf("decision %+v has an empty id or reason", d)
		}
		if !strings.HasSuffix(d.reason, ".") || strings.Count(d.reason, ". ") > 0 {
			t.Errorf("decision %s reason %q is not one sentence ending in a period", d.id, d.reason)
		}
		if ids[d.id] || reasons[d.reason] {
			t.Errorf("decision %s repeats an id or a reason", d.id)
		}
		ids[d.id], reasons[d.reason] = true, true
	}
}

func TestEveryExcludedOptionNamesAListedDecision(t *testing.T) {
	for _, row := range excludedOptions {
		if !slices.Contains(decisions, row.decision) {
			t.Errorf("row %s %q names the unlisted decision %q", row.scope, row.path, row.decision.id)
		}
	}
}

func TestExclusionFor(t *testing.T) {
	first := decision{"first", "First is excluded."}
	second := decision{"second", "Second is excluded."}
	object := map[string]any{
		"mode":   "field",
		"layers": []any{map[string]any{"type": "a"}, map[string]any{"type": "b"}},
		"nested": map[string]any{"flag": true},
		"axis":   "top",
	}
	valueAt := func(rel string) (any, bool) {
		switch rel {
		case "":
			return object, true
		case "axis":
			return "top", true
		case "layers[1]":
			return object["layers"].([]any)[1], true
		case "nested":
			return object["nested"], true
		}
		return nil, false
	}
	table := []excludedOption{
		{scope: panelScope(spec.Gauge), path: "axis", values: []any{"top", "bottom"}, decision: first},
		{scope: panelScope(spec.Stat), path: "axis", values: []any{"bottom"}, decision: second},
		{scope: optionScope("panel:*"), path: "layers[]", key: "type", values: []any{"b"}, decision: second},
		{scope: optionScope("transform:*"), path: "nested", key: "flag", values: []any{true}, decision: first},
		{scope: variableScope("custom"), key: "mode", values: []any{"field"}, decision: second},
		{scope: queryScope(queryAltinity), path: "streaming", decision: first},
		{scope: transformScope("timeSeriesTable"), path: "options.*.inlineStat", values: []any{true}, decision: second},
		{scope: scopeDashboard, path: "links[].icon", when: func(v any) bool { return v == "bolt" }, decision: first},
	}
	cases := []struct {
		name   string
		scoped scopedPath
		valid  func(rel string) (any, bool)
		want   string
	}{
		{"exact path with a listed value", scopedPath{scope: panelScope(spec.Gauge), rel: "axis"}, valueAt, "first"},
		{"a value the row does not list", scopedPath{scope: panelScope(spec.Stat), rel: "axis"}, valueAt, ""},
		{"another scope", scopedPath{scope: panelScope(spec.Table), rel: "axis"}, valueAt, ""},
		{"an unrelated path", scopedPath{scope: panelScope(spec.Gauge), rel: "other"}, valueAt, ""},
		{"a path that only shares a prefix", scopedPath{scope: panelScope(spec.Gauge), rel: "axisColor"}, valueAt, ""},
		{"wildcard scope with a member of an indexed object", scopedPath{scope: panelScope(spec.Table), rel: "layers[1]"}, valueAt, "second"},
		{"an ancestor row covers a deeper path", scopedPath{scope: panelScope(spec.Table), rel: "layers[1].config.kind"}, valueAt, "second"},
		{"the value is read at the row path, not at the reported path", scopedPath{scope: panelScope(spec.Table), rel: "layers[0].config"}, func(string) (any, bool) { return map[string]any{"type": "a"}, true }, ""},
		{"a nested key of an object", scopedPath{scope: transformScope("any"), rel: "nested.flag"}, valueAt, "first"},
		{"an empty row path reads the whole item", scopedPath{scope: variableScope("custom"), rel: ""}, valueAt, "second"},
		{"an empty row path covers every path of the item", scopedPath{scope: variableScope("custom"), rel: "options"}, valueAt, "second"},
		{"a value that cannot be read", scopedPath{scope: variableScope("custom"), rel: ""}, func(string) (any, bool) { return nil, false }, ""},
		{"a row without a value test covers any value", scopedPath{scope: queryScope(queryAltinity), rel: "streaming"}, valueAt, "first"},
		{"a wildcard segment", scopedPath{scope: transformScope("timeSeriesTable"), rel: "options.A.inlineStat"}, func(string) (any, bool) { return true, true }, "second"},
		{"a wildcard segment needs the path to be that deep", scopedPath{scope: transformScope("timeSeriesTable"), rel: "options.A"}, func(string) (any, bool) { return true, true }, ""},
		{"a predicate that holds", scopedPath{scope: scopeDashboard, rel: "links[0].icon"}, func(string) (any, bool) { return "bolt", true }, "first"},
		{"a predicate that fails", scopedPath{scope: scopeDashboard, rel: "links[0].icon"}, func(string) (any, bool) { return "alert", true }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := exclusionIn(table, tc.scoped, tc.valid)
			if tc.want == "" {
				if ok {
					t.Fatalf("decision = %q, want none", got.id)
				}
				return
			}
			if !ok || got.id != tc.want {
				t.Fatalf("decision = %q (%v), want %q", got.id, ok, tc.want)
			}
		})
	}
}

func TestExclusionForFirstMatchingRowWins(t *testing.T) {
	first, second := decision{"first", "First."}, decision{"second", "Second."}
	table := []excludedOption{
		{scope: scopeDashboard, path: "links[].icon", decision: first},
		{scope: scopeDashboard, path: "links[]", decision: second},
	}
	got, ok := exclusionIn(table, scopedPath{scope: scopeDashboard, rel: "links[0].icon"}, func(string) (any, bool) { return nil, false })
	if !ok || got.id != "first" {
		t.Fatalf("decision = %+v (%v), want the first row", got, ok)
	}
}

func TestIsDecisionReason(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{decisionAlertRules.reason, true},
		{decisionTransformationOptions.reason, true},
		{"Alertmanager is not supported yet.", false},
		{"", false},
		{strings.ToUpper(decisionLive.reason), false},
	}
	for _, tc := range cases {
		if got := isDecisionReason(tc.reason); got != tc.want {
			t.Errorf("isDecisionReason(%q) = %v, want %v", tc.reason, got, tc.want)
		}
	}
}

func TestDashboardNotesSplitByDecision(t *testing.T) {
	res := convertDoc(t, dashboardJSON(`"snapshot":{"key":"k"},"bogusOption":1,"editable":false`, panelJSON(1, 0, 0, 6, 4, "")))
	notes := notesFor(res, "dashboard options")
	reasonAt := map[string]string{}
	for _, n := range notes {
		for _, p := range n.Paths {
			reasonAt[p] = n.Reason
		}
	}
	want := map[string]string{
		"snapshot":    decisionSnapshots.reason,
		"editable":    decisionReadOnlyDashboards.reason,
		"bogusOption": "Some Grafana dashboard options are not supported yet.",
	}
	for path, reason := range want {
		if reasonAt[path] != reason {
			t.Errorf("reason at %s = %q, want %q (notes %+v)", path, reasonAt[path], reason, notes)
		}
	}
	if len(notes) != 3 {
		t.Errorf("notes = %+v, want one per distinct reason", notes)
	}
}

func TestPanelReportsUseTheDecisionOfExcludedOptions(t *testing.T) {
	_, line := convertedPanel(t, panelDoc("stat", `"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":"up"}],"fieldConfig":{"defaults":{"writeable":true,"unit":"bytes"},"overrides":[]},"options":{"bogusOption":1}`))
	if !slices.Contains(line.Reasons, decisionDataEditing.reason) || !slices.Contains(line.Reasons, "Some Grafana options of this panel are not supported yet.") {
		t.Fatalf("reasons = %v, want the decision and the ordinary reason", line.Reasons)
	}
	if !slices.Contains(line.Paths, "fieldConfig.defaults.writeable") || !slices.Contains(line.Paths, "options.bogusOption") {
		t.Fatalf("paths = %v", line.Paths)
	}
}
