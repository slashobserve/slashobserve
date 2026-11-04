package spec_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func statJSON(id string, x, y, w, h int) string {
	return `{"id":"` + id + `","type":"stat","title":"` + id + `","gridPos":{"x":` + itoa(x) + `,"y":` + itoa(y) + `,"w":` + itoa(w) + `,"h":` + itoa(h) + `},"datasource":"ch","queries":[{"query":"SELECT 1"}]}`
}

func rowJSON(id string, y int, children ...string) string {
	out := `{"id":"` + id + `","type":"row","title":"` + id + `","gridPos":{"x":0,"y":` + itoa(y) + `,"w":24,"h":1}`
	if len(children) > 0 {
		out += `,"panels":[` + strings.Join(children, ",") + `]`
	}
	return out + `}`
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func patchBase(t *testing.T) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(`{
		"specVersion": "` + spec.Version + `",
		"title": "Patch",
		"time": {"from": "now-1h", "to": "now"},
		"variables": [{"name": "env", "type": "custom", "options": ["a", "b"]}],
		"panels": [` + strings.Join([]string{
		rowJSON("r1", 0, statJSON("a", 0, 1, 12, 4), statJSON("b", 12, 1, 12, 4)),
		rowJSON("r2", 5, statJSON("c", 0, 6, 12, 4)),
		statJSON("t", 0, 11, 12, 4),
	}, ",") + `]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func nodeExporterFull(t *testing.T) *spec.Dashboard {
	t.Helper()
	data, err := os.ReadFile("../grafana/testdata/golden/rfmoz/node-exporter-full.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	d, err := spec.Parse(golden.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func raws(items ...string) []json.RawMessage {
	out := make([]json.RawMessage, len(items))
	for i, s := range items {
		out[i] = json.RawMessage(s)
	}
	return out
}

func fields(time, title, tags, links string) *spec.DashboardFields {
	return &spec.DashboardFields{Time: json.RawMessage(time), Title: json.RawMessage(title), Tags: json.RawMessage(tags), Links: json.RawMessage(links)}
}

func strPtr(s string) *string { return &s }

func apply(t *testing.T, d *spec.Dashboard, patch spec.PanelPatch) *spec.Dashboard {
	t.Helper()
	merged, err := spec.ApplyPanelPatch(d, patch)
	if err != nil {
		t.Fatalf("ApplyPanelPatch: %v", err)
	}
	return merged
}

func rejected(t *testing.T, d *spec.Dashboard, patch spec.PanelPatch) *spec.PatchError {
	t.Helper()
	merged, err := spec.ApplyPanelPatch(d, patch)
	if err == nil {
		t.Fatalf("ApplyPanelPatch succeeded: %+v", merged)
	}
	var pe *spec.PatchError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *PatchError", err)
	}
	if len(pe.Issues) == 0 {
		t.Fatal("PatchError has no issues")
	}
	return pe
}

func changedPaths(from, to *spec.Dashboard) []string {
	var paths []string
	for _, c := range spec.Diff(from, to) {
		paths = append(paths, c.Path)
	}
	return paths
}

func requireOnlyUnder(t *testing.T, from, to *spec.Dashboard, prefix string) {
	t.Helper()
	paths := changedPaths(from, to)
	if len(paths) == 0 {
		t.Fatal("the patch changed nothing")
	}
	for _, p := range paths {
		if p != prefix && !strings.HasPrefix(p, prefix+"/") {
			t.Errorf("change at %s, want only under %s (all: %v)", p, prefix, paths)
		}
	}
}

func panelWith(t *testing.T, d *spec.Dashboard, id string, edit func(map[string]any)) string {
	t.Helper()
	entry, ok := d.FindPanel(id)
	if !ok {
		t.Fatalf("no panel %q", id)
	}
	data, err := json.Marshal(entry.Panel)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func parentRowID(t *testing.T, d *spec.Dashboard, id string) string {
	t.Helper()
	entry, ok := d.FindPanel(id)
	if !ok {
		t.Fatalf("no panel %q in the merged dashboard", id)
	}
	if entry.Row == nil {
		return ""
	}
	return entry.Row.ID
}

func ownerOf(pe *spec.PatchError, path string) string {
	best, id := "", ""
	for ptr, panelID := range pe.PanelIDs {
		if (path == ptr || strings.HasPrefix(path, ptr+"/")) && len(ptr) > len(best) {
			best, id = ptr, panelID
		}
	}
	return id
}

func canonical(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPatchReplacesANestedPanelAndNothingElse(t *testing.T) {
	parent := nodeExporterFull(t)
	target, ok := parent.FindPanel("p323")
	if !ok {
		t.Fatal("golden has no p323")
	}
	edited := panelWith(t, parent, "p323", func(m map[string]any) { m["title"] = "Renamed pressure" })

	merged := apply(t, parent, spec.PanelPatch{Upsert: raws(edited)})

	got, _ := merged.FindPanel("p323")
	if got.Panel.Title != "Renamed pressure" {
		t.Errorf("title = %q", got.Panel.Title)
	}
	if got.Pointer != target.Pointer || parentRowID(t, merged, "p323") != "p261" {
		t.Errorf("panel moved: pointer %s, row %q", got.Pointer, parentRowID(t, merged, "p323"))
	}
	requireOnlyUnder(t, parent, merged, "/panels/p261/panels/p323")
}

func TestPatchAddsAPanelAtTheTopLevelAndIntoARow(t *testing.T) {
	parent := patchBase(t)

	top := apply(t, parent, spec.PanelPatch{Upsert: raws(statJSON("n", 12, 11, 12, 4))})
	if _, ok := top.FindPanel("n"); !ok || parentRowID(t, top, "n") != "" {
		t.Errorf("n should be a top-level panel, row %q", parentRowID(t, top, "n"))
	}

	body := strings.Replace(statJSON("n", 12, 6, 12, 4), `,"gridPos":{"x":12,"y":6,"w":12,"h":4}`, "", 1)
	inRow := apply(t, parent, spec.PanelPatch{
		Upsert: raws(body),
		Layout: []spec.PanelPlacement{{ID: "n", Row: strPtr("r2"), GridPos: &spec.GridPos{X: 12, Y: 6, W: 12, H: 4}}},
	})
	if parentRowID(t, inRow, "n") != "r2" {
		t.Errorf("n row = %q, want r2", parentRowID(t, inRow, "n"))
	}
}

func TestPatchDeletesPanelsAndRows(t *testing.T) {
	parent := patchBase(t)

	one := apply(t, parent, spec.PanelPatch{Delete: []string{"b"}})
	if _, ok := one.FindPanel("b"); ok {
		t.Error("b should be deleted")
	}
	if _, ok := one.FindPanel("a"); !ok {
		t.Error("a should stay")
	}

	pe := rejected(t, parent, spec.PanelPatch{Delete: []string{"r1"}})
	if pe.Issues[0].Code != spec.CodeSchema || !strings.Contains(pe.Issues[0].Message, `"a"`) {
		t.Errorf("issue = %+v, want a schema issue naming the first child", pe.Issues[0])
	}

	whole := apply(t, parent, spec.PanelPatch{Delete: []string{"r1", "a", "b"}})
	for _, id := range []string{"r1", "a", "b"} {
		if _, ok := whole.FindPanel(id); ok {
			t.Errorf("%s should be deleted", id)
		}
	}
	if _, ok := whole.FindPanel("c"); !ok {
		t.Error("c should stay")
	}
}

func TestPatchKeepsTheExistingGridPosWhenOmitted(t *testing.T) {
	parent := patchBase(t)
	edited := panelWith(t, parent, "b", func(m map[string]any) {
		delete(m, "gridPos")
		m["title"] = "Edited"
	})

	merged := apply(t, parent, spec.PanelPatch{Upsert: raws(edited)})

	b, _ := merged.FindPanel("b")
	if b.Panel.Title != "Edited" || b.Panel.GridPos != (spec.GridPos{X: 12, Y: 1, W: 12, H: 4}) {
		t.Errorf("b = %q at %+v", b.Panel.Title, b.Panel.GridPos)
	}
}

func TestPatchKeepsTheChildrenOfAnUpsertedRow(t *testing.T) {
	parent := patchBase(t)
	row := `{"id":"r1","type":"row","title":"Renamed row"}`

	merged := apply(t, parent, spec.PanelPatch{Upsert: raws(row)})

	r1, _ := merged.FindPanel("r1")
	if r1.Panel.Title != "Renamed row" || len(r1.Panel.Panels) != 2 {
		t.Errorf("r1 = %q with %d children, want the new title and 2 children", r1.Panel.Title, len(r1.Panel.Panels))
	}
	if r1.Panel.GridPos != (spec.GridPos{X: 0, Y: 0, W: 24, H: 1}) {
		t.Errorf("r1 gridPos = %+v", r1.Panel.GridPos)
	}
}

func TestPatchAddsANewRowWithItsChildren(t *testing.T) {
	parent := patchBase(t)
	row := rowJSON("r3", 20, statJSON("x", 0, 21, 12, 4), statJSON("y", 12, 21, 12, 4))

	merged := apply(t, parent, spec.PanelPatch{Upsert: raws(row)})

	if parentRowID(t, merged, "r3") != "" {
		t.Errorf("r3 should be a top-level row, parent %q", parentRowID(t, merged, "r3"))
	}
	for _, id := range []string{"x", "y"} {
		if parentRowID(t, merged, id) != "r3" {
			t.Errorf("%s row = %q, want r3", id, parentRowID(t, merged, id))
		}
	}
	r3, _ := merged.FindPanel("r3")
	if len(r3.Panel.Panels) != 2 {
		t.Errorf("r3 has %d children, want 2", len(r3.Panel.Panels))
	}
	requireOnlyUnder(t, parent, merged, "/panels/r3")
}

func TestPatchGivesChildrenToAnExistingEmptyRow(t *testing.T) {
	for name, empty := range map[string]string{
		"no panels key":      rowJSON("e", 20),
		"empty panels array": strings.TrimSuffix(rowJSON("e", 20), "}") + `,"panels":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			parent, err := spec.Parse([]byte(`{
				"specVersion": "` + spec.Version + `",
				"title": "Patch",
				"time": {"from": "now-1h", "to": "now"},
				"panels": [` + empty + `]
			}`))
			if err != nil {
				t.Fatal(err)
			}

			merged := apply(t, parent, spec.PanelPatch{Upsert: raws(rowJSON("e", 20, statJSON("x", 0, 21, 12, 4), statJSON("y", 12, 21, 12, 4)))})

			e, _ := merged.FindPanel("e")
			if len(e.Panel.Panels) != 2 {
				t.Fatalf("e has %d children, want 2", len(e.Panel.Panels))
			}
			if parentRowID(t, merged, "x") != "e" || parentRowID(t, merged, "y") != "e" {
				t.Errorf("children rows = %q and %q, want e", parentRowID(t, merged, "x"), parentRowID(t, merged, "y"))
			}
		})
	}
}

