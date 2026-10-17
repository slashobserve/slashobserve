package spec_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func diffBase(t *testing.T) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse(readExample(t, "shop-overview.json"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func diffMutated(t *testing.T, mutate func(d *spec.Dashboard)) *spec.Dashboard {
	t.Helper()
	d := diffBase(t)
	mutate(d)
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	out, err := spec.Parse(data)
	if err != nil {
		t.Fatalf("mutated dashboard is invalid: %v", err)
	}
	return out
}

func panelIndex(t *testing.T, d *spec.Dashboard, id string) int {
	t.Helper()
	i := slices.IndexFunc(d.Panels, func(p spec.Panel) bool { return p.ID == id })
	if i < 0 {
		t.Fatalf("panel %q not found", id)
	}
	return i
}

func marshalJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func canonicalJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	var tree any
	if err := json.Unmarshal([]byte(marshalJSON(t, v)), &tree); err != nil {
		t.Fatal(err)
	}
	return json.RawMessage(marshalJSON(t, tree))
}

func assertChanges(t *testing.T, got, want []spec.Change) {
	t.Helper()
	if g, w := marshalJSON(t, got), marshalJSON(t, want); g != w {
		t.Fatalf("changes mismatch\n got: %s\nwant: %s", g, w)
	}
}

func TestDiffIdentical(t *testing.T) {
	got := spec.Diff(diffBase(t), diffBase(t))
	if got == nil {
		t.Fatal("Diff returned nil, want empty slice")
	}
	if s := marshalJSON(t, got); s != "[]" {
		t.Fatalf("json = %s, want []", s)
	}
}

func TestDiffTitleChanged(t *testing.T) {
	base := diffBase(t)
	to := diffMutated(t, func(d *spec.Dashboard) { d.Title = "Renamed" })
	assertChanges(t, spec.Diff(base, to), []spec.Change{
		{Op: spec.OpReplace, Path: "/title", From: json.RawMessage(marshalJSON(t, base.Title)), To: json.RawMessage(`"Renamed"`)},
	})
}

func TestDiffPanelInsertedAtFront(t *testing.T) {
	to := diffMutated(t, func(d *spec.Dashboard) {
		p := d.Panels[0]
		p.ID = "new_panel"
		p.Title = "New panel"
		p.GridPos = spec.GridPos{X: 0, Y: 16, W: 24, H: 8}
		d.Panels = slices.Insert(d.Panels, 0, p)
	})
	got := spec.Diff(diffBase(t), to)
	if len(got) != 1 || got[0].Op != spec.OpAdd || got[0].Path != "/panels/new_panel" || got[0].From != nil {
		t.Fatalf("changes = %s, want a single add at /panels/new_panel", marshalJSON(t, got))
	}
	var added spec.Panel
	if err := json.Unmarshal(got[0].To, &added); err != nil {
		t.Fatal(err)
	}
	if added.ID != "new_panel" || added.GridPos != (spec.GridPos{X: 0, Y: 16, W: 24, H: 8}) {
		t.Fatalf("added panel = %+v", added)
	}
}

func TestDiffPanelRemoved(t *testing.T) {
	base := diffBase(t)
	removed := base.Panels[panelIndex(t, base, "log_volume")]
	to := diffMutated(t, func(d *spec.Dashboard) {
		d.Panels = slices.DeleteFunc(d.Panels, func(p spec.Panel) bool { return p.ID == "log_volume" })
	})
	assertChanges(t, spec.Diff(base, to), []spec.Change{
		{Op: spec.OpRemove, Path: "/panels/log_volume", From: canonicalJSON(t, removed)},
	})
}

func TestDiffPanelsSwapped(t *testing.T) {
	base := diffBase(t)
	to := diffMutated(t, func(d *spec.Dashboard) {
		i, j := panelIndex(t, d, "cpu_by_pod"), panelIndex(t, d, "rps_by_route")
		d.Panels[i], d.Panels[j] = d.Panels[j], d.Panels[i]
	})
	assertChanges(t, spec.Diff(base, to), []spec.Change{
		{
			Op:   spec.OpReorder,
			Path: "/panels",
			From: json.RawMessage(`["error_rate","cpu_by_pod","rps_by_route","log_volume","slow_spans"]`),
			To:   json.RawMessage(`["error_rate","rps_by_route","cpu_by_pod","log_volume","slow_spans"]`),
		},
	})
}

func TestDiffGridPosWidthChanged(t *testing.T) {
	to := diffMutated(t, func(d *spec.Dashboard) {
		d.Panels[panelIndex(t, d, "cpu_by_pod")].GridPos.W = 5
	})
	assertChanges(t, spec.Diff(diffBase(t), to), []spec.Change{
		{Op: spec.OpReplace, Path: "/panels/cpu_by_pod/gridPos/w", From: json.RawMessage(`6`), To: json.RawMessage(`5`)},
	})
}

func TestDiffThresholdsReplacedWhole(t *testing.T) {
	to := diffMutated(t, func(d *spec.Dashboard) {
		d.Panels[panelIndex(t, d, "error_rate")].Thresholds[0].Value = 0.02
	})
	assertChanges(t, spec.Diff(diffBase(t), to), []spec.Change{
		{
			Op:   spec.OpReplace,
			Path: "/panels/error_rate/thresholds",
			From: json.RawMessage(`[{"color":"warning","value":0.01},{"color":"critical","value":0.05}]`),
			To:   json.RawMessage(`[{"color":"warning","value":0.02},{"color":"critical","value":0.05}]`),
		},
	})
}

func TestDiffOptionalDescriptionRemoved(t *testing.T) {
	base := diffBase(t)
	to := diffMutated(t, func(d *spec.Dashboard) { d.Description = "" })
	assertChanges(t, spec.Diff(base, to), []spec.Change{
		{Op: spec.OpRemove, Path: "/description", From: json.RawMessage(marshalJSON(t, base.Description))},
	})
}

func TestDiffDeterministic(t *testing.T) {
	to := diffMutated(t, func(d *spec.Dashboard) {
		d.Title = "Changed"
		d.Description = "Added"
		d.Tags = []string{"x"}
		d.Panels = slices.DeleteFunc(d.Panels, func(p spec.Panel) bool { return p.ID == "log_volume" })
		i, j := panelIndex(t, d, "cpu_by_pod"), panelIndex(t, d, "rps_by_route")
		d.Panels[i], d.Panels[j] = d.Panels[j], d.Panels[i]
		d.Panels[panelIndex(t, d, "slow_spans")].GridPos.W = 20
		d.Variables[0].Options = []string{"checkout"}
	})
	first := marshalJSON(t, spec.Diff(diffBase(t), to))
	if first == "[]" {
		t.Fatal("expected changes")
	}
	for range 20 {
		if got := marshalJSON(t, spec.Diff(diffBase(t), to)); !bytes.Equal([]byte(got), []byte(first)) {
			t.Fatalf("non-deterministic output\n got: %s\nwant: %s", got, first)
		}
	}
}

func withVariables(d *spec.Dashboard, vars []spec.Variable) *spec.Dashboard {
	out := *d
	out.Variables = vars
	return &out
}

func TestDiffFirstVariableAdded(t *testing.T) {
	to := diffBase(t)
	from := withVariables(to, nil)
	var all []spec.Change
	for _, v := range to.Variables {
		all = append(all, spec.Change{Op: spec.OpAdd, Path: "/variables/" + v.Name, To: canonicalJSON(t, v)})
	}
	assertChanges(t, spec.Diff(from, to), all)

	one := withVariables(to, to.Variables[:1])
	assertChanges(t, spec.Diff(from, one), all[:1])
}

func TestDiffLastVariableRemoved(t *testing.T) {
	base := diffBase(t)
	from := withVariables(base, base.Variables[:1])
	to := withVariables(base, nil)
	assertChanges(t, spec.Diff(from, to), []spec.Change{
		{Op: spec.OpRemove, Path: "/variables/service", From: canonicalJSON(t, from.Variables[0])},
	})
}

func TestDiffVariableFieldChanged(t *testing.T) {
	base := diffBase(t)
	to := diffMutated(t, func(d *spec.Dashboard) { d.Variables[1].Query = "label_values(up, route)" })
	assertChanges(t, spec.Diff(base, to), []spec.Change{
		{Op: spec.OpReplace, Path: "/variables/route/query", From: json.RawMessage(marshalJSON(t, base.Variables[1].Query)), To: json.RawMessage(`"label_values(up, route)"`)},
	})
}

func TestDiffDuplicatePanelIDsFallBackToWholeArray(t *testing.T) {
	base := diffBase(t)
	dup := *base
	dup.Panels = []spec.Panel{base.Panels[0], base.Panels[0]}
	got := spec.Diff(base, &dup)
	assertChanges(t, got, []spec.Change{
		{Op: spec.OpReplace, Path: "/panels", From: canonicalJSON(t, base.Panels), To: canonicalJSON(t, dup.Panels)},
	})
}

func TestDiffEscapesKeysInPointerPaths(t *testing.T) {
	const name = "a/b~c"
	dashboard := func(vars ...spec.Variable) *spec.Dashboard {
		return &spec.Dashboard{SpecVersion: spec.Version, Title: "t", Variables: vars, Panels: []spec.Panel{}}
	}
	v := spec.Variable{Name: name, Type: spec.CustomVariable, Options: []string{"x"}}
	changed := v
	changed.Label = "L"
	cases := []struct {
		name     string
		from, to *spec.Dashboard
		want     spec.Change
	}{
		{"add", dashboard(), dashboard(v), spec.Change{Op: spec.OpAdd, Path: "/variables/a~1b~0c", To: canonicalJSON(t, v)}},
		{"remove", dashboard(v), dashboard(), spec.Change{Op: spec.OpRemove, Path: "/variables/a~1b~0c", From: canonicalJSON(t, v)}},
		{"nested", dashboard(v), dashboard(changed), spec.Change{Op: spec.OpAdd, Path: "/variables/a~1b~0c/label", To: json.RawMessage(`"L"`)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertChanges(t, spec.Diff(c.from, c.to), []spec.Change{c.want})
		})
	}
}
