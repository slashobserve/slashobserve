package grafana

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func gridOf(t *testing.T, res *Result, id string) spec.GridPos {
	t.Helper()
	for _, e := range res.Spec.AllPanels() {
		if e.Panel.ID == id {
			return e.Panel.GridPos
		}
	}
	t.Fatalf("no panel %q in the Spec", id)
	return spec.GridPos{}
}

func TestCompactionClosesGapsWithoutMovedNotes(t *testing.T) {
	res := convertDoc(t, dashboardJSON("", panelJSON(1, 0, 0, 12, 8, ""), panelJSON(2, 0, 20, 12, 8, ""), panelJSON(3, 12, 25, 12, 4, "")))
	if got := gridOf(t, res, "p2"); got.Y != 8 {
		t.Fatalf("p2 y = %d, want 8", got.Y)
	}
	if got := gridOf(t, res, "p3"); got.Y != 0 || got.X != 12 {
		t.Fatalf("p3 = %+v, want it pulled up to y 0", got)
	}
	for _, line := range res.Report.Panels {
		if isMoved(line) {
			t.Fatalf("%s is marked moved by plain compaction", line.PanelID)
		}
	}
}

func TestOverlappingPanelsAreStackedWithoutMovedNotes(t *testing.T) {
	res := convertDoc(t, dashboardJSON("", panelJSON(1, 0, 0, 12, 8, ""), panelJSON(2, 6, 4, 12, 8, "")))
	if got := gridOf(t, res, "p2"); got.Y != 8 {
		t.Fatalf("p2 y = %d, want 8 below p1", got.Y)
	}
	if isMoved(panelLine(t, res, "p2")) {
		t.Fatal("p2 is marked moved though compaction alone stacks it")
	}
}

func TestPanelsFarDownTheGridStillConvert(t *testing.T) {
	res := convertDoc(t, dashboardJSON("", panelJSON(1, 0, 9999, 12, 8, ""), panelJSON(2, 0, 9999, 12, 8, "")))
	if !res.Ready() {
		t.Fatal("the dashboard is not ready")
	}
	if a, b := gridOf(t, res, "p1"), gridOf(t, res, "p2"); a.Y != 0 || b.Y != 8 {
		t.Fatalf("y = %d and %d, want 0 and 8", a.Y, b.Y)
	}
	for _, line := range res.Report.Panels {
		if isMoved(line) {
			t.Fatalf("%s is marked moved", line.PanelID)
		}
	}
}

func TestClampedPanelsAreReportedAsMoved(t *testing.T) {
	res := convertDoc(t, dashboardJSON("", panelJSON(1, 0, 0, 12, 8, ""), panelJSON(2, 20, 0, 8, 8, ""), panelJSON(3, 0, 8, 12, 200, "")))
	if got := gridOf(t, res, "p2"); got.X != 20 || got.W != 4 {
		t.Fatalf("p2 = %+v, want w cut to 4", got)
	}
	moved := panelLine(t, res, "p2")
	if !isMoved(moved) || len(moved.Paths) != 1 || moved.Paths[0] != "gridPos" {
		t.Fatalf("p2 line = %+v, want a moved reason on gridPos", moved)
	}
	if got := gridOf(t, res, "p3"); got.H != 100 || !isMoved(panelLine(t, res, "p3")) {
		t.Fatalf("p3 = %+v, want h cut to 100 and reported", got)
	}
	if isMoved(panelLine(t, res, "p1")) {
		t.Fatal("p1 was not changed")
	}
}

func TestCollapsedRowChildrenAreRebasedUnderTheirRow(t *testing.T) {
	row := rowJSON(10, 5, true, `"panels":[`+panelJSON(11, 0, 105, 12, 4, "")+","+panelJSON(12, 12, 106, 12, 4, "")+","+panelJSON(13, 0, 130, 12, 4, "")+`]`)
	res := convertDoc(t, dashboardJSON("", panelJSON(1, 0, 0, 24, 3, ""), row))
	if got := gridOf(t, res, "p10"); got.Y != 3 {
		t.Fatalf("row y = %d, want 3 after compaction", got.Y)
	}
	want := map[string]int{"p11": 4, "p12": 4, "p13": 8}
	for id, y := range want {
		if got := gridOf(t, res, id); got.Y != y {
			t.Errorf("%s y = %d, want %d", id, got.Y, y)
		}
		if isMoved(panelLine(t, res, id)) {
			t.Errorf("%s is marked moved by rebasing and compaction", id)
		}
	}
}

