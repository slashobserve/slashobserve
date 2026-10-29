package spec_test

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
	"github.com/slashobserve/slashobserve/spec/schema"
)

func rawDefaults(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(schema.Defaults, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func rawObject(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func schemaDisplayPanelTypes(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Defs struct {
			DashlistPanel struct {
				Properties struct {
					Type struct {
						Const string `json:"const"`
					} `json:"type"`
				} `json:"properties"`
			} `json:"dashlistPanel"`
			NewsPanel struct {
				Properties struct {
					Type struct {
						Const string `json:"const"`
					} `json:"type"`
				} `json:"properties"`
			} `json:"newsPanel"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(spec.Schema(), &doc); err != nil {
		t.Fatal(err)
	}
	return append(schemaDataPanelTypes(t), doc.Defs.DashlistPanel.Properties.Type.Const, doc.Defs.NewsPanel.Properties.Type.Const)
}

func schemaDataPanelTypes(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Defs struct {
			DataPanel struct {
				Properties struct {
					Type struct {
						Enum []string `json:"enum"`
					} `json:"type"`
				} `json:"properties"`
			} `json:"dataPanel"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(spec.Schema(), &doc); err != nil {
		t.Fatal(err)
	}
	types := doc.Defs.DataPanel.Properties.Type.Enum
	if len(types) == 0 {
		t.Fatal("the schema lists no data panel types")
	}
	return types
}

func parseDefaultsDoc(t *testing.T, doc map[string]any) {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Parse(data); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
}

func defaultsDoc(panels ...map[string]any) map[string]any {
	items := make([]any, len(panels))
	for i, p := range panels {
		items[i] = p
	}
	return map[string]any{
		"specVersion": "0",
		"title":       "Defaults",
		"time":        map[string]any{"from": "now-15m", "to": "now"},
		"panels":      items,
	}
}

func defaultsPanel(id, panelType string) map[string]any {
	return map[string]any{
		"id":         id,
		"type":       panelType,
		"title":      id,
		"gridPos":    map[string]any{"x": 0, "y": 0, "w": 12, "h": 8},
		"datasource": "prom",
		"queries":    []any{map[string]any{"query": "up"}},
	}
}

func TestDefaultsTableDecodes(t *testing.T) {
	table := spec.Defaults()
	raw := rawDefaults(t)
	var display map[string]json.RawMessage
	if err := json.Unmarshal(raw["display"], &display); err != nil {
		t.Fatal(err)
	}
	if len(table.Display) != len(display) {
		t.Fatalf("Defaults() has %d display entries, the file has %d", len(table.Display), len(display))
	}
	if table.FieldProps.ThresholdMode == "" || table.Query.Format == "" || table.Variable.Refresh == "" || table.Dashboard.CursorSync == "" {
		t.Fatalf("Defaults() left a scalar section empty: %+v", table)
	}
}

func TestDefaultsDisplayKeysAreSchemaPanelTypes(t *testing.T) {
	types := schemaDisplayPanelTypes(t)
	for panelType := range spec.Defaults().Display {
		if !slices.Contains(types, string(panelType)) {
			t.Errorf("display default for %q, which is not a schema data panel type %v", panelType, types)
		}
	}
	var display map[string]json.RawMessage
	if err := json.Unmarshal(rawDefaults(t)["display"], &display); err != nil {
		t.Fatal(err)
	}
	for _, key := range slices.Sorted(maps.Keys(display)) {
		if !slices.Contains(types, key) {
			t.Errorf("defaults.json display key %q is not a schema data panel type", key)
		}
	}
}

func TestInstantPanelsAreSchemaDataPanelTypes(t *testing.T) {
	types := schemaDataPanelTypes(t)
	table := spec.Defaults()
	if len(table.InstantPanels) == 0 {
		t.Fatal("defaults list no instant panels")
	}
	for _, panelType := range table.InstantPanels {
		if !slices.Contains(types, string(panelType)) {
			t.Errorf("instant panel %q is not a schema data panel type", panelType)
		}
	}
}

func TestInstantByDefault(t *testing.T) {
	instant := []spec.PanelType{spec.Table, spec.Logs, spec.NodeGraph, spec.Traces, spec.FlameGraph, spec.Geomap, spec.AlertList}
	for _, panelType := range instant {
		if !spec.InstantByDefault(panelType) {
			t.Errorf("InstantByDefault(%q) = false, want true", panelType)
		}
	}
	for _, panelType := range []spec.PanelType{spec.Timeseries, spec.BarChart, spec.Canvas, spec.Text, spec.DashList, spec.News} {
		if spec.InstantByDefault(panelType) {
			t.Errorf("InstantByDefault(%q) = true, want false", panelType)
		}
	}
}

func TestDefaultsInstantPanelsAreCloned(t *testing.T) {
	first := spec.Defaults()
	first.InstantPanels[0] = "mutated"
	if spec.Defaults().InstantPanels[0] == "mutated" || spec.InstantByDefault("mutated") {
		t.Fatal("Defaults() shares the instant panel list between calls")
	}
}

func TestDefaultValuesAreSchemaValid(t *testing.T) {
	raw := rawDefaults(t)
	var display map[string]json.RawMessage
	if err := json.Unmarshal(raw["display"], &display); err != nil {
		t.Fatal(err)
	}

	for _, panelType := range slices.Sorted(maps.Keys(display)) {
		t.Run("display "+panelType, func(t *testing.T) {
			panel := defaultsPanel("p1", panelType)
			if panelType == "dashlist" || panelType == "news" {
				delete(panel, "datasource")
				delete(panel, "queries")
			}
			panel["display"] = rawObject(t, display[panelType])
			parseDefaultsDoc(t, defaultsDoc(panel))
		})
	}

	t.Run("canvas elements", func(t *testing.T) {
		element := map[string]any{"id": "a", "kind": "metric", "left": 0, "top": 0, "width": 10, "height": 10}
		maps.Copy(element, rawObject(t, raw["canvasElement"]))
		connection := map[string]any{"from": "a", "to": "a"}
		maps.Copy(connection, rawObject(t, raw["canvasConnection"]))
		panel := defaultsPanel("p1", "canvas")
		panel["display"] = map[string]any{"elements": []any{element}, "connections": []any{connection}}
		parseDefaultsDoc(t, defaultsDoc(panel))
	})

	t.Run("fieldProps", func(t *testing.T) {
		panel := defaultsPanel("p1", "timeseries")
		maps.Copy(panel, rawObject(t, raw["fieldProps"]))
		parseDefaultsDoc(t, defaultsDoc(panel))
	})

	t.Run("query", func(t *testing.T) {
		panel := defaultsPanel("p1", "timeseries")
		panel["queries"] = []any{rawQueryWithDefaults(t, raw)}
		parseDefaultsDoc(t, defaultsDoc(panel))
	})

	t.Run("variable", func(t *testing.T) {
		variable := map[string]any{"name": "job", "type": "query", "datasource": "prom", "query": "label_values(up, job)"}
		maps.Copy(variable, rawObject(t, raw["variable"]))
		doc := defaultsDoc(defaultsPanel("p1", "timeseries"))
		doc["variables"] = []any{variable}
		parseDefaultsDoc(t, doc)
	})

	t.Run("row", func(t *testing.T) {
		row := map[string]any{"id": "r1", "type": "row", "title": "Row", "gridPos": map[string]any{"x": 0, "y": 0, "w": 24, "h": 1}}
		maps.Copy(row, rawObject(t, raw["row"]))
		row["panels"] = []any{}
		parseDefaultsDoc(t, defaultsDoc(row))
	})

	t.Run("link", func(t *testing.T) {
		link := map[string]any{"title": "Docs", "url": "https://example.com"}
		maps.Copy(link, rawObject(t, raw["link"]))
		doc := defaultsDoc(defaultsPanel("p1", "timeseries"))
		doc["links"] = []any{link}
		parseDefaultsDoc(t, doc)
	})
}

func rawQueryWithDefaults(t *testing.T, raw map[string]json.RawMessage) map[string]any {
	t.Helper()
	query := map[string]any{"query": "up"}
	maps.Copy(query, rawObject(t, raw["query"]))
	return query
}

func TestNewsPanelFeedURL(t *testing.T) {
	cases := []struct {
		name    string
		display map[string]any
		valid   bool
	}{
		{"https feed", map[string]any{"feedUrl": "https://grafana.com/blog/news.xml"}, true},
		{"no display options", map[string]any{}, true},
		{"http feed", map[string]any{"feedUrl": "http://grafana.com/blog/news.xml"}, false},
		{"scheme only", map[string]any{"feedUrl": "https://"}, false},
		{"whitespace in url", map[string]any{"feedUrl": "https://example.com/a b"}, false},
		{"relative url", map[string]any{"feedUrl": "/blog/news.xml"}, false},
		{"unknown option", map[string]any{"feedUrl": "https://example.com/feed", "limit": 5}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			panel := defaultsPanel("p1", "news")
			delete(panel, "datasource")
			delete(panel, "queries")
			panel["display"] = c.display
			data, err := json.Marshal(defaultsDoc(panel))
			if err != nil {
				t.Fatal(err)
			}
			_, err = spec.Parse(data)
			if c.valid && err != nil {
				t.Fatalf("Parse() = %v, want valid", err)
			}
			if !c.valid && err == nil {
				t.Fatal("Parse() accepted an invalid news panel")
			}
		})
	}
}
