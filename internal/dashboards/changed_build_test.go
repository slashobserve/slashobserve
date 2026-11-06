package dashboards

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	testRows        = 4
	testRowChildren = 10
	testRowHeight   = 13
)

func rowedStat(id string, x, y, h int) string {
	return fmt.Sprintf(`{"id":%q,"type":"stat","title":%q,"gridPos":{"x":%d,"y":%d,"w":6,"h":%d},"datasource":"prom","queries":[{"query":"up"}]}`, id, id, x, y, h)
}

func rowedDashboard(t *testing.T, extraRow int, extra string, extraHeight int) *spec.Dashboard {
	t.Helper()
	var rows []string
	shift := 0
	for r := range testRows {
		top := r*testRowHeight + shift
		var children []string
		for j := range testRowChildren {
			children = append(children, rowedStat(fmt.Sprintf("r%dp%d", r, j), j%4*6, top+1+j/4*4, 4))
		}
		if r == extraRow && extra != "" {
			children = append(children, rowedStat(extra, 0, top+testRowHeight, extraHeight))
			shift = extraHeight
		}
		rows = append(rows, fmt.Sprintf(`{"id":"row%d","type":"row","title":"Row %d","gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[%s]}`, r, r, top, strings.Join(children, ",")))
	}
	d, err := spec.Parse([]byte(`{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[` + strings.Join(rows, ",") + `]}`))
	if err != nil {
		t.Fatalf("parse rowed dashboard: %v", err)
	}
	return d
}

func TestChangedSinceIgnoresPanelsShiftedDownByAnInsertion(t *testing.T) {
	prev := rowedDashboard(t, 1, "", 0)
	next := rowedDashboard(t, 1, "added", 8)
	if got := next.PanelCount(); got != prev.PanelCount()+1 {
		t.Fatalf("next has %d panels, want %d", got, prev.PanelCount()+1)
	}
	moved, ok := next.FindPanel("r2p0")
	if !ok {
		t.Fatal("panel r2p0 missing")
	}
	was, _ := prev.FindPanel("r2p0")
	if moved.Panel.GridPos.Y != was.Panel.GridPos.Y+8 {
		t.Fatalf("r2p0 moved to y %d from %d, want 8 lower", moved.Panel.GridPos.Y, was.Panel.GridPos.Y)
	}

	panels, variables := ChangedSince(prev, next)
	if !slices.Equal(panels, []string{"added"}) || len(variables) != 0 {
		t.Fatalf("changed panels %v variables %v, want [added] and none", panels, variables)
	}
}
