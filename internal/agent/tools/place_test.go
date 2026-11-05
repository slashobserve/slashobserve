package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const placedID = "new"

func placeStat(id string, x, y, w, h int) string {
	return fmt.Sprintf(`{"id":%q,"type":"stat","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":%d},"datasource":%q,"queries":[{"query":"up"}]}`, id, id, x, y, w, h, fakeUID)
}

func placeSeries(id string, x, y, w, h int) string {
	return fmt.Sprintf(`{"id":%q,"type":"timeseries","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":%d},"datasource":%q,"queries":[{"query":"up"}]}`, id, id, x, y, w, h, fakeUID)
}

func placeRow(id string, y int, collapsed bool, children ...string) string {
	return fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"collapsed":%t,"gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, id, id, collapsed, y, strings.Join(children, ","))
}

type shiftTo struct {
	id string
	y  int
}

type parentOf map[string]string

func parentsOf(d *spec.Dashboard) parentOf {
	parents := parentOf{}
	for _, e := range d.AllPanels() {
		parents[e.Panel.ID] = ""
		if e.Row != nil {
			parents[e.Panel.ID] = e.Row.ID
		}
	}
	return parents
}

func applyPlacement(t *testing.T, d *spec.Dashboard, rowID string, plan layoutPlan) *spec.Dashboard {
	t.Helper()
	pos := plan.Positions[placedID]
	panel := placeStat(placedID, pos.X, pos.Y, pos.W, pos.H)
	existing := false
	for _, p := range d.Panels {
		existing = existing || (p.Type == spec.Row && p.ID == rowID)
	}
	patch := spec.PanelPatch{Layout: plan.Moves}
	switch rowPos, isNew := plan.NewRows[rowID]; {
	case isNew:
		row := fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":%d},"panels":[%s]}`, rowID, rowID, rowPos.X, rowPos.Y, rowPos.W, rowPos.H, panel)
		patch.Upsert = []json.RawMessage{json.RawMessage(row)}
	case existing:
		patch.Upsert = []json.RawMessage{json.RawMessage(panel)}
		patch.Layout = append([]spec.PanelPlacement{{ID: placedID, Row: &rowID}}, plan.Moves...)
	default:
		patch.Upsert = []json.RawMessage{json.RawMessage(panel)}
	}
	merged, err := spec.ApplyPanelPatch(d, patch)
	if err != nil {
		t.Fatalf("applying the placement: %v", err)
	}
	return merged
}

func assertExistingPanelsKept(t *testing.T, before, after *spec.Dashboard, rowID string) {
	t.Helper()
	oldParents, newParents := parentsOf(before), parentsOf(after)
	for _, e := range before.AllPanels() {
		moved, ok := after.FindPanel(e.Panel.ID)
		if !ok {
			t.Fatalf("panel %q disappeared", e.Panel.ID)
		}
		if moved.Panel.GridPos.W != e.Panel.GridPos.W || moved.Panel.GridPos.H != e.Panel.GridPos.H || moved.Panel.GridPos.X != e.Panel.GridPos.X {
			t.Errorf("panel %q changed from %+v to %+v; only y may move", e.Panel.ID, e.Panel.GridPos, moved.Panel.GridPos)
		}
		if oldParents[e.Panel.ID] != newParents[e.Panel.ID] {
			t.Errorf("panel %q moved from row %q to row %q", e.Panel.ID, oldParents[e.Panel.ID], newParents[e.Panel.ID])
		}
	}
	if got := newParents[placedID]; got != rowID {
		t.Errorf("new panel is in row %q, want %q", got, rowID)
	}
}

func caseABoard() []string {
	return []string{
		placeRow("row_cpu", 0, false, placeStat("usage", 0, 1, 12, 8), placeStat("cores", 12, 1, 4, 4), placeStat("load", 16, 1, 8, 4)),
		placeRow("row_mem", 9, false, placeStat("m1", 0, 10, 12, 8), placeStat("m2", 12, 10, 12, 8)),
	}
}

func caseBBoard(extra ...string) []string {
	basic := append([]string{placeStat("b1", 0, 6, 4, 4), placeStat("b2", 4, 6, 8, 4), placeStat("b3", 12, 6, 8, 4)}, extra...)
	cmndY := 10
	if len(extra) > 0 {
		cmndY = 14
	}
	return []string{
		placeRow("row_quick", 0, false, placeStat("q1", 0, 1, 6, 4), placeStat("q2", 6, 1, 6, 4), placeStat("q3", 12, 1, 6, 4), placeStat("q4", 18, 1, 6, 4)),
		placeRow("row_basic", 5, false, basic...),
		placeRow("row_cmnd", cmndY, false, placeStat("c1", 0, cmndY+1, 8, 7)),
	}
}