func TestPatchRejectsChildrenForARowThatAlreadyHasPanels(t *testing.T) {
	parent := patchBase(t)

	pe := rejected(t, parent, spec.PanelPatch{Upsert: raws(rowJSON("r1", 0, statJSON("z", 0, 1, 12, 4)))})

	if pe.Issues[0].Path != "/upsert/0/panels" || !strings.Contains(pe.Issues[0].Message, `row "r1" already has panels`) {
		t.Errorf("issue = %+v", pe.Issues[0])
	}
}

func TestPatchRejectsChildrenOnAPanelThatIsNotARow(t *testing.T) {
	parent := patchBase(t)
	for name, body := range map[string]string{
		"new panel":      strings.Replace(statJSON("n", 0, 20, 12, 4), `"datasource"`, `"panels":[`+statJSON("z", 0, 21, 12, 4)+`],"datasource"`, 1),
		"existing panel": panelWith(t, parent, "t", func(m map[string]any) { m["panels"] = []any{} }),
	} {
		t.Run(name, func(t *testing.T) {
			pe := rejected(t, parent, spec.PanelPatch{Upsert: raws(body)})
			if pe.Issues[0].Path != "/upsert/0/panels" || !strings.Contains(pe.Issues[0].Message, "is not a row") {
				t.Errorf("issue = %+v", pe.Issues[0])
			}
		})
	}
}

