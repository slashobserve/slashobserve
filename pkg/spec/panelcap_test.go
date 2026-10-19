package spec_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func dashboardWithPanelCount(t *testing.T, total int) []byte {
	t.Helper()
	children := total / 4
	stat := func(i int) spec.Panel {
		return spec.Panel{
			ID:         fmt.Sprintf("p%d", i),
			Type:       spec.Stat,
			Title:      fmt.Sprintf("Panel %d", i),
			GridPos:    spec.GridPos{X: 0, Y: i, W: spec.GridColumns, H: 1},
			Datasource: "ch",
			Queries:    []spec.Query{{Query: "SELECT count() FROM otel_logs WHERE $__timeFilter(Timestamp)"}},
		}
	}
	row := spec.Panel{
		ID:      "r1",
		Type:    spec.Row,
		Title:   "Row",
		GridPos: spec.GridPos{X: 0, Y: 0, W: spec.GridColumns, H: 1},
	}
	for i := range children {
		row.Panels = append(row.Panels, stat(i+1))
	}
	panels := []spec.Panel{row}
	for i := children + 1; i < total; i++ {
		panels = append(panels, stat(i))
	}
	data, err := json.Marshal(spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       "Panel cap",
		Time:        spec.TimeRange{From: "now-15m", To: "now"},
		Panels:      panels,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPanelCap(t *testing.T) {
	cases := []struct {
		name      string
		total     int
		wantIssue bool
	}{
		{"at the cap", spec.MaxPanels, false},
		{"over the cap", spec.MaxPanels + 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := spec.Parse(dashboardWithPanelCount(t, tc.total))
			if !tc.wantIssue {
				if err != nil {
					t.Fatalf("Parse error = %v, want none", err)
				}
				if got := d.PanelCount(); got != tc.total {
					t.Fatalf("PanelCount = %d, want %d", got, tc.total)
				}
				return
			}
			var ve *spec.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Parse error = %v, want *spec.ValidationError", err)
			}
			if len(ve.Issues) != 1 || ve.Issues[0].Code != spec.CodeTooManyPanels || ve.Issues[0].Path != "/panels" {
				t.Fatalf("issues = %+v, want one %s at /panels", ve.Issues, spec.CodeTooManyPanels)
			}
		})
	}
}
