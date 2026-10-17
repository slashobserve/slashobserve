package spec_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type schemaNode struct {
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
}

type schemaDoc struct {
	schemaNode
	Defs map[string]schemaNode `json:"$defs"`
}

func loadSchema(t *testing.T) schemaDoc {
	t.Helper()
	var doc schemaDoc
	if err := json.Unmarshal(spec.Schema(), &doc); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return doc
}

type jsonField struct {
	name      string
	omitEmpty bool
}

func jsonFields(t reflect.Type) []jsonField {
	var out []jsonField
	for i := range t.NumField() {
		name, opts, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		out = append(out, jsonField{name: name, omitEmpty: slices.Contains(strings.Split(opts, ","), "omitempty")})
	}
	return out
}

func TestTypesMatchSchema(t *testing.T) {
	doc := loadSchema(t)
	cases := []struct {
		name  string
		typ   reflect.Type
		nodes []schemaNode
	}{
		{"Dashboard", reflect.TypeFor[spec.Dashboard](), []schemaNode{doc.schemaNode}},
		{"TimeRange", reflect.TypeFor[spec.TimeRange](), []schemaNode{doc.Defs["timeRange"]}},
		{"Panel", reflect.TypeFor[spec.Panel](), []schemaNode{doc.Defs["panel"]}},
		{"GridPos", reflect.TypeFor[spec.GridPos](), []schemaNode{doc.Defs["gridPos"]}},
		{"DatasourceRef", reflect.TypeFor[spec.DatasourceRef](), []schemaNode{doc.Defs["datasourceRef"]}},
		{"Threshold", reflect.TypeFor[spec.Threshold](), []schemaNode{doc.Defs["threshold"]}},
		{"Variable", reflect.TypeFor[spec.Variable](), []schemaNode{doc.Defs["queryVariable"], doc.Defs["customVariable"]}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			props := map[string]bool{}
			required := map[string]bool{}
			for _, n := range tc.nodes {
				for p := range n.Properties {
					props[p] = true
				}
			}
			for p := range props {
				required[p] = true
				for _, n := range tc.nodes {
					if !slices.Contains(n.Required, p) {
						required[p] = false
					}
				}
			}
			fields := jsonFields(tc.typ)
			seen := map[string]bool{}
			for _, f := range fields {
				seen[f.name] = true
				if !props[f.name] {
					t.Errorf("json field %q is not a schema property", f.name)
					continue
				}
				if f.omitEmpty == required[f.name] {
					t.Errorf("field %q: omitempty=%v but required=%v", f.name, f.omitEmpty, required[f.name])
				}
			}
			for p := range props {
				if !seen[p] {
					t.Errorf("schema property %q has no field", p)
				}
			}
		})
	}
}

func TestUnits(t *testing.T) {
	units := spec.Units()
	if !slices.Contains(units, "reqps") {
		t.Fatalf("Units() does not contain reqps: %v", units)
	}
	units[0] = "mutated"
	if spec.Units()[0] == "mutated" {
		t.Fatal("Units() returns shared backing storage")
	}
}