func TestPatchMovesAPanelAcrossRows(t *testing.T) {
	parent := patchBase(t)

	toRow := apply(t, parent, spec.PanelPatch{Layout: []spec.PanelPlacement{
		{ID: "a", Row: strPtr("r2"), GridPos: &spec.GridPos{X: 12, Y: 6, W: 12, H: 4}},
	}})
	if parentRowID(t, toRow, "a") != "r2" {
		t.Errorf("a row = %q, want r2", parentRowID(t, toRow, "a"))
	}
	if r1, _ := toRow.FindPanel("r1"); len(r1.Panel.Panels) != 1 {
		t.Errorf("r1 has %d children, want 1", len(r1.Panel.Panels))
	}

	toTop := apply(t, parent, spec.PanelPatch{Layout: []spec.PanelPlacement{
		{ID: "a", Row: strPtr(""), GridPos: &spec.GridPos{X: 12, Y: 11, W: 12, H: 4}},
	}})
	if _, ok := toTop.FindPanel("a"); !ok || parentRowID(t, toTop, "a") != "" {
		t.Errorf("a should be top level, row %q", parentRowID(t, toTop, "a"))
	}

	keep := apply(t, parent, spec.PanelPatch{Layout: []spec.PanelPlacement{
		{ID: "t", GridPos: &spec.GridPos{X: 12, Y: 11, W: 12, H: 4}},
	}})
	if got, _ := keep.FindPanel("t"); got.Panel.GridPos.X != 12 || parentRowID(t, keep, "t") != "" {
		t.Errorf("t = %+v in row %q", got.Panel.GridPos, parentRowID(t, keep, "t"))
	}
	keepRow := apply(t, parent, spec.PanelPatch{Layout: []spec.PanelPlacement{
		{ID: "b", GridPos: &spec.GridPos{X: 12, Y: 1, W: 6, H: 4}},
	}})
	if parentRowID(t, keepRow, "b") != "r1" {
		t.Errorf("b row = %q, want r1 kept", parentRowID(t, keepRow, "b"))
	}
}