func TestCollapsedRowChildrenAreCompactedSeparately(t *testing.T) {
	row := rowJSON(10, 0, true, `"panels":[`+panelJSON(11, 0, 1, 24, 6, "")+","+panelJSON(12, 0, 1, 24, 6, "")+`]`)
	res := convertDoc(t, dashboardJSON("", row, panelJSON(1, 0, 1, 24, 6, "")))
	if a, b := gridOf(t, res, "p11"), gridOf(t, res, "p12"); a.Y != 1 || b.Y != 7 {
		t.Fatalf("children y = %d and %d, want 1 and 7 under the row", a.Y, b.Y)
	}
	if got := gridOf(t, res, "p1"); got.Y != 1 {
		t.Fatalf("p1 y = %d, want 1: collapsed children take no room", got.Y)
	}
}

func TestPanelsWithoutGridPosUseGrafanasDefaultSize(t *testing.T) {
	res := convertDoc(t, `{"title":"T","panels":[{"id":1,"type":"text","title":"A"},{"id":2,"type":"text","title":"B"}]}`)
	if a, b := gridOf(t, res, "p1"), gridOf(t, res, "p2"); a != (spec.GridPos{X: 0, Y: 0, W: 6, H: 3}) || b != (spec.GridPos{X: 0, Y: 3, W: 6, H: 3}) {
		t.Fatalf("positions = %+v and %+v, want 6x3 panels stacked", a, b)
	}
	for _, line := range res.Report.Panels {
		if isMoved(line) {
			t.Fatalf("%s is marked moved", line.PanelID)
		}
	}
}

func TestATallDashboardAlwaysConverts(t *testing.T) {
	var panels []string
	for i := 1; i <= 150; i++ {
		panels = append(panels, panelJSON(i, 0, (i-1)*100, 24, 100, ""))
	}
	res := convertDoc(t, dashboardJSON("", panels...))
	if !res.Ready() || len(res.Report.Panels) != 150 {
		t.Fatalf("ready = %v, panels = %d", res.Ready(), len(res.Report.Panels))
	}
	last := gridOf(t, res, "p150")
	if last.Y > maxGridY {
		t.Fatalf("last y = %d, above the grid limit", last.Y)
	}
	if !strings.Contains(fmt.Sprint(panelLine(t, res, "p150").Reasons), movedReason) {
		t.Fatal("the panels that were shrunk to fit are not reported")
	}
}

// Expected positions come from react-grid-layout's verticalCompactor on the same input.
func TestOverlapCascadeMatchesReactGridLayout(t *testing.T) {
	res := convertDoc(t, dashboardJSON("",
		panelJSON(1, 0, 0, 12, 4, ""),
		panelJSON(2, 6, 2, 12, 4, ""),
		panelJSON(3, 12, 5, 12, 4, ""),
		panelJSON(4, 0, 6, 24, 2, ""),
	))
	want := map[string]spec.GridPos{
		"p1": {X: 0, Y: 0, W: 12, H: 4},
		"p2": {X: 6, Y: 4, W: 12, H: 4},
		"p3": {X: 12, Y: 8, W: 12, H: 4},
		"p4": {X: 0, Y: 12, W: 24, H: 2},
	}
	for id, grid := range want {
		if got := gridOf(t, res, id); got != grid {
			t.Errorf("%s = %+v, want %+v", id, got, grid)
		}
		if isMoved(panelLine(t, res, id)) {
			t.Errorf("%s is marked moved by plain compaction", id)
		}
	}
}

func TestHugeGridPositionsConvertQuickly(t *testing.T) {
	const huge = 1000000000
	panels := []string{panelJSON(1, 0, 0, 1, huge, "")}
	for i := 2; i <= 8; i++ {
		panels = append(panels, panelJSON(i, i-1, huge, 1, 1, ""))
	}
	doc := dashboardJSON("", panels...)
	done := make(chan *Result, 1)
	go func() {
		res, err := Convert([]byte(doc), Options{Datasources: testDatasources})
		if err != nil {
			t.Errorf("Convert: %v", err)
		}
		done <- res
	}()
	select {
	case res := <-done:
		if got := gridOf(t, res, "p2"); got.Y != 0 || got.X != 1 {
			t.Fatalf("p2 = %+v, want it pulled up to y 0", got)
		}
		if got := gridOf(t, res, "p1"); got.H != maxGridHeight || !isMoved(panelLine(t, res, "p1")) {
			t.Fatalf("p1 = %+v, want h cut to %d and reported", got, maxGridHeight)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Convert still running after 2s on huge grid positions")
	}
}
