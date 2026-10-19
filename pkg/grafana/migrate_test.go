package grafana

import (
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestLegacyPanelWithoutUsableHeightTakesTheRowHeight(t *testing.T) {
	res := convertDoc(t, `{"title":"T","schemaVersion":12,"rows":[{"height":"250px","panels":[
		{"id":1,"type":"text","span":6,"height":0},
		{"id":2,"type":"text","span":6,"height":"380px"},
		{"id":3,"type":"text","span":6},
		{"id":4,"type":"text","span":6,"height":"tall"}
	]}]}`)
	want := map[string]spec.GridPos{
		"p1": {X: 0, Y: 0, W: 12, H: 7},
		"p2": {X: 12, Y: 0, W: 12, H: 10},
		"p3": {X: 0, Y: 7, W: 12, H: 7},
		"p4": {X: 12, Y: 10, W: 12, H: 7},
	}
	for id, grid := range want {
		if got := gridOf(t, res, id); got != grid {
			t.Errorf("%s = %+v, want %+v", id, got, grid)
		}
	}
}

func TestRepeatClonesNeedARepeatMarker(t *testing.T) {
	res := convertDoc(t, dashboardJSON("",
		panelJSON(1, 0, 0, 12, 4, `"repeatIteration":1700000000000`),
		panelJSON(2, 12, 0, 12, 4, `"repeat":"host","repeatIteration":1700000000000`),
		panelJSON(3, 0, 4, 12, 4, `"repeatPanelId":2`),
		panelJSON(4, 12, 4, 12, 4, `"repeatedByRow":true`),
	))
	for _, id := range []string{"p1", "p2"} {
		if !hasLine(res, id) {
			t.Errorf("%s was dropped though nothing marks it as a clone", id)
		}
	}
	for _, id := range []string{"p3", "p4"} {
		if hasLine(res, id) {
			t.Errorf("%s is a repeat clone and should be skipped", id)
		}
	}
	notes := notesFor(res, "repeated panels")
	if len(notes) != 1 || !hasPath(notes, "panels[2]") || !hasPath(notes, "panels[3]") || hasPath(notes, "panels[0]") {
		t.Fatalf("repeated panels notes = %+v", notes)
	}
}

func TestLegacyRowsUseRepeatIterationToFindClones(t *testing.T) {
	res := convertDoc(t, `{"title":"T","schemaVersion":12,"rows":[
		{"title":"Host","repeat":"host","repeatIteration":5,"panels":[{"id":1,"type":"text","span":12}]},
		{"title":"Host","repeatIteration":5,"panels":[{"id":2,"type":"text","span":12}]}
	]}`)
	if !hasLine(res, "p1") || hasLine(res, "p2") {
		t.Fatalf("panel lines = %+v, want only p1; notes %+v", res.Report.Panels, res.Report.Notes)
	}
	if notes := notesFor(res, "repeated panels"); len(notes) != 1 || !hasPath(notes, "rows[1]") || hasPath(notes, "rows[0]") {
		t.Fatalf("repeated panels notes = %+v", notes)
	}
}

func TestOpenRowIgnoresLeftoverPanels(t *testing.T) {
	stale := panelJSON(7, 0, 1, 12, 4, "")
	doc := dashboardJSON("", rowJSON(5, 0, false, `"panels":[`+stale+`]`), panelJSON(8, 0, 1, 12, 4, ""))
	res := convertDoc(t, doc)
	if hasLine(res, "p7") || !hasLine(res, "p8") {
		t.Fatalf("panel lines = %+v, want only p8", res.Report.Panels)
	}
	if notes := notesFor(res, "rows"); len(notes) != 1 || !hasPath(notes, "panels[0].panels") {
		t.Fatalf("rows notes = %+v", notes)
	}
	if summary, err := Detect([]byte(doc)); err != nil || summary.Panels != 1 {
		t.Fatalf("summary = %+v, err = %v, want 1 panel", summary, err)
	}
}