func caseCBoard() []string {
	return []string{
		placeStat("p_cpu_busy", 0, 0, 4, 5),
		placeRow("r_quick", 5, false, placeStat("p_mem_busy", 0, 6, 6, 5)),
	}
}

func TestPlaceChoosesWhereAPanelGoes(t *testing.T) {
	cases := []struct {
		name       string
		panels     []string
		rowID      string
		w, h       int
		wantPanel  spec.GridPos
		wantRow    *spec.GridPos
		wantShifts []shiftTo
	}{
		{
			name:      "the first panel of an empty dashboard goes to the origin",
			rowID:     "",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 0, W: 12, H: 8},
		},
		{
			name:       "a top level panel on a dashboard of only rows goes above them",
			panels:     []string{placeRow("r1", 0, false, placeStat("c1", 0, 1, 12, 8))},
			w:          12,
			h:          8,
			wantPanel:  spec.GridPos{X: 0, Y: 0, W: 12, H: 8},
			wantShifts: []shiftTo{{"r1", 8}, {"c1", 9}},
		},
		{
			name:      "a second panel goes beside the first",
			panels:    []string{placeStat("a", 0, 0, 12, 8)},
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 12, Y: 0, W: 12, H: 8},
		},
		{
			name:      "a third panel wraps under the first two",
			panels:    []string{placeStat("a", 0, 0, 12, 8), placeStat("b", 12, 0, 12, 8)},
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 8, W: 12, H: 8},
		},
		{
			name:      "a panel too wide for the rest of the line wraps",
			panels:    []string{placeStat("a", 0, 0, 18, 8)},
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 8, W: 12, H: 8},
		},
		{
			name: "a new row goes below panels and rows with its panel one line under it",
			panels: []string{
				placeStat("a", 0, 0, 12, 8),
				placeRow("r1", 8, false, placeStat("c1", 0, 9, 12, 8)),
			},
			rowID:     "r2",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 18, W: 12, H: 8},
			wantRow:   &spec.GridPos{X: 0, Y: 17, W: 24, H: 1},
		},
		{
			name:      "a new row on an empty dashboard starts at the top",
			rowID:     "r1",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 1, W: 12, H: 8},
			wantRow:   &spec.GridPos{X: 0, Y: 0, W: 24, H: 1},
		},
		{
			name:      "the last row takes a panel beside its last panel",
			panels:    []string{placeRow("r1", 0, false, placeStat("c1", 0, 1, 12, 8))},
			rowID:     "r1",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 12, Y: 1, W: 12, H: 8},
		},
		{
			name: "the last row wraps when its line is full",
			panels: []string{placeRow("r1", 0, false,
				placeStat("c1", 0, 1, 12, 8), placeStat("c2", 12, 1, 12, 8))},
			rowID:     "r1",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 9, W: 12, H: 8},
		},
		{
			name: "a wrap in a middle row moves the next row and its children down by the panel height",
			panels: []string{
				placeStat("t", 0, 0, 12, 8),
				placeRow("r1", 8, false, placeStat("c1", 0, 9, 12, 8), placeStat("c2", 12, 9, 12, 8)),
				placeRow("r2", 17, false, placeStat("d1", 0, 18, 12, 8)),
			},
			rowID:      "r1",
			w:          12,
			h:          8,
			wantPanel:  spec.GridPos{X: 0, Y: 17, W: 12, H: 8},
			wantShifts: []shiftTo{{"r2", 25}, {"d1", 26}},
		},
		{
			name: "a taller panel beside a shorter last one moves the rows below by the difference only",
			panels: []string{
				placeRow("r1", 0, false, placeStat("c1", 0, 1, 6, 4)),
				placeRow("r2", 5, false, placeStat("d1", 0, 6, 12, 8)),
			},
			rowID:      "r1",
			w:          6,
			h:          8,
			wantPanel:  spec.GridPos{X: 6, Y: 1, W: 6, H: 8},
			wantShifts: []shiftTo{{"r2", 9}, {"d1", 10}},
		},
		{
			name: "a panel beside an earlier taller panel wraps instead of overlapping it",
			panels: []string{placeRow("r1", 0, false,
				placeStat("c1", 12, 1, 12, 8), placeStat("c2", 0, 5, 12, 4))},
			rowID:     "r1",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 9, W: 12, H: 8},
		},
		{
			name: "a top level panel that wraps moves every row down",
			panels: []string{
				placeStat("t1", 0, 0, 12, 8),
				placeStat("t2", 12, 0, 12, 8),
				placeRow("r1", 8, false, placeStat("c1", 0, 9, 12, 8)),
				placeRow("r2", 17, false, placeStat("d1", 0, 18, 12, 8)),
			},
			w:          12,
			h:          8,
			wantPanel:  spec.GridPos{X: 0, Y: 8, W: 12, H: 8},
			wantShifts: []shiftTo{{"r1", 16}, {"c1", 17}, {"r2", 25}, {"d1", 26}},
		},
		{
			name: "a collapsed row takes a panel among its children and moves nothing",
			panels: []string{
				placeRow("r1", 0, true, placeStat("c1", 0, 1, 12, 8), placeStat("c2", 12, 1, 12, 8)),
				placeRow("r2", 1, false, placeStat("d1", 0, 2, 12, 8)),
			},
			rowID:     "r1",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 9, W: 12, H: 8},
		},
		{
			name: "a collapsed row below an insertion moves with its children",
			panels: []string{
				placeStat("t", 0, 0, 24, 8),
				placeRow("r1", 8, true, placeStat("c1", 0, 9, 12, 8)),
				placeRow("r2", 9, false, placeStat("d1", 0, 10, 12, 8)),
			},
			w:          12,
			h:          8,
			wantPanel:  spec.GridPos{X: 0, Y: 8, W: 12, H: 8},
			wantShifts: []shiftTo{{"r1", 16}, {"c1", 17}, {"r2", 17}, {"d1", 18}},
		},
		{
			name: "a new row counts a collapsed row as one line",
			panels: []string{
				placeRow("r1", 0, true, placeStat("c1", 0, 1, 12, 8)),
			},
			rowID:     "r2",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 2, W: 12, H: 8},
			wantRow:   &spec.GridPos{X: 0, Y: 1, W: 24, H: 1},
		},
		{
			name:      "case A: a panel as short as its neighbours fills the space under them",
			panels:    caseABoard(),
			rowID:     "row_cpu",
			w:         12,
			h:         4,
			wantPanel: spec.GridPos{X: 12, Y: 5, W: 12, H: 4},
		},
		{
			name:      "case A: a narrower short panel fills the space from its left",
			panels:    caseABoard(),
			rowID:     "row_cpu",
			w:         8,
			h:         4,
			wantPanel: spec.GridPos{X: 12, Y: 5, W: 8, H: 4},
		},
		{
			name:       "case A: a panel taller than the space moves the next row by the overrun only",
			panels:     caseABoard(),
			rowID:      "row_cpu",
			w:          12,
			h:          8,
			wantPanel:  spec.GridPos{X: 12, Y: 5, W: 12, H: 8},
			wantShifts: []shiftTo{{"row_mem", 13}, {"m1", 14}, {"m2", 14}},
		},
		{
			name:       "case A: a full-width panel starts a new line under the row",
			panels:     caseABoard(),
			rowID:      "row_cpu",
			w:          24,
			h:          8,
			wantPanel:  spec.GridPos{X: 0, Y: 9, W: 24, H: 8},
			wantShifts: []shiftTo{{"row_mem", 17}, {"m1", 18}, {"m2", 18}},
		},
		{
			name:       "case B: a panel wider than the end of the line wraps and leaves the gap",
			panels:     caseBBoard(),
			rowID:      "row_basic",
			w:          8,
			h:          4,
			wantPanel:  spec.GridPos{X: 0, Y: 10, W: 8, H: 4},
			wantShifts: []shiftTo{{"row_cmnd", 14}, {"c1", 15}},
		},
		{
			name:      "case B: a panel that fits the end of the line closes it",
			panels:    caseBBoard(),
			rowID:     "row_basic",
			w:         4,
			h:         4,
			wantPanel: spec.GridPos{X: 20, Y: 6, W: 4, H: 4},
		},
		{
			name:      "case B: after a wrap, a narrow panel goes back to the gap",
			panels:    caseBBoard(placeStat("b4", 0, 10, 8, 4)),
			rowID:     "row_basic",
			w:         4,
			h:         4,
			wantPanel: spec.GridPos{X: 20, Y: 6, W: 4, H: 4},
		},
		{
			name:      "case C: a panel into the row goes beside its first panel",
			panels:    caseCBoard(),
			rowID:     "r_quick",
			w:         6,
			h:         5,
			wantPanel: spec.GridPos{X: 6, Y: 6, W: 6, H: 5},
		},
		{
			name:      "case C: a top level panel goes beside the loose panel",
			panels:    caseCBoard(),
			rowID:     "",
			w:         4,
			h:         5,
			wantPanel: spec.GridPos{X: 4, Y: 0, W: 4, H: 5},
		},
		{
			name:       "case C: a taller top level panel moves the row down by the overrun",
			panels:     caseCBoard(),
			rowID:      "",
			w:          4,
			h:          6,
			wantPanel:  spec.GridPos{X: 4, Y: 0, W: 4, H: 6},
			wantShifts: []shiftTo{{"r_quick", 6}, {"p_mem_busy", 7}},
		},
		{
			name:       "case C: a full-width top level panel goes under the loose one",
			panels:     caseCBoard(),
			rowID:      "",
			w:          24,
			h:          5,
			wantPanel:  spec.GridPos{X: 0, Y: 5, W: 24, H: 5},
			wantShifts: []shiftTo{{"r_quick", 10}, {"p_mem_busy", 11}},
		},
		{
			name:      "case C: a new row goes below everything",
			panels:    caseCBoard(),
			rowID:     "r_basic",
			w:         6,
			h:         5,
			wantPanel: spec.GridPos{X: 0, Y: 12, W: 6, H: 5},
			wantRow:   &spec.GridPos{X: 0, Y: 11, W: 24, H: 1},
		},
		{
			name: "case D: a fifth stat wraps under four",
			panels: []string{
				placeRow("row_quick", 0, false, placeStat("q1", 0, 1, 6, 4), placeStat("q2", 6, 1, 6, 4), placeStat("q3", 12, 1, 6, 4), placeStat("q4", 18, 1, 6, 4)),
				placeRow("row_basic", 5, false, placeStat("b1", 0, 6, 6, 4), placeStat("b2", 6, 6, 6, 4), placeStat("b3", 12, 6, 6, 4), placeStat("b4", 18, 6, 6, 4)),
			},
			rowID:      "row_quick",
			w:          6,
			h:          4,
			wantPanel:  spec.GridPos{X: 0, Y: 5, W: 6, H: 4},
			wantShifts: []shiftTo{{"row_basic", 9}, {"b1", 10}, {"b2", 10}, {"b3", 10}, {"b4", 10}},
		},
		{
			name: "case B Meminfo: a full line wraps in the last row",
			panels: []string{
				placeRow("row_mem", 29, false, placeStat("m1", 0, 30, 12, 8), placeStat("m2", 12, 30, 6, 8), placeStat("m3", 18, 30, 6, 8)),
			},
			rowID:     "row_mem",
			w:         6,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 38, W: 6, H: 8},
		},
		{
			name: "case B Filesystem: the open end of the last line takes the next panel",
			panels: []string{
				placeRow("row_fs", 117, false, placeStat("f1", 0, 118, 12, 8), placeStat("f2", 12, 118, 6, 8), placeStat("f3", 18, 118, 6, 8), placeStat("f4", 0, 126, 12, 8)),
			},
			rowID:     "row_fs",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 12, Y: 126, W: 12, H: 8},
		},
		{
			name: "case A Disk: a third panel wraps and moves the rows below",
			panels: []string{
				placeRow("row_disk", 18, false, placeStat("d1", 0, 19, 12, 8), placeStat("d2", 12, 19, 12, 8)),
				placeRow("row_fs", 27, false, placeStat("f1", 0, 28, 12, 8)),
			},
			rowID:      "row_disk",
			w:          12,
			h:          8,
			wantPanel:  spec.GridPos{X: 0, Y: 27, W: 12, H: 8},
			wantShifts: []shiftTo{{"row_fs", 35}, {"f1", 36}},
		},
		{
			name: "a row whose panels end before the next row starts takes a taller panel without moving it",
			panels: []string{
				placeRow("r1", 0, false, placeStat("b", 12, 1, 12, 4)),
				placeRow("r2", 9, false, placeStat("d1", 0, 10, 12, 8)),
			},
			rowID:     "r1",
			w:         12,
			h:         8,
			wantPanel: spec.GridPos{X: 0, Y: 1, W: 12, H: 8},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := &spec.Dashboard{}
			if len(c.panels) > 0 {
				before = parseBoard(t, c.panels...)
			}
			plan := place(before, []slot{{ID: placedID, Row: c.rowID, RowTitle: c.rowID, W: c.w, H: c.h}})

			if got := plan.Positions[placedID]; got != c.wantPanel {
				t.Errorf("panel = %+v, want %+v", got, c.wantPanel)
			}
			gotRow, hasRow := plan.NewRows[c.rowID]
			switch {
			case c.wantRow == nil && hasRow:
				t.Errorf("row = %+v, want none", gotRow)
			case c.wantRow != nil && (!hasRow || gotRow != *c.wantRow):
				t.Errorf("row = %+v, want %+v", gotRow, *c.wantRow)
			}
			if len(plan.Moves) != len(c.wantShifts) {
				t.Fatalf("shifts = %d, want %d: %+v", len(plan.Moves), len(c.wantShifts), plan.Moves)
			}
			for i, want := range c.wantShifts {
				got := plan.Moves[i]
				old, ok := before.FindPanel(want.id)
				if !ok {
					t.Fatalf("test names unknown panel %q", want.id)
				}
				wantPos := old.Panel.GridPos
				wantPos.Y = want.y
				if got.ID != want.id || string(got.GridPos) != string(gridPosJSON(wantPos)) || got.Row != nil {
					t.Errorf("shift %d = %+v, want %s at %+v", i, got, want.id, wantPos)
				}
			}

			if len(c.panels) == 0 {
				return
			}
			after := applyPlacement(t, before, c.rowID, plan)
			assertExistingPanelsKept(t, before, after, c.rowID)
			placed, _ := after.FindPanel(placedID)
			if placed.Panel.GridPos != c.wantPanel {
				t.Errorf("merged panel = %+v, want %+v", placed.Panel.GridPos, c.wantPanel)
			}
		})
	}
}