func TestPatchRejectsBadPlacements(t *testing.T) {
	parent := patchBase(t)
	tests := []struct {
		name   string
		layout spec.PanelPlacement
		path   string
	}{
		{"unknown panel", spec.PanelPlacement{ID: "zz", Row: strPtr("")}, "/layout/0"},
		{"unknown row", spec.PanelPlacement{ID: "a", Row: strPtr("zz")}, "/layout/0/row"},
		{"target is not a row", spec.PanelPlacement{ID: "a", Row: strPtr("t")}, "/layout/0/row"},
		{"row inside a row", spec.PanelPlacement{ID: "r2", Row: strPtr("r1")}, "/layout/0/row"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := rejected(t, parent, spec.PanelPatch{Layout: []spec.PanelPlacement{tt.layout}})
			if pe.Issues[0].Path != tt.path || pe.Issues[0].Code != spec.CodeSchema {
				t.Errorf("issue = %+v, want a schema issue at %s", pe.Issues[0], tt.path)
			}
		})
	}
}

func TestPatchReplacesTheVariables(t *testing.T) {
	parent := patchBase(t)

	merged := apply(t, parent, spec.PanelPatch{Variables: json.RawMessage(`[{"name":"region","type":"custom","options":["x"]}]`)})

	if len(merged.Variables) != 1 || merged.Variables[0].Name != "region" {
		t.Errorf("variables = %+v, want only region", merged.Variables)
	}
	if len(parent.Variables) != 1 || parent.Variables[0].Name != "env" {
		t.Errorf("parent variables = %+v", parent.Variables)
	}

	cleared := apply(t, parent, spec.PanelPatch{Variables: json.RawMessage(`[]`)})
	if len(cleared.Variables) != 0 {
		t.Errorf("variables = %+v, want none", cleared.Variables)
	}
}

func TestPatchThatOverlapsNamesThePanel(t *testing.T) {
	parent := patchBase(t)
	overlapping := panelWith(t, parent, "t", func(m map[string]any) {
		m["gridPos"] = map[string]any{"x": 0, "y": 6, "w": 12, "h": 4}
	})

	pe := rejected(t, parent, spec.PanelPatch{Upsert: raws(overlapping)})

	var overlap *spec.Issue
	for i := range pe.Issues {
		if pe.Issues[i].Code == spec.CodeGridOverlap {
			overlap = &pe.Issues[i]
		}
	}
	if overlap == nil {
		t.Fatalf("issues = %+v, want a grid_overlap", pe.Issues)
	}
	if got := ownerOf(pe, overlap.Path); got != "t" && got != "c" {
		t.Errorf("owner of %s = %q, want t or c", overlap.Path, got)
	}
	if pe.PanelIDs["/panels/2"] != "t" || pe.PanelIDs["/panels/1/panels/0"] != "c" || pe.PanelIDs["/panels/0"] != "r1" {
		t.Errorf("PanelIDs = %v", pe.PanelIDs)
	}
}

