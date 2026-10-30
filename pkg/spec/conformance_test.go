package spec_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type schemaNode struct {
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
	OneOf      []schemaNode               `json:"oneOf"`
	AllOf      []json.RawMessage          `json:"allOf"`
	Items      *schemaNode                `json:"items"`
	Ref        string                     `json:"$ref"`
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
		f := t.Field(i)
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			out = append(out, jsonFields(f.Type)...)
			continue
		}
		out = append(out, jsonField{name: name, omitEmpty: slices.Contains(strings.Split(opts, ","), "omitempty")})
	}
	return out
}

func propertyNode(t *testing.T, node schemaNode, name string) schemaNode {
	t.Helper()
	raw, ok := node.Properties[name]
	if !ok {
		t.Fatalf("schema has no property %q", name)
	}
	var out schemaNode
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode property %q: %v", name, err)
	}
	return out
}

func mergeProps(node schemaNode, extra schemaNode) schemaNode {
	props := map[string]json.RawMessage{}
	maps.Copy(props, node.Properties)
	maps.Copy(props, extra.Properties)
	return schemaNode{Required: node.Required, Properties: props}
}

func displayNode(t *testing.T, doc schemaDoc) schemaNode {
	t.Helper()
	props := map[string]json.RawMessage{}
	for _, raw := range doc.Defs["dataPanel"].AllOf {
		var branch struct {
			Then struct {
				Properties struct {
					Display schemaNode `json:"display"`
				} `json:"properties"`
			} `json:"then"`
		}
		if err := json.Unmarshal(raw, &branch); err != nil {
			t.Fatalf("decode dataPanel allOf: %v", err)
		}
		maps.Copy(props, branch.Then.Properties.Display.Properties)
	}
	var dashlistDisplay schemaNode
	if err := json.Unmarshal(doc.Defs["dashlistPanel"].Properties["display"], &dashlistDisplay); err != nil {
		t.Fatalf("decode dashlistPanel display: %v", err)
	}
	maps.Copy(props, dashlistDisplay.Properties)
	var newsDisplay schemaNode
	if err := json.Unmarshal(doc.Defs["newsPanel"].Properties["display"], &newsDisplay); err != nil {
		t.Fatalf("decode newsPanel display: %v", err)
	}
	maps.Copy(props, newsDisplay.Properties)
	if len(props) == 0 {
		t.Fatal("dataPanel has no display properties")
	}
	return schemaNode{Properties: props}
}

func TestTypesMatchSchema(t *testing.T) {
	doc := loadSchema(t)
	d := doc.Defs
	fieldProps := d["fieldProps"]
	variables := []schemaNode{d["queryVariable"], d["customVariable"], d["constantVariable"], d["intervalVariable"], d["textboxVariable"], d["datasourceVariable"], d["adhocVariable"]}
	panels := []schemaNode{mergeProps(d["dataPanel"], fieldProps), d["textPanel"], d["dashlistPanel"], d["newsPanel"], d["placeholderPanel"], d["rowPanel"]}
	cases := []struct {
		name  string
		typ   reflect.Type
		nodes []schemaNode
	}{
		{"Dashboard", reflect.TypeFor[spec.Dashboard](), []schemaNode{doc.schemaNode}},
		{"TimeRange", reflect.TypeFor[spec.TimeRange](), []schemaNode{d["timeRange"]}},
		{"GridPos", reflect.TypeFor[spec.GridPos](), []schemaNode{d["gridPos"]}},
		{"Threshold", reflect.TypeFor[spec.Threshold](), []schemaNode{d["threshold"]}},
		{"Link", reflect.TypeFor[spec.Link](), []schemaNode{d["link"]}},
		{"Query", reflect.TypeFor[spec.Query](), []schemaNode{d["panelQuery"]}},
		{"FieldProps", reflect.TypeFor[spec.FieldProps](), []schemaNode{fieldProps}},
		{"Variable", reflect.TypeFor[spec.Variable](), variables},
		{"Panel", reflect.TypeFor[spec.Panel](), panels},
		{"Override", reflect.TypeFor[spec.Override](), []schemaNode{mergeProps(d["override"], fieldProps)}},
		{"Mapping", reflect.TypeFor[spec.Mapping](), d["mapping"].OneOf},
		{"Match", reflect.TypeFor[spec.Match](), d["match"].OneOf},
		{"Transform", reflect.TypeFor[spec.Transform](), d["transform"].OneOf},
		{"CanvasElement", reflect.TypeFor[spec.CanvasElement](), []schemaNode{d["canvasElement"]}},
		{"CanvasConnection", reflect.TypeFor[spec.CanvasConnection](), []schemaNode{d["canvasConnection"]}},
		{"Display", reflect.TypeFor[spec.Display](), []schemaNode{displayNode(t, doc)}},
		{"TextCode", reflect.TypeFor[spec.TextCode](), []schemaNode{propertyNode(t, d["textPanel"], "code")}},
		{"CanvasPlacement", reflect.TypeFor[spec.CanvasPlacement](), []schemaNode{propertyNode(t, d["canvasElement"], "placement")}},
		{"CanvasVertex", reflect.TypeFor[spec.CanvasVertex](), []schemaNode{*propertyNode(t, d["canvasConnection"], "vertices").Items}},
		{"ImageRef", reflect.TypeFor[spec.ImageRef](), d["imageRef"].OneOf},
		{"Action", reflect.TypeFor[spec.Action](), []schemaNode{d["action"]}},
		{"ActionParam", reflect.TypeFor[spec.ActionParam](), []schemaNode{d["actionParam"]}},
		{"GeomapLayer", reflect.TypeFor[spec.GeomapLayer](), []schemaNode{d["geomapLayer"]}},
		{"GeomapRule", reflect.TypeFor[spec.GeomapRule](), []schemaNode{d["geomapRule"]}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.nodes) == 0 {
				t.Fatal("no schema nodes")
			}
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
	if !slices.Contains(units, "reqps") || !slices.Contains(units, "µs") {
		t.Fatalf("Units() does not contain reqps and µs: %v", units)
	}
	if len(units) != 112 {
		t.Fatalf("Units() has %d entries, want 112", len(units))
	}
	units[0] = "mutated"
	if spec.Units()[0] == "mutated" {
		t.Fatal("Units() returns shared backing storage")
	}
}
