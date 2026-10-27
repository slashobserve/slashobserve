package queryproxy

import (
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestPanelRange(t *testing.T) {
	dashboard := testRange()
	at := testNow.Add
	tests := []struct {
		name  string
		panel spec.Panel
		want  datasource.TimeRange
	}{
		{"neither keeps the range", spec.Panel{}, dashboard},
		{"timeFrom replaces the range", spec.Panel{TimeFrom: "now-1h"}, datasource.TimeRange{From: at(-time.Hour), To: testNow}},
		{"timeFrom in days", spec.Panel{TimeFrom: "now-2d"}, datasource.TimeRange{From: at(-48 * time.Hour), To: testNow}},
		{"timeShift moves the range back", spec.Panel{TimeShift: "1d"}, datasource.TimeRange{From: at(-30 * time.Hour), To: at(-24 * time.Hour)}},
		{"both shift the replaced range", spec.Panel{TimeFrom: "now-1h", TimeShift: "1d"}, datasource.TimeRange{From: at(-25 * time.Hour), To: at(-24 * time.Hour)}},
		{"large timeFrom is not capped here", spec.Panel{TimeFrom: "now-500d"}, datasource.TimeRange{From: at(-500 * 24 * time.Hour), To: testNow}},
		{"unresolvable timeFrom leaves the range", spec.Panel{TimeFrom: "later"}, dashboard},
		{"unresolvable timeShift leaves the range", spec.Panel{TimeShift: "later"}, dashboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PanelRange(&tt.panel, dashboard, testNow); got != tt.want {
				t.Fatalf("PanelRange = %v, want %v", got, tt.want)
			}
		})
	}
}
