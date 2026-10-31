package grafana

import (
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestGaugeWithOnlyALayoutDecisionIsImported(t *testing.T) {
	_, line := vizPanel(t, "gauge", `{"orientation":"horizontal"}`)
	if line.Status != statusImported {
		t.Fatalf("status = %s, want imported", line.Status)
	}
	if !slices.Equal(line.Paths, []string{"options.orientation"}) {
		t.Fatalf("paths = %v, want options.orientation", line.Paths)
	}
	if !slices.Equal(line.Reasons, []string{decisionGaugeLayout.reason}) {
		t.Fatalf("reasons = %v, want the gauge layout reason", line.Reasons)
	}
}

func TestGaugeWithADecisionAndAnUnsupportedOptionIsPartial(t *testing.T) {
	_, line := vizPanel(t, "gauge", `{"orientation":"horizontal","sizing":"manual","minVizWidth":-5}`)
	if line.Status != statusPartial {
		t.Fatalf("status = %s, want partial", line.Status)
	}
	if len(line.Paths) != 2 || !slices.Contains(line.Paths, "options.orientation") || !slices.Contains(line.Paths, "options.minVizWidth") {
		t.Fatalf("paths = %v, want both options", line.Paths)
	}
	if len(line.Reasons) != 2 || !slices.Contains(line.Reasons, decisionGaugeLayout.reason) {
		t.Fatalf("reasons = %v, want the layout reason and the unsupported one", line.Reasons)
	}
}

func TestDecidedNeedsEveryEntryAndNoOmittedPathToBeADecision(t *testing.T) {
	excluded := func(path string) (decision, bool) {
		if path == "options.orientation" {
			return decisionGaugeLayout, true
		}
		return decision{}, false
	}
	cases := []struct {
		name  string
		limit int
		adds  []string
		want  bool
	}{
		{"no entries", 10, nil, true},
		{"a decision only", 10, []string{"options.orientation"}, true},
		{"a decision and another option", 10, []string{"options.orientation", "options.other"}, false},
		{"another option only", 10, []string{"options.other"}, false},
		{"a decision past the path limit", 1, []string{"options.other", "options.orientation"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := newIssueLog(excluded)
			log.limit = tc.limit
			for _, path := range tc.adds {
				log.add(path, "not supported")
			}
			if got := log.decided(); got != tc.want {
				t.Fatalf("decided() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestImportedLinesCarryOnlyDecisions(t *testing.T) {
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	files, err := fs.Glob(root.FS(), "corpus/*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("the corpus is empty")
	}
	for _, file := range files {
		raw, err := root.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Convert(raw, conformanceOptions)
		if err != nil {
			continue
		}
		for _, line := range res.Report.Panels {
			if line.Status != statusImported {
				continue
			}
			for _, reason := range line.Reasons {
				if !isDecisionReason(reason) {
					t.Errorf("%s panel %s is imported with a reason that is not a decision", file, line.PanelID)
				}
			}
		}
	}
}

func requireDecided(t *testing.T, line PanelLine, path, reason string) {
	t.Helper()
	if line.Status != statusImported || !slices.Equal(line.Paths, []string{path}) {
		t.Fatalf("status = %s, paths %v, want imported with %s", line.Status, line.Paths, path)
	}
	if !slices.Contains(line.Reasons, reason) {
		t.Fatalf("reasons = %v, want %q", line.Reasons, reason)
	}
}

func TestTransformationOptionsExcludedByDecisionLeaveThePanelImported(t *testing.T) {
	cases := []struct {
		name, transformation, path string
		kept                       int
		reshape                    bool
	}{
		{"enum settings", `{"id":"convertFieldType","options":{"conversions":[{"targetField":"s","destinationType":"enum","enumConfig":{"text":["a"]}}]}}`, "transformations[0].options.conversions[0].enumConfig", 1, false},
		{"percent unary", `{"id":"calculateField","options":{"mode":"unary","unary":{"operator":"percent"}}}`, "transformations[0].options.unary.operator", 0, false},
		{"window max", `{"id":"calculateField","options":{"mode":"windowFunctions","window":{"reducer":"max"}}}`, "transformations[0].options.window.reducer", 0, false},
		{"row reduction null handling", `{"id":"calculateField","options":{"mode":"reduceRow","reduce":{"reducer":"sum","nullValueMode":"null as zero"}}}`, "transformations[0].options.reduce.nullValueMode", 1, false},
		{"calculation without joining series by time", `{"id":"calculateField","options":{"mode":"reduceRow","timeSeries":false}}`, "transformations[0].options.timeSeries", 1, false},
		{"orderBy", `{"id":"order","options":{"indexByName":{"a":0},"orderBy":[{"type":"name","name":"a"}]}}`, "transformations[0].options.orderBy", 1, false},
		{"rowsToFields threshold handler", `{"id":"rowsToFields","options":{"mappings":[{"fieldName":"T","handlerKey":"threshold1"}]}}`, "transformations[0].options.mappings[0].handlerKey", 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var panel spec.Panel
			var line PanelLine
			if tc.reshape {
				panel, line = reshapePanel(t, tc.transformation)
			} else {
				panel, line = transformPanel(t, "", tc.transformation)
			}
			if line.Status != statusImported || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v, want imported with %s listed", line.Status, line.Paths, tc.path)
			}
			if len(panel.Transform) != tc.kept {
				t.Fatalf("kept %d transformations, want %d: %+v", len(panel.Transform), tc.kept, panel.Transform)
			}
		})
	}
}

func TestAxisScaleExcludedByDecisionLeavesThePanelImported(t *testing.T) {
	for _, typ := range []string{"timeseries", "xychart"} {
		t.Run(typ, func(t *testing.T) {
			_, line := customPanelOf(t, typ, `{"scaleDistribution":{"type":"ordinal"}}`, "")
			requireDecided(t, line, "fieldConfig.defaults.custom.scaleDistribution.type", decisionValueAxisPlacement.reason)
		})
	}
}

func TestDroppedExpressionsExcludedByDecisionLeaveThePanelImported(t *testing.T) {
	cases := []struct{ name, members, reason string }{
		{"classic conditions", `"type":"classic_conditions","conditions":[{"type":"query","query":{"params":["A"]},"reducer":{"type":"avg","params":[]},"evaluator":{"type":"gt","params":[5]},"operator":{"type":"and"}}]`, decisionAlertRules.reason},
		{"sql", `"type":"sql","expression":"SELECT * FROM A LIMIT 10"`, decisionSQLExpressions.reason},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := expressionPanel(t, `{"refId":"A","expr":"up"}`, expressionTarget("B", tc.members))
			if len(panel.Queries) != 1 || panel.Queries[0].Expression != nil {
				t.Fatalf("queries = %+v, want only the data query", panel.Queries)
			}
			if line.Status != statusImported || line.Queries[1].Dropped == nil || *line.Queries[1].Dropped != tc.reason {
				t.Fatalf("status = %s, query line = %+v, want imported and dropped with %q", line.Status, line.Queries[1], tc.reason)
			}
			if !slices.Contains(line.Reasons, tc.reason) || !slices.Contains(line.Paths, "targets[1]") {
				t.Fatalf("reasons = %v, paths = %v, want %q at targets[1]", line.Reasons, line.Paths, tc.reason)
			}
		})
	}
}
