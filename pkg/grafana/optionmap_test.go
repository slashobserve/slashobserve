package grafana

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSpecFields(t *testing.T) {
	m := optionMap{
		"panel:stat": {
			"options.reduceOptions.calcs": {Fields: []string{"reducer"}},
			"options.colorMode":           {Fields: []string{"colorMode", "color"}, Values: map[string][]string{"s:background": {"colorMode"}}},
			"fieldConfig.overrides[].properties[]": {
				Fields: []string{"overrides[].unit", "overrides[].min"},
				Values: map[string][]string{"id:unit": {"overrides[].unit"}, "id:min": {"overrides[].min"}},
			},
		},
		"panel:*": {
			"options.reduceOptions.calcs": {Fields: []string{"reducer", "stats"}},
			"title":                       {Fields: []string{"title"}},
		},
	}
	at := func(values map[string]any) func(string) (any, bool) {
		return func(rel string) (any, bool) {
			v, ok := values[rel]
			return v, ok
		}
	}
	cases := []struct {
		name       string
		scope      optionScope
		rel        string
		values     map[string]any
		wantFields []string
		wantOption string
		wantFound  bool
	}{
		{"an exact hit", "panel:stat", "options.reduceOptions.calcs", nil, []string{"reducer"}, "options.reduceOptions.calcs", true},
		{"an indexed path hits the normalized key", "panel:stat", "fieldConfig.overrides[2].properties[0]", map[string]any{"fieldConfig.overrides[2].properties[0]": map[string]any{"id": "min"}}, []string{"overrides[].min"}, "fieldConfig.overrides[2].properties[0]", true},
		{"an ancestor hit names the ancestor as the option", "panel:stat", "options.reduceOptions.calcs[1]", nil, []string{"reducer"}, "options.reduceOptions.calcs", true},
		{"a nested path under an override property hits the property", "panel:stat", "fieldConfig.overrides[0].properties[1].value", map[string]any{"fieldConfig.overrides[0].properties[1]": map[string]any{"id": "unit"}}, []string{"overrides[].unit"}, "fieldConfig.overrides[0].properties[1]", true},
		{"the wildcard scope answers when the exact scope has no entry", "panel:stat", "title", nil, []string{"title"}, "title", true},
		{"an unknown scope falls back to the wildcard", "panel:gauge", "options.reduceOptions.calcs", nil, []string{"reducer", "stats"}, "options.reduceOptions.calcs", true},
		{"the exact scope wins over the wildcard", "panel:stat", "options.reduceOptions.calcs", nil, []string{"reducer"}, "options.reduceOptions.calcs", true},
		{"a value refines the fields", "panel:stat", "options.colorMode", map[string]any{"options.colorMode": "background"}, []string{"colorMode"}, "options.colorMode", true},
		{"a value without a refinement keeps the option's fields", "panel:stat", "options.colorMode", map[string]any{"options.colorMode": "value"}, []string{"colorMode", "color"}, "options.colorMode", true},
		{"a discriminated option with an unknown id has no fields but is found", "panel:stat", "fieldConfig.overrides[0].properties[0]", map[string]any{"fieldConfig.overrides[0].properties[0]": map[string]any{"id": "custom.unheard"}}, nil, "fieldConfig.overrides[0].properties[0]", true},
		{"a discriminated option with no value has no fields but is found", "panel:stat", "fieldConfig.overrides[0].properties[0]", nil, nil, "fieldConfig.overrides[0].properties[0]", true},
		{"a path with no entry anywhere is not found", "panel:stat", "options.unknown.deep", nil, nil, "", false},
		{"an empty relative path is not found", "panel:stat", "", nil, nil, "", false},
		{"a scope with no entries and no wildcard match is not found", "variable:custom", "query", nil, nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields, option, found := m.specFields(scopedPath{scope: tc.scope, rel: tc.rel}, at(tc.values))
			if !slices.Equal(fields, tc.wantFields) || option != tc.wantOption || found != tc.wantFound {
				t.Fatalf("specFields = %v, %q, %v; want %v, %q, %v", fields, option, found, tc.wantFields, tc.wantOption, tc.wantFound)
			}
		})
	}
}

func TestOptionMapWildcards(t *testing.T) {
	m := optionMap{
		"panel:stat":  {"options.x": {Fields: []string{"b"}, Values: map[string][]string{"s:a": {"b"}}}},
		"panel:gauge": {"options.x": {Fields: []string{"c"}, Values: map[string][]string{"s:a": {"c"}, "s:z": {"d"}}}},
		"dashboard":   {"title": {Fields: []string{"title"}}},
	}
	wild := m.withWildcards()
	got := wild["panel:*"]["options.x"]
	if want := []string{"b", "c", "d"}; !slices.Equal(got.Fields, want) {
		t.Fatalf("wildcard fields = %v, want %v", got.Fields, want)
	}
	if want := []string{"b", "c"}; !slices.Equal(got.Values["s:a"], want) {
		t.Fatalf("wildcard s:a = %v, want %v", got.Values["s:a"], want)
	}
	if _, ok := wild["dashboard:*"]; ok {
		t.Fatal("the dashboard scope must not get a wildcard")
	}
	if len(m) != 3 {
		t.Fatal("withWildcards changed its receiver")
	}
}

func TestEmbeddedOptionMapAnswersForKnownOptions(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal(optionMapJSON, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("optionmap.json is empty; run make golden")
	}
	fields, option, found := loadedOptionMap().specFields(
		scopedPath{scope: "panel:timeseries", rel: "fieldConfig.defaults.custom.lineWidth"},
		func(string) (any, bool) { return json.Number("4"), true },
	)
	if !found || option != "fieldConfig.defaults.custom.lineWidth" || !slices.Equal(fields, []string{"lineWidth"}) {
		t.Fatalf("specFields = %v, %q, %v", fields, option, found)
	}
}
