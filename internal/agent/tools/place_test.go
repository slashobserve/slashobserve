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
