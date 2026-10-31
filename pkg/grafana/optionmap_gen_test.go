package grafana

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

const maxFieldsPerOption = 4

var (
	panelPrefix     = regexp.MustCompile(`^/panels/[0-9]+`)
	queryPrefix     = regexp.MustCompile(`^/panels/[0-9]+/queries/[0-9]+`)
	transformPrefix = regexp.MustCompile(`^/panels/[0-9]+/transform/[0-9]+`)
	variablePrefix  = regexp.MustCompile(`^/variables/[0-9]+`)
)

func decodeSpecJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func flattenSpec(pointer string, v any, out map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) > 0 {
			for key, child := range x {
				escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
				flattenSpec(pointer+"/"+escaped, child, out)
			}
			return
		}
	case []any:
		if len(x) > 0 {
			for i, child := range x {
				flattenSpec(fmt.Sprintf("%s/%d", pointer, i), child, out)
			}
			return
		}
	}
	leaf, _ := json.Marshal(v)
	out[pointer] = string(leaf)
}

func specDiff(before, after []byte) []string {
	flat := func(raw []byte) map[string]string {
		doc, err := decodeSpecJSON(raw)
		if err != nil {
			return nil
		}
		out := map[string]string{}
		flattenSpec("", doc, out)
		return out
	}
	was, now := flat(before), flat(after)
	var changed []string
	for pointer, leaf := range now {
		if old, ok := was[pointer]; !ok || old != leaf {
			changed = append(changed, pointer)
		}
	}
	for pointer := range was {
		if _, ok := now[pointer]; !ok {
			changed = append(changed, pointer)
		}
	}
	sort.Strings(changed)
	return changed
}

func scopeSpecPrefix(kind string) *regexp.Regexp {
	switch kind {
	case "panel":
		return panelPrefix
	case "query":
		return queryPrefix
	case "transform":
		return transformPrefix
	case "variable":
		return variablePrefix
	}
	return nil
}

func dottedFromPointer(pointer string) string {
	var out strings.Builder
	for _, segment := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		if segment == "" {
			continue
		}
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		if strings.Trim(segment, "0123456789") == "" {
			out.WriteString("[]")
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('.')
		}
		out.WriteString(segment)
	}
	return out.String()
}

func commonDottedPrefix(paths []string) string {
	prefix := strings.Split(paths[0], ".")
	for _, p := range paths[1:] {
		segments := strings.Split(p, ".")
		n := 0
		for n < len(prefix) && n < len(segments) && prefix[n] == segments[n] {
			n++
		}
		prefix = prefix[:n]
	}
	return strings.Join(prefix, ".")
}

func fieldPaths(pointers []string, kind string) []string {
	prefix := scopeSpecPrefix(kind)
	var paths []string
	for _, pointer := range pointers {
		rest, inside := pointer, true
		if prefix != nil {
			loc := prefix.FindStringIndex(pointer)
			inside = loc != nil
			if inside {
				rest = pointer[loc[1]:]
			}
		}
		path := dottedFromPointer(rest)
		if !inside {
			path = "dashboard." + path
		}
		if path != "" {
			paths = append(paths, path)
		}
	}
	paths = unionSorted(paths)
	if len(paths) <= maxFieldsPerOption {
		return paths
	}
	if common := commonDottedPrefix(paths); common != "" {
		return []string{common}
	}
	return paths[:maxFieldsPerOption]
}

type optionMapBuilder optionMap

func (b optionMapBuilder) record(scope optionScope, rel, key string, fields []string) {
	if b[scope] == nil {
		b[scope] = map[string]optionFields{}
	}
	entry := b[scope][rel]
	entry.Fields = unionSorted(entry.Fields, fields)
	if key != "" {
		if entry.Values == nil {
			entry.Values = map[string][]string{}
		}
		entry.Values[key] = unionSorted(entry.Values[key], fields)
	}
	b[scope][rel] = entry
}