func TestPatchRejectsMalformedPatches(t *testing.T) {
	parent := patchBase(t)
	tests := []struct {
		name  string
		patch spec.PanelPatch
		path  string
		code  spec.IssueCode
	}{
		{"empty patch", spec.PanelPatch{}, "", spec.CodeSchema},
		{"dashboard with no keys", spec.PanelPatch{Dashboard: &spec.DashboardFields{}}, "", spec.CodeSchema},
		{"unknown delete", spec.PanelPatch{Delete: []string{"zz"}}, "/delete/0", spec.CodeSchema},
		{"duplicate upsert", spec.PanelPatch{Upsert: raws(statJSON("n", 12, 11, 6, 4), statJSON("n", 18, 11, 6, 4))}, "/upsert/1", spec.CodeSchema},
		{"duplicate delete", spec.PanelPatch{Delete: []string{"t", "t"}}, "/delete/1", spec.CodeSchema},
		{"duplicate layout", spec.PanelPatch{Layout: []spec.PanelPlacement{{ID: "t"}, {ID: "t"}}}, "/layout/1", spec.CodeSchema},
		{"upserted and deleted", spec.PanelPatch{Upsert: raws(statJSON("t", 0, 11, 12, 4)), Delete: []string{"t"}}, "/delete/0", spec.CodeSchema},
		{"upsert without an id", spec.PanelPatch{Upsert: raws(`{"type":"stat"}`)}, "/upsert/0/id", spec.CodeSchema},
		{"upsert that is not an object", spec.PanelPatch{Upsert: raws(`[1]`)}, "/upsert/0", spec.CodeSchema},
		{"upsert with a duplicate key", spec.PanelPatch{Upsert: raws(`{"id":"t","title":"a","title":"b"}`)}, "/upsert/0/title", spec.CodeDuplicateKey},
		{"upsert that is not json", spec.PanelPatch{Upsert: raws(`{"id":`)}, "/upsert/0", spec.CodeMalformedJSON},
		{"variables with a duplicate key", spec.PanelPatch{Variables: json.RawMessage(`[{"name":"a","name":"b"}]`)}, "/variables/0/name", spec.CodeDuplicateKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := rejected(t, parent, tt.patch)
			if pe.Issues[0].Path != tt.path || pe.Issues[0].Code != tt.code {
				t.Errorf("issue = %+v, want %s at %q", pe.Issues[0], tt.code, tt.path)
			}
		})
	}
}