func TestPlaceBuildsCaseAInOrder(t *testing.T) {
	board := []string{placeRow("row_cpu", 0, false, placeStat("usage", 0, 1, 12, 8))}
	steps := []struct {
		id   string
		w, h int
		want spec.GridPos
	}{
		{"cores", 4, 4, spec.GridPos{X: 12, Y: 1, W: 4, H: 4}},
		{"load", 8, 4, spec.GridPos{X: 16, Y: 1, W: 8, H: 4}},
	}
	for i, step := range steps {
		before := parseBoard(t, board...)
		plan := place(before, []slot{{ID: step.id, Row: "row_cpu", W: step.w, H: step.h}})
		if got := plan.Positions[step.id]; got != step.want {
			t.Fatalf("step %d: panel = %+v, want %+v", i, got, step.want)
		}
		if len(plan.Moves) != 0 {
			t.Fatalf("step %d: shifts = %+v, want none", i, plan.Moves)
		}
		board = []string{placeRow("row_cpu", 0, false, append(rowChildren(before, "row_cpu"), placeStat(step.id, step.want.X, step.want.Y, step.want.W, step.want.H))...)}
	}
}

func rowChildren(d *spec.Dashboard, rowID string) []string {
	var out []string
	for _, e := range d.AllPanels() {
		if e.Row != nil && e.Row.ID == rowID {
			g := e.Panel.GridPos
			out = append(out, placeStat(e.Panel.ID, g.X, g.Y, g.W, g.H))
		}
	}
	return out
}