func generateOptionMap(t *testing.T, areas []*conformanceArea) optionMap {
	t.Helper()
	m := optionMap{}
	b := optionMapBuilder(m)
	for _, a := range areas {
		for _, e := range a.catalog.Entries {
			if !e.converts() {
				continue
			}
			baselineDoc, variantDoc, err := a.documents(e)
			if err != nil {
				t.Fatalf("%s/%s: %v", a.name, e.ID, err)
			}
			baseline, variant := observeConversion(baselineDoc), observeConversion(variantDoc)
			if baseline.spec == nil || variant.spec == nil {
				continue
			}
			src, err := load(variantDoc)
			if err != nil {
				t.Fatalf("%s/%s: %v", a.name, e.ID, err)
			}
			if err := newRegistry(src).resolve(conformanceOptions); err != nil {
				t.Fatalf("%s/%s: %v", a.name, e.ID, err)
			}
			p := src.scopeOf(joinPath(a.catalog.Root, e.Path))
			fields := fieldPaths(specDiff(baseline.spec, variant.spec), p.scope.kind())
			b.record(p.scope, normalizePath(p.rel), valueKey(e.Value), fields)
		}
	}
	return m
}

func updateRequested() bool {
	f := flag.Lookup("update")
	if f == nil {
		return false
	}
	return f.Value.String() == "true"
}

