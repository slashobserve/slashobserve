package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const placedID = "new"

func placeStat(id string, x, y, w, h int) string {
	return fmt.Sprintf(`{"id":%q,"type":"stat","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":%d},"datasource":%q,"queries":[{"query":"up"}]}`, id, id, x, y, w, h, fakeUID)
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

func applyPlacement(t *testing.T, d *spec.Dashboard, rowID string, pl placement) *spec.Dashboard {
	t.Helper()
	panel := placeStat(placedID, pl.Panel.X, pl.Panel.Y, pl.Panel.W, pl.Panel.H)
	existing := false
	for _, p := range d.Panels {
		existing = existing || (p.Type == spec.Row && p.ID == rowID)
	}
	patch := spec.PanelPatch{Layout: pl.Shifts}
	switch {
	case pl.Row != nil:
		row := fmt.Sprintf(`{"id":%q,"type":"row","title":%q,"gridPos":{"x":%d,"y":%d,"w":%d,"h":%d},"panels":[%s]}`, rowID, rowID, pl.Row.X, pl.Row.Y, pl.Row.W, pl.Row.H, panel)
		patch.Upsert = []json.RawMessage{json.RawMessage(row)}
	case existing:
		patch.Upsert = []json.RawMessage{json.RawMessage(panel)}
		patch.Layout = append([]spec.PanelPlacement{{ID: placedID, Row: &rowID}}, pl.Shifts...)
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
			pl := place(before, c.rowID, c.w, c.h)

			if pl.Panel != c.wantPanel {
				t.Errorf("panel = %+v, want %+v", pl.Panel, c.wantPanel)
			}
			switch {
			case c.wantRow == nil && pl.Row != nil:
				t.Errorf("row = %+v, want none", *pl.Row)
			case c.wantRow != nil && (pl.Row == nil || *pl.Row != *c.wantRow):
				t.Errorf("row = %v, want %+v", pl.Row, *c.wantRow)
			}
			if len(pl.Shifts) != len(c.wantShifts) {
				t.Fatalf("shifts = %d, want %d: %+v", len(pl.Shifts), len(c.wantShifts), pl.Shifts)
			}
			for i, want := range c.wantShifts {
				got := pl.Shifts[i]
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
			after := applyPlacement(t, before, c.rowID, pl)
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
		w, h int
		want spec.GridPos
	}{
		{4, 4, spec.GridPos{X: 12, Y: 1, W: 4, H: 4}},
		{8, 4, spec.GridPos{X: 16, Y: 1, W: 8, H: 4}},
	}
	ids := []string{"cores", "load"}
	for i, step := range steps {
		before := parseBoard(t, board...)
		pl := place(before, "row_cpu", step.w, step.h)
		if pl.Panel != step.want {
			t.Fatalf("step %d: panel = %+v, want %+v", i, pl.Panel, step.want)
		}
		if len(pl.Shifts) != 0 {
			t.Fatalf("step %d: shifts = %+v, want none", i, pl.Shifts)
		}
		board = []string{placeRow("row_cpu", 0, false, append(rowChildren(before, "row_cpu"), placeStat(ids[i], pl.Panel.X, pl.Panel.Y, pl.Panel.W, pl.Panel.H))...)}
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
	pl := place(before, "r1", 12, 8)
	for _, s := range pl.Shifts {
		switch s.ID {
		case "t", "r1", "c1", "c2":
			t.Errorf("panel %q is above the insertion and must not move", s.ID)
		}
	}
	if len(pl.Shifts) != 2 {
		t.Errorf("shifts = %+v, want only r2 and d1", pl.Shifts)
	}
}

func TestPlaceTreatsAnIdThatIsNotATopLevelRowAsANewRow(t *testing.T) {
	before := parseBoard(t, placeRow("r1", 0, false, placeStat("c1", 0, 1, 12, 8)))
	pl := place(before, "c1", 12, 8)
	if pl.Row == nil || *pl.Row != (spec.GridPos{X: 0, Y: 9, W: 24, H: 1}) {
		t.Fatalf("row = %v, want a new row at y 9", pl.Row)
	}
}