func TestPlaceMovesNothingAboveTheInsertion(t *testing.T) {
	before := parseBoard(t,
		placeStat("t", 0, 0, 12, 8),
		placeRow("r1", 8, false, placeStat("c1", 0, 9, 12, 8), placeStat("c2", 12, 9, 12, 8)),
		placeRow("r2", 17, false, placeStat("d1", 0, 18, 12, 8)),
	)
	plan := place(before, []slot{{ID: placedID, Row: "r1", W: 12, H: 8}})
	for _, m := range plan.Moves {
		switch m.ID {
		case "t", "r1", "c1", "c2":
			t.Errorf("panel %q is above the insertion and must not move", m.ID)
		}
	}
	if len(plan.Moves) != 2 {
		t.Errorf("shifts = %+v, want only r2 and d1", plan.Moves)
	}
}

func TestPlaceTreatsAnIdThatIsNotATopLevelRowAsANewRow(t *testing.T) {
	before := parseBoard(t, placeRow("r1", 0, false, placeStat("c1", 0, 1, 12, 8)))
	plan := place(before, []slot{{ID: placedID, Row: "c1", W: 12, H: 8}})
	if got := plan.NewRows["c1"]; got != (spec.GridPos{X: 0, Y: 9, W: 24, H: 1}) {
		t.Fatalf("row = %+v, want a new row at y 9", got)
	}
}

