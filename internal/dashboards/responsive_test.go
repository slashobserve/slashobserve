package dashboards

import (
	"bytes"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func sizedPanel(id string, x, y, w, h int) spec.Panel {
	return spec.Panel{ID: id, Type: spec.Timeseries, GridPos: spec.GridPos{X: x, Y: y, W: w, H: h}}
}

func fixedPanel(p spec.Panel) spec.Panel {
	p.Responsive = new(false)
	return p
}

func dashboardOf(panels ...spec.Panel) *spec.Dashboard {
	return &spec.Dashboard{Panels: panels}
}

func rowOf(id string, children ...spec.Panel) spec.Panel {
	return spec.Panel{ID: id, Type: spec.Row, GridPos: spec.GridPos{W: 24, H: 1}, Panels: children}
}

func TestFreezeResized(t *testing.T) {
	tests := []struct {
		name        string
		prev, next  *spec.Dashboard
		wantFrozen  map[string]bool
		wantChanged bool
	}{
		{
			name:        "a wider panel is frozen",
			prev:        dashboardOf(sizedPanel("a", 0, 0, 12, 8)),
			next:        dashboardOf(sizedPanel("a", 0, 0, 16, 8)),
			wantFrozen:  map[string]bool{"a": true},
			wantChanged: true,
		},
		{
			name:        "a taller panel is frozen",
			prev:        dashboardOf(sizedPanel("a", 0, 0, 12, 8)),
			next:        dashboardOf(sizedPanel("a", 0, 0, 12, 10)),
			wantFrozen:  map[string]bool{"a": true},
			wantChanged: true,
		},
		{
			name:       "a moved panel stays responsive",
			prev:       dashboardOf(sizedPanel("a", 0, 0, 12, 8)),
			next:       dashboardOf(sizedPanel("a", 12, 8, 12, 8)),
			wantFrozen: map[string]bool{"a": false},
		},
		{
			name:       "a new panel stays responsive",
			prev:       dashboardOf(sizedPanel("a", 0, 0, 12, 8)),
			next:       dashboardOf(sizedPanel("a", 0, 0, 12, 8), sizedPanel("b", 12, 0, 6, 4)),
			wantFrozen: map[string]bool{"a": false, "b": false},
		},
		{
			name:        "a row child is frozen",
			prev:        dashboardOf(rowOf("r", sizedPanel("a", 0, 1, 12, 8))),
			next:        dashboardOf(rowOf("r", sizedPanel("a", 0, 1, 24, 8))),
			wantFrozen:  map[string]bool{"a": true},
			wantChanged: true,
		},
		{
			name:       "a panel already fixed is left alone",
			prev:       dashboardOf(fixedPanel(sizedPanel("a", 0, 0, 12, 8))),
			next:       dashboardOf(fixedPanel(sizedPanel("a", 0, 0, 6, 8))),
			wantFrozen: map[string]bool{"a": true},
		},
		{
			name:       "a panel made responsive at the same size stays responsive",
			prev:       dashboardOf(fixedPanel(sizedPanel("a", 0, 0, 12, 8))),
			next:       dashboardOf(sizedPanel("a", 0, 0, 12, 8)),
			wantFrozen: map[string]bool{"a": false},
		},
		{
			name:       "a resized row is left alone",
			prev:       dashboardOf(rowOf("r")),
			next:       dashboardOf(spec.Panel{ID: "r", Type: spec.Row, GridPos: spec.GridPos{W: 24, H: 2}}),
			wantFrozen: map[string]bool{"r": false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := freezeResized(tt.prev, tt.next); got != tt.wantChanged {
				t.Errorf("freezeResized = %v, want %v", got, tt.wantChanged)
			}
			for id, want := range tt.wantFrozen {
				entry, ok := tt.next.FindPanel(id)
				if !ok {
					t.Fatalf("no panel %s", id)
				}
				frozen := entry.Panel.Responsive != nil && !*entry.Panel.Responsive
				if frozen != want {
					t.Errorf("panel %s frozen = %v, want %v", id, frozen, want)
				}
			}
		})
	}
}

func resizedRPS() []byte {
	return bytes.Replace(panelsSpec("t", "rps"), []byte(`"w":12`), []byte(`"w":24`), 1)
}

func isFrozen(t *testing.T, d *spec.Dashboard, id string) bool {
	t.Helper()
	entry, ok := d.FindPanel(id)
	if !ok {
		t.Fatalf("no panel %s", id)
	}
	return entry.Panel.Responsive != nil && !*entry.Panel.Responsive
}

func TestASaveThatResizesAPanelKeepsItAtItsSize(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t, "rps")

	_, ver, _, err := r.service.AddVersion(t.Context(), testUser, dash, 1, resizedRPS(), "wider")
	if err != nil {
		t.Fatal(err)
	}

	stored, err := r.service.VersionSpec(t.Context(), dash, ver.Number)
	if err != nil {
		t.Fatal(err)
	}
	if !isFrozen(t, stored, "rps") || stored.Panels[0].GridPos.W != 24 {
		t.Errorf("rps = %+v with responsive %v, want 24 wide and false", stored.Panels[0].GridPos, stored.Panels[0].Responsive)
	}
}

func TestARepeatedResizeSaveReplays(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t, "rps")
	_, first, _, err := r.service.AddVersion(t.Context(), testUser, dash, 1, resizedRPS(), "wider")
	if err != nil {
		t.Fatal(err)
	}

	_, again, replayed, err := r.service.AddVersion(t.Context(), testUser, dash, 1, resizedRPS(), "wider")
	if err != nil || !replayed || again.Number != first.Number {
		t.Fatalf("second save: version %d, replayed %v, err %v", again.Number, replayed, err)
	}
}

func TestARestoreKeepsTheRestoredFlags(t *testing.T) {
	r := newRig(t, true)
	dash := r.createDashboard(t, "rps")
	if _, _, _, err := r.service.AddVersion(t.Context(), testUser, dash, 1, resizedRPS(), "wider"); err != nil {
		t.Fatal(err)
	}

	_, ver, err := r.service.Restore(t.Context(), testUser, dash, 1, 2, "")
	if err != nil {
		t.Fatal(err)
	}

	if isFrozen(t, ver.Spec, "rps") || ver.Spec.Panels[0].GridPos.W != 12 {
		t.Errorf("restored rps = %+v with responsive %v, want 12 wide and responsive", ver.Spec.Panels[0].GridPos, ver.Spec.Panels[0].Responsive)
	}
}

func TestACommittedRevisionBuildFreezesAResizedPanel(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "rps", "p99")
	id := r.runningRequest(t, dash)
	if _, err := r.service.SaveBuild(t.Context(), testUser, BuildSave{
		RequestID: id, Attempt: buildAttempt, DashboardID: dash, BaseVersion: 1,
		Spec: bytes.Replace(panelsSpec("t", "rps", "p99"), []byte(`"w":12`), []byte(`"w":24`), 1),
	}); err != nil {
		t.Fatal(err)
	}

	_, ver, _, err := r.service.CommitBuild(t.Context(), testUser, id, buildAttempt)
	if err != nil {
		t.Fatal(err)
	}

	if !isFrozen(t, ver.Spec, "rps") || isFrozen(t, ver.Spec, "p99") {
		t.Errorf("rps responsive %v, p99 responsive %v, want false and absent", ver.Spec.Panels[0].Responsive, ver.Spec.Panels[1].Responsive)
	}
}
