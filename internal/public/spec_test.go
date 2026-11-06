package public

import (
	"encoding/json"
	"io/fs"
	"os"
	"reflect"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

var examples = os.DirFS("../../spec/examples")

func fixtureSpec(t *testing.T, name string) *spec.Dashboard {
	t.Helper()
	raw, err := fs.ReadFile(examples, name)
	if err != nil {
		t.Fatal(err)
	}
	d, err := spec.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return d
}

func asJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func objects(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func queryHolders(doc map[string]any) []map[string]any {
	holders := objects(doc["variables"])
	for _, panel := range objects(doc["panels"]) {
		holders = append(holders, objects(panel["queries"])...)
		for _, nested := range objects(panel["panels"]) {
			holders = append(holders, objects(nested["queries"])...)
		}
	}
	return holders
}

func TestPublicSpecDropsOnlyQueryText(t *testing.T) {
	fixtures, err := fs.Glob(examples, "*.json")
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			original := fixtureSpec(t, name)
			before := asJSON(t, original)
			shown, err := publicSpec(original)
			if err != nil {
				t.Fatal(err)
			}
			after := asJSON(t, shown)
			for _, holder := range queryHolders(after) {
				if _, ok := holder["query"]; ok {
					t.Fatalf("query text survived: %v", holder)
				}
			}
			for _, holder := range queryHolders(before) {
				delete(holder, "query")
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("public spec changed more than query text")
			}
		})
	}
}

func TestPublicSpecLeavesTheOriginalAlone(t *testing.T) {
	original := fixtureSpec(t, "checkout-service-health.json")
	want := original.Panels[0].Queries[0].Query
	if want == "" {
		t.Fatal("fixture panel has no query text")
	}
	if _, err := publicSpec(original); err != nil {
		t.Fatal(err)
	}
	if original.Panels[0].Queries[0].Query != want {
		t.Fatalf("publicSpec emptied the original's query")
	}
}

func TestReferencedDatasources(t *testing.T) {
	d := &spec.Dashboard{
		Variables: []spec.Variable{
			{Name: "ds", Type: spec.DatasourceVariable, DatasourceType: spec.ClickHouse},
			{Name: "job", Type: spec.QueryVariable, Datasource: "prom"},
		},
		Panels: []spec.Panel{
			{ID: "a", Datasource: "${ds}", Queries: []spec.Query{{Ref: "A"}}},
			{ID: "row", Panels: []spec.Panel{{ID: "b", Queries: []spec.Query{{Ref: "A", Datasource: "loki"}}}}},
			{ID: "c", Datasource: "missing"},
		},
	}
	types := map[string]string{"prom": "prometheus", "prom2": "prometheus", "loki": "loki", "ch1": "clickhouse", "ch2": "clickhouse", "tempo": "tempo"}
	got := referencedDatasources(d, types)
	want := []Datasource{{"ch1", "clickhouse"}, {"ch2", "clickhouse"}, {"loki", "loki"}, {"prom", "prometheus"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
