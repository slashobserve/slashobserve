package agent

import "testing"

func TestImportCountsAsSave(t *testing.T) {
	if !isSave("import_grafana_dashboard") {
		t.Fatal("import_grafana_dashboard is not a save")
	}
	if got := phaseFor("import_grafana_dashboard", false); got != phaseSave {
		t.Fatalf("phase = %q, want %q", got, phaseSave)
	}
	if got := phaseFor("import_grafana_dashboard", true); got != phaseRepair {
		t.Fatalf("repairing phase = %q, want %q", got, phaseRepair)
	}
	if isSave("list_dashboards") {
		t.Fatal("list_dashboards is a save")
	}
}