func TestOptionMapIsCurrent(t *testing.T) {
	areas := loadConformanceAreas(t, conformanceRoot(t))
	generated, err := json.MarshalIndent(generateOptionMap(t, areas), "", " ")
	if err != nil {
		t.Fatal(err)
	}
	generated = append(generated, '\n')
	if updateRequested() {
		if err := os.WriteFile("optionmap.json", generated, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if !bytes.Equal(generated, optionMapJSON) {
		t.Fatal("optionmap.json is stale; run make golden")
	}
}

func TestSpecDiff(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		want          []string
	}{
		{"identical documents differ nowhere", `{"a":1,"b":[1,2]}`, `{"a":1,"b":[1,2]}`, nil},
		{"a changed leaf", `{"a":1}`, `{"a":2}`, []string{"/a"}},
		{"a key added", `{"a":1}`, `{"a":1,"b":true}`, []string{"/b"}},
		{"a key removed", `{"a":1,"b":true}`, `{"a":1}`, []string{"/b"}},
		{"a nested change names its leaf", `{"p":[{"q":{"r":1}}]}`, `{"p":[{"q":{"r":2}}]}`, []string{"/p/0/q/r"}},
		{"an array element added", `{"l":[1]}`, `{"l":[1,2]}`, []string{"/l/1"}},
		{"an empty object is a leaf", `{"a":{}}`, `{"a":{"b":1}}`, []string{"/a", "/a/b"}},
		{"an empty array is a leaf", `{"a":[]}`, `{"a":[0]}`, []string{"/a", "/a/0"}},
		{"keys with slash and tilde are escaped", `{"a/b":1,"c~d":1}`, `{"a/b":2,"c~d":2}`, []string{"/a~1b", "/c~0d"}},
		{"results are sorted", `{"z":1,"a":1}`, `{"z":2,"a":2}`, []string{"/a", "/z"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := specDiff([]byte(tc.before), []byte(tc.after)); !slices.Equal(got, tc.want) {
				t.Fatalf("specDiff = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFieldPaths(t *testing.T) {
	cases := []struct {
		name     string
		pointers []string
		kind     string
		want     []string
	}{
		{"panel strips its object", []string{"/panels/2/lineWidth"}, "panel", []string{"lineWidth"}},
		{"array indexes become brackets", []string{"/panels/0/overrides/1/match/value", "/panels/0/overrides/1/lineWidth"}, "panel", []string{"overrides[].lineWidth", "overrides[].match.value"}},
		{"query strips panel and query", []string{"/panels/3/queries/1/legend"}, "query", []string{"legend"}},
		{"transform strips panel and transform", []string{"/panels/0/transform/4/by/0"}, "transform", []string{"by[]"}},
		{"variable strips its object", []string{"/variables/1/default"}, "variable", []string{"default"}},
		{"dashboard strips nothing", []string{"/time/from", "/panels/0/title"}, "dashboard", []string{"panels[].title", "time.from"}},
		{"a pointer outside the scope is the dashboard's", []string{"/title", "/panels/0/title"}, "variable", []string{"dashboard.panels[].title", "dashboard.title"}},
		{"a query pointer outside its query is the dashboard's", []string{"/panels/0/title"}, "query", []string{"dashboard.panels[].title"}},
		{"escapes are undone", []string{"/panels/0/a~1b/c~0d"}, "panel", []string{"a/b.c~d"}},
		{"duplicates collapse", []string{"/panels/0/queries/0", "/panels/0/queries/1"}, "dashboard", []string{"panels[].queries[]"}},
		{"four fields are kept", []string{"/panels/0/a", "/panels/0/b", "/panels/0/c", "/panels/0/d"}, "panel", []string{"a", "b", "c", "d"}},
		{"many fields collapse to their common prefix", []string{"/panels/0/display/a", "/panels/0/display/b", "/panels/0/display/c", "/panels/0/display/d", "/panels/0/display/e"}, "panel", []string{"display"}},
		{"common prefix is whole segments", []string{"/panels/0/displayA/x", "/panels/0/displayB/x", "/panels/0/displayC/x", "/panels/0/displayD/x", "/panels/0/displayE/x"}, "panel", []string{"displayA.x", "displayB.x", "displayC.x", "displayD.x"}},
		{"nothing in common keeps the first four", []string{"/panels/0/e", "/panels/0/d", "/panels/0/c", "/panels/0/b", "/panels/0/a"}, "panel", []string{"a", "b", "c", "d"}},
		{"the scope object itself names no field", []string{"/panels/0"}, "panel", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldPaths(tc.pointers, tc.kind); !slices.Equal(got, tc.want) {
				t.Fatalf("fieldPaths = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValueKey(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"string", "bytes", "s:bytes"},
		{"empty string", "", "s:"},
		{"integer number", json.Number("500"), "n:500"},
		{"fractional number", json.Number("0.25"), "n:0.25"},
		{"float64", 1.5, "n:1.5"},
		{"number with exponent", json.Number("1e3"), "n:1000"},
		{"true", true, "b:true"},
		{"false", false, "b:false"},
		{"object with id", map[string]any{"id": "unit", "value": "bytes"}, "id:unit"},
		{"id wins over type", map[string]any{"id": "x", "type": "y"}, "id:x"},
		{"object with type", map[string]any{"type": "range"}, "type:range"},
		{"object with a non-string id falls to type", map[string]any{"id": 3, "type": "range"}, "type:range"},
		{"plain object", map[string]any{"a": 1}, ""},
		{"list", []any{"a"}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := valueKey(tc.value); got != tc.want {
				t.Fatalf("valueKey = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOptionMapBuilderUnionsSorted(t *testing.T) {
	m := optionMap{}
	b := optionMapBuilder(m)
	b.record("panel:stat", "options.x", "s:a", []string{"z", "b"})
	b.record("panel:stat", "options.x", "s:b", []string{"b", "c"})
	b.record("panel:stat", "options.x", "", []string{"d"})
	want := optionFields{
		Fields: []string{"b", "c", "d", "z"},
		Values: map[string][]string{"s:a": {"b", "z"}, "s:b": {"b", "c"}},
	}
	got := m["panel:stat"]["options.x"]
	if !slices.Equal(got.Fields, want.Fields) || fmt.Sprint(got.Values) != fmt.Sprint(want.Values) {
		t.Fatalf("entry = %+v, want %+v", got, want)
	}
}
