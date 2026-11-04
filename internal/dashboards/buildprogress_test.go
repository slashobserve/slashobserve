package dashboards

import (
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func gridPanel(id string, typ spec.PanelType, y, h int, children ...spec.Panel) spec.Panel {
	return spec.Panel{ID: id, Type: typ, GridPos: spec.GridPos{Y: y, W: 6, H: h}, Panels: children}
}

func TestBuildProgressCountsEveryNonRowPanel(t *testing.T) {
	tests := []struct {
		name string
		d    *spec.Dashboard
		want int
	}{
		{"an empty dashboard", &spec.Dashboard{}, 0},
		{"top-level panels", &spec.Dashboard{Panels: []spec.Panel{gridPanel("a", spec.Stat, 0, 4), gridPanel("b", spec.Stat, 4, 4)}}, 2},
		{"row children count and the row does not", &spec.Dashboard{Panels: []spec.Panel{
			gridPanel("cpu", spec.Row, 0, 1, gridPanel("a", spec.Stat, 1, 4), gridPanel("b", spec.Timeseries, 1, 8)),
		}}, 2},
		{"an empty row", &spec.Dashboard{Panels: []spec.Panel{gridPanel("cpu", spec.Row, 0, 1)}}, 0},
		{"rows and panels together", &spec.Dashboard{Panels: []spec.Panel{
			gridPanel("top", spec.Stat, 0, 4),
			gridPanel("cpu", spec.Row, 4, 1, gridPanel("a", spec.Stat, 5, 4)),
			gridPanel("mem", spec.Row, 9, 1, gridPanel("b", spec.Stat, 10, 4), gridPanel("c", spec.Stat, 10, 4)),
		}}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProgress(tt.d, 7)
			if got.Panels != tt.want || got.PlannedPanels != 7 {
				t.Fatalf("progress = %+v, want %d panels and 7 planned", got, tt.want)
			}
		})
	}
}

func TestPendingIsThePlannedPanelsNotYetBuilt(t *testing.T) {
	tests := []struct {
		name string
		p    BuildProgress
		want int
	}{
		{"some left", BuildProgress{Panels: 37, PlannedPanels: 114}, 77},
		{"none planned", BuildProgress{Panels: 5}, 0},
		{"all built", BuildProgress{Panels: 5, PlannedPanels: 5}, 0},
		{"built above planned", BuildProgress{Panels: 9, PlannedPanels: 5}, 0},
		{"nothing built", BuildProgress{PlannedPanels: 3}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Pending(); got != tt.want {
				t.Fatalf("Pending() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPlannedPanelsMustBeOneToTwoHundred(t *testing.T) {
	for n, valid := range map[int]bool{-1: false, 0: false, 1: true, 114: true, 200: true, 201: false} {
		issues := validatePlannedPanels(n)
		if valid != (len(issues) == 0) {
			t.Errorf("validatePlannedPanels(%d) = %v, valid = %v", n, issues, valid)
		}
		if !valid && (issues[0].Path != "/planned_panels" || issues[0].Code != spec.CodeSchema || issues[0].Message != "planned_panels must be between 1 and 200") {
			t.Errorf("validatePlannedPanels(%d) issue = %+v", n, issues[0])
		}
	}
}

func TestStoppedSummary(t *testing.T) {
	tests := []struct {
		name string
		p    BuildProgress
		want string
	}{
		{"planned", BuildProgress{Panels: 41, PlannedPanels: 114}, "Saved what the agent built before it stopped: 41 of 114 panels"},
		{"all planned built", BuildProgress{Panels: 3, PlannedPanels: 3}, "Saved what the agent built before it stopped: 3 of 3 panels"},
		{"unplanned", BuildProgress{Panels: 41}, "Saved what the agent built before it stopped: 41 panels"},
		{"one panel unplanned", BuildProgress{Panels: 1}, "Saved what the agent built before it stopped: 1 panel"},
		{"built above planned", BuildProgress{Panels: 9, PlannedPanels: 5}, "Saved what the agent built before it stopped: 9 panels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stoppedSummary(tt.p); got != tt.want {
				t.Fatalf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}
