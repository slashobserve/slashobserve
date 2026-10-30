package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func exemplarPanel(t *testing.T, typ, target string) (spec.Panel, PanelLine) {
	t.Helper()
	return convertedPanel(t, panelDoc(typ, `"targets":[`+target+`]`))
}

func TestExemplarOnARangeTargetOfATimeSeriesIsKept(t *testing.T) {
	panel, line := exemplarPanel(t, "timeseries", `{"refId":"A","expr":"up","exemplar":true}`)
	if !panel.Queries[0].Exemplar {
		t.Fatalf("query = %+v, want exemplar kept", panel.Queries[0])
	}
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v, want imported with no report", line.Status, line.Paths)
	}
}

func TestExemplarFalseIsSilentAndNotWritten(t *testing.T) {
	panel, line := exemplarPanel(t, "timeseries", `{"refId":"A","expr":"up","exemplar":false}`)
	if panel.Queries[0].Exemplar {
		t.Fatalf("query = %+v, want no exemplar", panel.Queries[0])
	}
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v, want imported", line.Status, line.Paths)
	}
}

func TestExemplarOnAnInstantOnlyTargetIsDroppedAndReported(t *testing.T) {
	panel, line := exemplarPanel(t, "timeseries", `{"refId":"A","expr":"up","instant":true,"exemplar":true}`)
	if panel.Queries[0].Exemplar || !panel.Queries[0].Instant {
		t.Fatalf("query = %+v, want an instant query without exemplar", panel.Queries[0])
	}
	if !slices.Equal(line.Paths, []string{"targets[0].exemplar"}) || !slices.Contains(line.Reasons, exemplarsTimeSeriesOnly) {
		t.Fatalf("paths = %v, reasons = %v, want the exemplar path with %q", line.Paths, line.Reasons, exemplarsTimeSeriesOnly)
	}
}

func TestExemplarOnAnInstantAndRangeTargetStaysOnTheRangeQuery(t *testing.T) {
	panel, line := exemplarPanel(t, "timeseries", `{"refId":"A","expr":"up","instant":true,"range":true,"exemplar":true}`)
	if len(panel.Queries) != 2 || !panel.Queries[0].Exemplar || panel.Queries[1].Exemplar || !panel.Queries[1].Instant {
		t.Fatalf("queries = %+v, want exemplar on the range query only", panel.Queries)
	}
	if line.Status != statusImported || len(line.Paths) != 0 {
		t.Fatalf("status = %s, paths %v, want imported", line.Status, line.Paths)
	}
}

func TestExemplarOnAPanelThatDrawsNoneIsDroppedAndReported(t *testing.T) {
	for _, typ := range []string{"stat", "table", "gauge", "bargauge", "state-timeline"} {
		t.Run(typ, func(t *testing.T) {
			panel, line := exemplarPanel(t, typ, `{"refId":"A","expr":"up","exemplar":true}`)
			if panel.Queries[0].Exemplar {
				t.Fatalf("query = %+v, want no exemplar", panel.Queries[0])
			}
			if !slices.Contains(line.Reasons, exemplarsTimeSeriesOnly) {
				t.Fatalf("reasons = %v, want %q", line.Reasons, exemplarsTimeSeriesOnly)
			}
		})
	}
}
