package tools

import (
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestDefaultSizeFollowsTheGuide(t *testing.T) {
	tests := []struct {
		panelType spec.PanelType
		w, h      int
	}{
		{spec.Timeseries, 12, 8},
		{spec.Stat, 6, 4},
		{spec.Gauge, 6, 4},
		{spec.Table, 24, 8},
		{spec.Text, 12, 8},
	}
	for _, tt := range tests {
		t.Run(string(tt.panelType), func(t *testing.T) {
			got := defaultSize(tt.panelType)
			if got.W != tt.w || got.H != tt.h {
				t.Errorf("size = %dx%d, want %dx%d", got.W, got.H, tt.w, tt.h)
			}
		})
	}
}