func TestPlaceSlots(t *testing.T) {
	cases := []struct {
		name      string
		panels    []string
		slots     []slot
		wantPos   map[string]spec.GridPos
		wantMoves []shiftTo
		wantRows  map[string]spec.GridPos
	}{
		{
			name:      "resize in place when it fits (case A)",
			panels:    caseABoard(),
			slots:     []slot{{ID: "cores", Row: "row_cpu", W: 4, H: 8, Prefer: &spec.GridPos{X: 12, Y: 1}}},
			wantPos:   map[string]spec.GridPos{"cores": {X: 12, Y: 1, W: 4, H: 8}},
			wantMoves: []shiftTo{{"cores", 1}},
		},
		{
			name:      "resize closes B's gap",
			panels:    caseBBoard(),
			slots:     []slot{{ID: "b3", Row: "row_basic", W: 12, H: 4, Prefer: &spec.GridPos{X: 12, Y: 6}}},
			wantPos:   map[string]spec.GridPos{"b3": {X: 12, Y: 6, W: 12, H: 4}},
			wantMoves: []shiftTo{{"b3", 6}},
		},
		{
			name: "resize that does not fit is placed again",
			panels: []string{
				placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8), placeStat("b", 12, 1, 12, 8)),
				placeRow("r2", 9, false, placeStat("c", 0, 10, 12, 8)),
			},
			slots:     []slot{{ID: "a", Row: "r1", W: 16, H: 8, Prefer: &spec.GridPos{X: 0, Y: 1}}},
			wantPos:   map[string]spec.GridPos{"a": {X: 0, Y: 9, W: 16, H: 8}},
			wantMoves: []shiftTo{{"a", 9}, {"r2", 17}, {"c", 18}},
		},
		{
			name:      "a wanted free spot is used",
			panels:    []string{placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8))},
			slots:     []slot{{ID: "a", Row: "r1", W: 12, H: 8, Prefer: &spec.GridPos{X: 12, Y: 1}}},
			wantPos:   map[string]spec.GridPos{"a": {X: 12, Y: 1, W: 12, H: 8}},
			wantMoves: []shiftTo{{"a", 1}},
		},
		{
			name:    "a wanted occupied spot falls back to first free",
			panels:  []string{placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8), placeStat("b", 12, 1, 12, 8))},
			slots:   []slot{{ID: "n", Row: "r1", W: 12, H: 8, Prefer: &spec.GridPos{X: 0, Y: 1}}},
			wantPos: map[string]spec.GridPos{"n": {X: 0, Y: 9, W: 12, H: 8}},
		},
		{
			name:   "two slots swap",
			panels: []string{placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8), placeStat("b", 12, 1, 12, 8))},
			slots: []slot{
				{ID: "a", Row: "r1", W: 12, H: 8, Prefer: &spec.GridPos{X: 12, Y: 1}},
				{ID: "b", Row: "r1", W: 12, H: 8, Prefer: &spec.GridPos{X: 0, Y: 1}},
			},
			wantPos:   map[string]spec.GridPos{"a": {X: 12, Y: 1, W: 12, H: 8}, "b": {X: 0, Y: 1, W: 12, H: 8}},
			wantMoves: []shiftTo{{"a", 1}, {"b", 1}},
		},
		{
			name: "a wanted y outside the row is ignored",
			panels: []string{
				placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8)),
				placeRow("r2", 9, false, placeStat("c", 0, 10, 12, 8)),
			},
			slots:   []slot{{ID: "a", Row: "r1", W: 12, H: 8, Prefer: &spec.GridPos{X: 0, Y: 30}}},
			wantPos: map[string]spec.GridPos{"a": {X: 0, Y: 1, W: 12, H: 8}},
		},
		{
			name:      "a move to another row",
			panels:    caseCBoard(),
			slots:     []slot{{ID: "p_cpu_busy", Row: "r_quick", W: 4, H: 5}},
			wantPos:   map[string]spec.GridPos{"p_cpu_busy": {X: 6, Y: 6, W: 4, H: 5}},
			wantMoves: []shiftTo{{"p_cpu_busy", 6}},
		},
		{
			name:      "a move to a new row",
			panels:    caseCBoard(),
			slots:     []slot{{ID: "p_mem_busy", Row: "r_new", RowTitle: "New", W: 6, H: 5}},
			wantPos:   map[string]spec.GridPos{"p_mem_busy": {X: 0, Y: 7, W: 6, H: 5}},
			wantMoves: []shiftTo{{"p_mem_busy", 7}},
			wantRows:  map[string]spec.GridPos{"r_new": {X: 0, Y: 6, W: 24, H: 1}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := parseBoard(t, c.panels...)
			plan := place(before, c.slots)

			for id, pos := range c.wantPos {
				if got := plan.Positions[id]; got != pos {
					t.Errorf("%s = %+v, want %+v", id, got, pos)
				}
			}
			if len(plan.Moves) != len(c.wantMoves) {
				t.Fatalf("moves = %+v, want %d", plan.Moves, len(c.wantMoves))
			}
			for i, m := range c.wantMoves {
				if got := plan.Moves[i]; got.ID != m.id {
					t.Errorf("move %d = %s, want %s", i, got.ID, m.id)
				}
			}
			if len(plan.NewRows) != len(c.wantRows) {
				t.Errorf("new rows = %+v, want %+v", plan.NewRows, c.wantRows)
			}
			for id, pos := range c.wantRows {
				if got := plan.NewRows[id]; got != pos {
					t.Errorf("new row %s = %+v, want %+v", id, got, pos)
				}
			}
			if hasNewSlot(before, c.slots) || len(plan.Moves) == 0 {
				return
			}
			var patch spec.PanelPatch
			for _, s := range c.slots {
				if pos, ok := plan.NewRows[s.Row]; ok {
					patch.Upsert = append(patch.Upsert, json.RawMessage(fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":%d}}`, s.Row, s.RowTitle, pos.X, pos.Y, pos.W, pos.H)))
				}
			}
			patch.Layout = plan.Moves
			after, err := spec.ApplyPanelPatch(before, patch)
			if err != nil {
				t.Fatalf("the moves do not merge: %v", err)
			}
			for id, pos := range c.wantPos {
				if e, ok := after.FindPanel(id); !ok || e.Panel.GridPos != pos {
					t.Errorf("merged %s = %+v, want %+v", id, e.Panel.GridPos, pos)
				}
			}
		})
	}
}