func TestPatchDoesNotMutateTheInput(t *testing.T) {
	parent := patchBase(t)
	before := canonical(t, parent)

	apply(t, parent, spec.PanelPatch{
		Upsert:    raws(panelWith(t, parent, "a", func(m map[string]any) { m["title"] = "X" }), statJSON("n", 12, 11, 12, 4)),
		Delete:    []string{"c"},
		Layout:    []spec.PanelPlacement{{ID: "b", Row: strPtr("r2"), GridPos: &spec.GridPos{X: 12, Y: 6, W: 12, H: 4}}},
		Variables: json.RawMessage(`[]`),
		Dashboard: fields(`{"from":"now-6h","to":"now"}`, `"New"`, `["x"]`, `[{"title":"T","url":"https://example.com"}]`),
	})
	if _, err := spec.ApplyPanelPatch(parent, spec.PanelPatch{Delete: []string{"zz"}}); err == nil {
		t.Fatal("expected a rejection")
	}

	if after := canonical(t, parent); !reflect.DeepEqual(before, after) {
		t.Errorf("the input changed\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestPatchMergesToTheExpectedDocument(t *testing.T) {
	parent, err := spec.Parse([]byte(`{
		"specVersion": "` + spec.Version + `",
		"title": "Small",
		"time": {"from": "now-1h", "to": "now"},
		"panels": [` + statJSON("one", 0, 0, 12, 4) + `]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal([]byte(`{
		"specVersion": "`+spec.Version+`",
		"title": "Small",
		"tags": ["ops"],
		"time": {"from": "now-6h", "to": "now"},
		"panels": [
			{"id":"one","type":"stat","title":"Renamed","gridPos":{"x":0,"y":0,"w":12,"h":4},"datasource":"ch","queries":[{"query":"SELECT 1"}]},
			{"id":"two","type":"stat","title":"two","gridPos":{"x":12,"y":0,"w":12,"h":4},"datasource":"ch","queries":[{"query":"SELECT 1"}]}
		]
	}`), &want); err != nil {
		t.Fatal(err)
	}

	merged := apply(t, parent, spec.PanelPatch{
		Upsert: raws(
			`{"id":"one","type":"stat","title":"Renamed","datasource":"ch","queries":[{"query":"SELECT 1"}]}`,
			statJSON("two", 12, 0, 12, 4),
		),
		Dashboard: fields(`{"from":"now-6h","to":"now"}`, "", `["ops"]`, ""),
	})

	data, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged document\n got: %s\nwant: %v", data, want)
	}
}

func TestPatchDashboardFieldsEachChangeOnlyTheirOwnKey(t *testing.T) {
	parent := nodeExporterFull(t)
	tests := []struct {
		name   string
		fields *spec.DashboardFields
		path   string
	}{
		{"time", fields(`{"from":"now-6h","to":"now"}`, "", "", ""), "/time"},
		{"title", fields("", `"Renamed"`, "", ""), "/title"},
		{"tags", fields("", "", `["linux","prod"]`, ""), "/tags"},
		{"links", fields("", "", "", `[{"title":"Docs","url":"https://example.com/docs"}]`), "/links"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := apply(t, parent, spec.PanelPatch{Dashboard: tt.fields})
			requireOnlyUnder(t, parent, merged, tt.path)
		})
	}
}

func TestPatchAbsentDashboardKeysKeepTheParentValues(t *testing.T) {
	parent := nodeExporterFull(t)

	merged := apply(t, parent, spec.PanelPatch{Dashboard: fields("", `"Renamed"`, "", "")})

	if merged.Title != "Renamed" || merged.Time != parent.Time || !reflect.DeepEqual(merged.Tags, parent.Tags) || !reflect.DeepEqual(merged.Links, parent.Links) {
		t.Errorf("title %q, time %+v, tags %v, links %d", merged.Title, merged.Time, merged.Tags, len(merged.Links))
	}
	if len(parent.Tags) == 0 || len(parent.Links) == 0 {
		t.Fatal("the golden should carry tags and links")
	}
}

func TestPatchAppliesDashboardFieldsAndAnUpsertTogether(t *testing.T) {
	parent := patchBase(t)

	merged := apply(t, parent, spec.PanelPatch{
		Upsert:    raws(panelWith(t, parent, "t", func(m map[string]any) { m["title"] = "Edited" })),
		Dashboard: fields("", `"Both"`, "", ""),
	})

	if got, _ := merged.FindPanel("t"); got.Panel.Title != "Edited" || merged.Title != "Both" {
		t.Errorf("panel title %q, dashboard title %q", got.Panel.Title, merged.Title)
	}
}

func TestPatchEmptyTagsAndLinksClearThem(t *testing.T) {
	parent := nodeExporterFull(t)

	merged := apply(t, parent, spec.PanelPatch{Dashboard: fields("", "", `[]`, `[]`)})

	if len(merged.Tags) != 0 || len(merged.Links) != 0 {
		t.Errorf("tags %v, links %d, want none", merged.Tags, len(merged.Links))
	}
	data, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tags", "links"} {
		if _, ok := doc[key]; ok {
			t.Errorf("canonical JSON still has %q", key)
		}
	}
}

func TestPatchRejectsBadDashboardFieldsWithTheirPath(t *testing.T) {
	parent := patchBase(t)
	tests := []struct {
		name   string
		fields *spec.DashboardFields
		path   string
	}{
		{"empty title", fields("", `""`, "", ""), "/title"},
		{"null title", fields("", `null`, "", ""), "/title"},
		{"null time", fields(`null`, "", "", ""), "/time"},
		{"time without to", fields(`{"from":"now-6h"}`, "", "", ""), "/time"},
		{"time from that is not a time", fields(`{"from":"yesterday","to":"now"}`, "", "", ""), "/time/from"},
		{"uppercase tag", fields("", "", `["Prod"]`, ""), "/tags/0"},
		{"21 tags", fields("", "", `["a","b","c","d","e","f","g","h","i","j","k","l","m","n","o","p","q","r","s","t","u"]`, ""), "/tags"},
		{"link without a title", fields("", "", "", `[{"url":"https://example.com"}]`), "/links/0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := rejected(t, parent, spec.PanelPatch{Dashboard: tt.fields})
			found := false
			for _, i := range pe.Issues {
				if i.Path == tt.path || strings.HasPrefix(i.Path, tt.path+"/") {
					found = true
				}
			}
			if !found {
				t.Errorf("issues = %+v, want one at %s", pe.Issues, tt.path)
			}
			if got := ownerOf(pe, pe.Issues[0].Path); got != "" {
				t.Errorf("a dashboard path names panel %q", got)
			}
		})
	}
}
