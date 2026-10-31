package grafana

import (
	"encoding/json"
	"reflect"
	"testing"
)

func scopeTestSource(t *testing.T) *source {
	t.Helper()
	prom := map[string]any{"type": "prometheus", "uid": "prom"}
	altinity := map[string]any{"type": "vertamedia-clickhouse-datasource", "uid": "ch"}
	official := map[string]any{"type": "grafana-clickhouse-datasource", "uid": "ch2"}
	expression := map[string]any{"type": "__expr__", "uid": "__expr__"}
	dashboard := map[string]any{"type": "datasource", "uid": "-- Dashboard --"}
	loki := map[string]any{"type": "loki", "uid": "loki"}
	pos := func(y int) map[string]any { return map[string]any{"x": 0, "y": y, "w": 12, "h": 4} }
	panels := []any{
		map[string]any{
			"id": 1, "type": "timeseries", "title": "Latency", "gridPos": pos(0), "datasource": prom,
			"targets":         []any{map[string]any{"refId": "A", "expr": "up"}},
			"transformations": []any{map[string]any{"id": "calculateField", "options": map[string]any{"mode": "binary"}}},
		},
		map[string]any{
			"id": 2, "type": "row", "title": "More", "collapsed": true, "gridPos": pos(4),
			"panels": []any{map[string]any{
				"id": 3, "type": "stat", "title": "Errors", "gridPos": pos(5), "datasource": prom,
				"options": map[string]any{"reduceOptions": map[string]any{"calcs": []any{"mean"}}},
				"targets": []any{map[string]any{"refId": "A", "expr": "up"}},
			}},
		},
		map[string]any{
			"id": 4, "type": "timeseries", "title": "Mixed", "gridPos": pos(9),
			"targets": []any{
				map[string]any{"refId": "A", "datasource": altinity, "query": "select 1"},
				map[string]any{"refId": "B", "datasource": expression, "type": "math", "expression": "$A"},
				map[string]any{"refId": "C", "datasource": official, "rawSql": "select 1"},
				map[string]any{"refId": "D", "datasource": dashboard, "panelId": 1},
				map[string]any{"refId": "E", "datasource": loki, "expr": "{a=\"b\"}"},
			},
		},
	}
	for i := 3; i < 11; i++ {
		panels = append(panels, map[string]any{"id": 10 + i, "type": "text", "title": "Note", "gridPos": pos(13 + 4*i), "options": map[string]any{"content": "x"}})
	}
	doc := map[string]any{
		"title": "Scopes", "uid": "scopes", "schemaVersion": 39,
		"time":        map[string]any{"from": "now-6h", "to": "now"},
		"annotations": map[string]any{"list": []any{map[string]any{"name": "Deploys", "datasource": prom}}},
		"templating": map[string]any{"list": []any{
			map[string]any{"name": "pod", "type": "query", "datasource": prom, "query": "label_values(up, pod)", "refresh": 1},
			map[string]any{"name": "env", "type": "custom", "query": "a,b"},
		}},
		"panels": panels,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	src, err := load(raw)
	if err != nil {
		t.Fatal(err)
	}
	reg := newRegistry(src)
	opts := Options{Datasources: []Datasource{{UID: "prom", Type: "prometheus"}, {UID: "ch", Type: "clickhouse"}}}
	if err := reg.resolve(opts); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestScopeOf(t *testing.T) {
	src := scopeTestSource(t)
	cases := []struct {
		name string
		path string
		want scopedPath
	}{
		{"dashboard key", "time.from", scopedPath{scopeDashboard, "", "time.from"}},
		{"path into no node", "annotations.list[0]", scopedPath{scopeDashboard, "", "annotations.list[0]"}},
		{"variable itself", "templating.list[0]", scopedPath{"variable:query", "templating.list[0]", ""}},
		{"variable key", "templating.list[1].query", scopedPath{"variable:custom", "templating.list[1]", "query"}},
		{"panel itself", "panels[0]", scopedPath{"panel:timeseries", "panels[0]", ""}},
		{"panel option", "panels[0].fieldConfig.defaults.custom.lineWidth", scopedPath{"panel:timeseries", "panels[0]", "fieldConfig.defaults.custom.lineWidth"}},
		{"target key", "panels[0].targets[0].expr", scopedPath{"query:prometheus", "panels[0].targets[0]", "expr"}},
		{"target itself", "panels[0].targets[0]", scopedPath{"query:prometheus", "panels[0].targets[0]", ""}},
		{"transformation option", "panels[0].transformations[0].options.mode", scopedPath{"transform:calculateField", "panels[0].transformations[0]", "options.mode"}},
		{"transformation itself", "panels[0].transformations[0]", scopedPath{"transform:calculateField", "panels[0].transformations[0]", ""}},
		{"row itself", "panels[1]", scopedPath{"panel:row", "panels[1]", ""}},
		{"row option", "panels[1].collapsed", scopedPath{"panel:row", "panels[1]", "collapsed"}},
		{"row child option", "panels[1].panels[0].options.reduceOptions.calcs", scopedPath{"panel:stat", "panels[1].panels[0]", "options.reduceOptions.calcs"}},
		{"row child target", "panels[1].panels[0].targets[0].expr", scopedPath{"query:prometheus", "panels[1].panels[0].targets[0]", "expr"}},
		{"altinity target", "panels[2].targets[0].query", scopedPath{"query:altinity", "panels[2].targets[0]", "query"}},
		{"expression target", "panels[2].targets[1].expression", scopedPath{"query:expression", "panels[2].targets[1]", "expression"}},
		{"clickhouse target", "panels[2].targets[2].rawSql", scopedPath{"query:clickhouse", "panels[2].targets[2]", "rawSql"}},
		{"dashboard target", "panels[2].targets[3].panelId", scopedPath{"query:dashboard", "panels[2].targets[3]", "panelId"}},
		{"unsupported target", "panels[2].targets[4].expr", scopedPath{"query:unsupported", "panels[2].targets[4]", "expr"}},
		{"target past the list", "panels[0].targets[7].expr", scopedPath{"query:unsupported", "panels[0].targets[7]", "expr"}},
		{"panels[1] does not capture panels[10]", "panels[10].options.content", scopedPath{"panel:text", "panels[10]", "options.content"}},
		{"panels[10] is its own node", "panels[10]", scopedPath{"panel:text", "panels[10]", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := src.scopeOf(tc.path); got != tc.want {
				t.Fatalf("scopeOf(%q) = %+v, want %+v", tc.path, got, tc.want)
			}
		})
	}
}

func TestOptionScopeKindAndWildcard(t *testing.T) {
	cases := []struct {
		scope    optionScope
		kind     string
		wildcard optionScope
	}{
		{scopeDashboard, "dashboard", scopeDashboard},
		{variableScope("query"), "variable", "variable:*"},
		{panelScope("timeseries"), "panel", "panel:*"},
		{queryScope(queryAltinity), "query", "query:*"},
		{transformScope("calculateField"), "transform", "transform:*"},
	}
	for _, tc := range cases {
		t.Run(string(tc.scope), func(t *testing.T) {
			if got := tc.scope.kind(); got != tc.kind {
				t.Fatalf("kind() = %q, want %q", got, tc.kind)
			}
			if got := tc.scope.wildcard(); got != tc.wildcard {
				t.Fatalf("wildcard() = %q, want %q", got, tc.wildcard)
			}
		})
	}
}

func TestQueryKindOf(t *testing.T) {
	src := scopeTestSource(t)
	node := func(path string) *panelNode {
		var found *panelNode
		src.walk(func(n *panelNode) {
			if n.path == path {
				found = n
			}
		})
		if found == nil {
			t.Fatalf("no node at %s", path)
		}
		return found
	}
	mixed := node("panels[2]")
	targets, _ := asArray(mixed.raw["targets"])
	target := func(i int) map[string]any {
		obj, _ := asObject(targets[i])
		return obj
	}
	prometheus := node("panels[0]")
	sqlOnAltinity := map[string]any{"rawSql": "select 1"}
	queryOnOfficial := map[string]any{"query": "select 1"}
	bare := map[string]any{}

	cases := []struct {
		name   string
		ref    dsRef
		target map[string]any
		want   queryKind
	}{
		{"expression", mixed.ds.targets[1], target(1), queryExpression},
		{"dashboard", mixed.ds.targets[3], target(3), queryDashboard},
		{"unsupported datasource", mixed.ds.targets[4], target(4), queryUnsupported},
		{"missing datasource", dsRef{}, bare, queryUnsupported},
		{"prometheus", prometheus.ds.targets[0], bare, queryPrometheus},
		{"altinity by its query text", mixed.ds.targets[0], target(0), queryAltinity},
		{"altinity plugin with neither text", mixed.ds.targets[0], bare, queryAltinity},
		{"altinity plugin holding raw sql", mixed.ds.targets[0], sqlOnAltinity, queryClickHouse},
		{"official by its raw sql", mixed.ds.targets[2], target(2), queryClickHouse},
		{"official holding a query text", mixed.ds.targets[2], queryOnOfficial, queryAltinity},
		{"official with neither text", mixed.ds.targets[2], bare, queryClickHouse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := queryKindOf(tc.ref, tc.target); got != tc.want {
				t.Fatalf("queryKindOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValueAt(t *testing.T) {
	src := scopeTestSource(t)
	t.Run("root and rel empty", func(t *testing.T) {
		got, ok := src.valueAt("", "")
		if !ok || !reflect.DeepEqual(got, src.dash) {
			t.Fatalf("valueAt(\"\", \"\") = %v, %v, want the dashboard", got, ok)
		}
	})
	cases := []struct {
		name   string
		root   string
		rel    string
		want   any
		wantOK bool
	}{
		{"rel only", "", "time.from", "now-6h", true},
		{"root only", "panels[0].targets[0]", "", map[string]any{"refId": "A", "expr": "up"}, true},
		{"root and rel", "panels[1].panels[0]", "options.reduceOptions.calcs[0]", "mean", true},
		{"rel starting with an index", "panels[1].panels[0].options.reduceOptions.calcs", "[0]", "mean", true},
		{"missing", "panels[0]", "options.nothing", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := src.valueAt(tc.root, tc.rel)
			if ok != tc.wantOK || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("valueAt(%q, %q) = %v, %v, want %v, %v", tc.root, tc.rel, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