func hasNewSlot(d *spec.Dashboard, slots []slot) bool {
	for _, s := range slots {
		if _, ok := d.FindPanel(s.ID); !ok {
			return true
		}
	}
	return false
}

func TestGaps(t *testing.T) {
	cases := []struct {
		name   string
		panels []string
		want   []gap
	}{
		{"case A: the space under a short pair", caseABoard(), []gap{{"row_cpu", spec.GridPos{X: 12, Y: 5, W: 12, H: 4}}}},
		{"case B: the end of a line left behind by a wrap", caseBBoard(placeStat("b4", 0, 10, 8, 4)), []gap{{"row_basic", spec.GridPos{X: 20, Y: 6, W: 4, H: 4}}}},
		{"case B: still there after the line below fills", caseBBoard(placeStat("b4", 0, 10, 8, 4), placeStat("b5", 8, 10, 8, 4), placeStat("b6", 16, 10, 8, 4)), []gap{{"row_basic", spec.GridPos{X: 20, Y: 6, W: 4, H: 4}}}},
		{"case B: the open end of the last line is no gap", caseBBoard(), nil},
		{"case C: a loose panel beside nothing", caseCBoard(), nil},
		{
			name: "case D: a fifth stat under four",
			panels: []string{
				placeRow("row_quick", 0, false, placeStat("q1", 0, 1, 6, 4), placeStat("q2", 6, 1, 6, 4), placeStat("q3", 12, 1, 6, 4), placeStat("q4", 18, 1, 6, 4), placeStat("q5", 0, 5, 6, 4)),
				placeRow("row_basic", 9, false, placeStat("b1", 0, 10, 6, 4)),
			},
		},
		{
			name:   "case B Meminfo: one panel on the second line",
			panels: []string{placeRow("row_mem", 29, false, placeStat("m1", 0, 30, 12, 8), placeStat("m2", 12, 30, 6, 8), placeStat("m3", 18, 30, 6, 8), placeStat("m4", 0, 38, 6, 8))},
		},
		{
			name:   "case B Filesystem: the open end of the last line",
			panels: []string{placeRow("row_fs", 117, false, placeStat("f1", 0, 118, 12, 8), placeStat("f2", 12, 118, 6, 8), placeStat("f3", 18, 118, 6, 8), placeStat("f4", 0, 126, 12, 8))},
		},
		{
			name: "case A Disk: a third panel on its own line",
			panels: []string{
				placeRow("row_disk", 18, false, placeStat("d1", 0, 19, 12, 8), placeStat("d2", 12, 19, 12, 8), placeStat("d3", 0, 27, 12, 8)),
				placeRow("row_fs", 35, false, placeStat("f1", 0, 36, 12, 8)),
			},
		},
		{"a collapsed row with a hole", []string{placeRow("r1", 0, true, placeStat("a", 0, 1, 12, 8), placeStat("b", 12, 1, 12, 4))}, nil},
		{"a top level panel with an empty band above it", []string{placeStat("a", 0, 2, 12, 8)}, []gap{{"", spec.GridPos{X: 0, Y: 0, W: 24, H: 2}}}},
		{"a top level panel under rows leaves no gap where the rows are", []string{placeRow("r1", 0, false, placeStat("a", 0, 1, 12, 8)), placeStat("t", 0, 9, 12, 8)}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := gaps(parseBoard(t, c.panels...))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("gaps = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestPlaceSizesNewPanels(t *testing.T) {
	t.Run("a stat beside a chart and the next one after it", func(t *testing.T) {
		board := []string{placeRow("row_cpu", 0, false, placeSeries("usage", 0, 1, 12, 8))}
		before := parseBoard(t, board...)
		plan := place(before, []slot{{ID: placedID, Type: spec.Stat, Row: "row_cpu"}})
		if got, want := plan.Positions[placedID], (spec.GridPos{X: 12, Y: 1, W: 6, H: 8}); got != want {
			t.Fatalf("first stat = %+v, want %+v", got, want)
		}
		applyPlacement(t, before, "row_cpu", plan)

		board = []string{placeRow("row_cpu", 0, false, placeSeries("usage", 0, 1, 12, 8), placeStat("first", 12, 1, 6, 8))}
		plan = place(parseBoard(t, board...), []slot{{ID: placedID, Type: spec.Stat, Row: "row_cpu"}})
		if got, want := plan.Positions[placedID], (spec.GridPos{X: 18, Y: 1, W: 6, H: 8}); got != want {
			t.Errorf("second stat = %+v, want %+v", got, want)
		}
	})
	t.Run("a strip of four in a new row", func(t *testing.T) {
		var stats []string
		for i, want := range []spec.GridPos{{X: 0, Y: 1, W: 4, H: 4}, {X: 4, Y: 1, W: 4, H: 4}, {X: 8, Y: 1, W: 4, H: 4}, {X: 12, Y: 1, W: 4, H: 4}} {
			s := slot{ID: placedID, Type: spec.Stat, Row: "row_new"}
			if i == 0 {
				s.W = 4
			}
			before := &spec.Dashboard{}
			if i > 0 {
				before = parseBoard(t, placeRow("row_new", 0, false, stats...))
			}
			plan := place(before, []slot{s})
			if got := plan.Positions[placedID]; got != want {
				t.Fatalf("stat %d = %+v, want %+v", i, got, want)
			}
			stats = append(stats, placeStat(fmt.Sprintf("s%d", i), want.X, want.Y, want.W, want.H))
		}
	})
	t.Run("a full-width chart into an empty section", func(t *testing.T) {
		plan := place(&spec.Dashboard{}, []slot{{ID: placedID, Type: spec.Timeseries, W: spec.GridColumns}})
		if got, want := plan.Positions[placedID], (spec.GridPos{X: 0, Y: 0, W: 24, H: 7}); got != want {
			t.Errorf("chart = %+v, want %+v", got, want)
		}
	})
}

func TestOpenLine(t *testing.T) {
	tests := []struct {
		name    string
		panels  []string
		section string
		want    openLine
	}{
		{"an empty section", []string{placeRow("r", 0, false)}, "r", openLine{}},
		{"a full line", []string{placeRow("r", 0, false, placeSeries("a", 0, 1, 12, 8), placeSeries("b", 12, 1, 12, 8))}, "r", openLine{}},
		{"half a line", []string{placeRow("r", 0, false, placeSeries("usage", 0, 1, 12, 8))}, "r", openLine{X: 12, Y: 1, W: 12, H: 8, Type: spec.Timeseries, LastW: 12}},
		{"a stacked section ending at column 24", []string{placeRow("r", 0, false, placeSeries("a", 0, 1, 12, 4), placeSeries("b", 12, 1, 12, 4), placeSeries("c", 0, 5, 12, 4), placeSeries("d", 12, 5, 12, 4))}, "r", openLine{}},
		{"the last panel is the lowest and rightmost", []string{placeRow("r", 0, false, placeStat("a", 18, 1, 6, 4), placeStat("b", 0, 5, 6, 4), placeStat("c", 6, 5, 6, 4))}, "r", openLine{X: 12, Y: 5, W: 12, H: 4, Type: spec.Stat, LastW: 6}},
		{"the top level ignores rows", []string{placeSeries("t", 0, 0, 8, 8), placeRow("r", 8, false, placeSeries("a", 0, 9, 12, 8))}, "", openLine{X: 8, Y: 0, W: 16, H: 8, Type: spec.Timeseries, LastW: 8}},
		{"a collapsed row's children count", []string{placeRow("r", 0, true, placeSeries("a", 0, 1, 12, 8))}, "r", openLine{X: 12, Y: 1, W: 12, H: 8, Type: spec.Timeseries, LastW: 12}},
		{"an unknown section", []string{placeRow("r", 0, false, placeSeries("a", 0, 1, 12, 8))}, "other", openLine{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newWorkingLayout(parseBoard(t, tt.panels...)).openLine(tt.section); got != tt.want {
				t.Errorf("open line = %+v, want %+v", got, tt.want)
			}
		})
	}
}
