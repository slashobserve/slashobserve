package grafana

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const twoTargets = `"targets":[{"refId":"A","expr":"up"},{"refId":"config","expr":"limits"}]`

func reshapePanel(t *testing.T, transformation string) (spec.Panel, PanelLine) {
	t.Helper()
	doc := `{"title":"T","schemaVersion":39,"panels":[{` + panelFrame +
		`,"type":"table",` + twoTargets + `,"transformations":[` + transformation + `]}]}`
	return convertedPanel(t, doc)
}

func TestReshapeTransformsConvert(t *testing.T) {
	cases := []struct {
		name, transformation, want string
	}{
		{
			"labelsToFields defaults",
			`{"id":"labelsToFields","options":{}}`,
			`[{"type":"labelsToFields"}]`,
		},
		{
			"labelsToFields rows with kept labels and a value label",
			`{"id":"labelsToFields","options":{"mode":"rows","keepLabels":["pod","pod","namespace"],"valueLabel":"pod"}}`,
			`[{"type":"labelsToFields","mode":"rows","include":["pod","namespace"],"valueLabel":"pod"}]`,
		},
		{
			"labelsToFields columns is the default",
			`{"id":"labelsToFields","options":{"mode":"columns","keepLabels":["job"]}}`,
			`[{"type":"labelsToFields","include":["job"]}]`,
		},
		{
			"joinByLabels",
			`{"id":"joinByLabels","options":{"value":"mode","join":["instance","job"]}}`,
			`[{"type":"joinByLabels","value":"mode","on":["instance","job"]}]`,
		},
		{
			"partitionByValues as labels",
			`{"id":"partitionByValues","options":{"fields":["host","env"],"keepFields":true,"naming":{"asLabels":true}}}`,
			`[{"type":"partitionByValues","fields":["host","env"],"keepFields":true}]`,
		},
		{
			"partitionByValues as names",
			`{"id":"partitionByValues","options":{"fields":["host"],"naming":{"asLabels":false}}}`,
			`[{"type":"partitionByValues","fields":["host"],"naming":"name"}]`,
		},
		{
			"partitionByValues appended to the frame name",
			`{"id":"partitionByValues","options":{"fields":["host"],"naming":{"asLabels":false,"append":true}}}`,
			`[{"type":"partitionByValues","fields":["host"],"naming":"append"}]`,
		},
		{
			"prepareTimeSeries multi is many frames",
			`{"id":"prepareTimeSeries","options":{"format":"multi"}}`,
			`[{"type":"prepareTimeSeries","format":"many"}]`,
		},
		{
			"prepareTimeSeries many",
			`{"id":"prepareTimeSeries","options":{"format":"many"}}`,
			`[{"type":"prepareTimeSeries","format":"many"}]`,
		},
		{
			"prepareTimeSeries without a format is wide",
			`{"id":"prepareTimeSeries","options":{}}`,
			`[{"type":"prepareTimeSeries","format":"wide"}]`,
		},
		{
			"prepareTimeSeries long",
			`{"id":"prepareTimeSeries","options":{"format":"long"}}`,
			`[{"type":"prepareTimeSeries","format":"long"}]`,
		},
		{
			"timeSeriesTable stat per query",
			`{"id":"timeSeriesTable","options":{"A":{"stat":"max","timeField":"Time"},"config":{"stat":"lastNotNull"}}}`,
			`[{"type":"timeSeriesTable","stats":{"A":"max"}}]`,
		},
		{
			"timeSeriesTable with only the time field",
			`{"id":"timeSeriesTable","options":{"A":{"timeField":"Time"},"receive":{"timeField":"Time"}}}`,
			`[{"type":"timeSeriesTable"}]`,
		},
		{
			"rowsToFields",
			`{"id":"rowsToFields","options":{"mappings":[
				{"fieldName":"Field","handlerKey":"field.name"},
				{"fieldName":"Value","handlerKey":"field.value"},
				{"fieldName":"Unit","handlerKey":"unit"},
				{"fieldName":"Max","handlerKey":"max"},
				{"fieldName":"Skip","handlerKey":""}]}}`,
			`[{"type":"rowsToFields","mappings":[{"field":"Field","to":"name"},{"field":"Value","to":"value"},{"field":"Unit","to":"unit"},{"field":"Max","to":"max"}]}]`,
		},
		{
			"configFromData",
			`{"id":"configFromData","options":{"configRefId":"config","mappings":[
				{"fieldName":"limit","handlerKey":"max","reducerId":"max"},
				{"fieldName":"unit","handlerKey":"unit","reducerId":"lastNotNull"}],
				"applyTo":{"id":"byName","options":"cpu"}}}`,
			`[{"type":"configFromData","mappings":[{"field":"limit","to":"max","reducer":"max"},{"field":"unit","to":"unit"}],"configQuery":"config","applyTo":{"field":"cpu"}}]`,
		},
		{
			"configFromData by type",
			`{"id":"configFromData","options":{"configRefId":"config","mappings":[{"fieldName":"limit","handlerKey":"min"}],"applyTo":{"id":"byType","options":"number"}}}`,
			`[{"type":"configFromData","mappings":[{"field":"limit","to":"min"}],"configQuery":"config","applyTo":{"fieldType":"number"}}]`,
		},
		{
			"concatenate drops the frame name by default",
			`{"id":"concatenate","options":{"frameNameMode":"drop","frameNameLabel":"frame"}}`,
			`[{"type":"concatenate"}]`,
		},
		{
			"concatenate with the frame name in the field name",
			`{"id":"concatenate","options":{"frameNameMode":"field"}}`,
			`[{"type":"concatenate","nameMode":"field"}]`,
		},
		{
			"concatenate with a label",
			`{"id":"concatenate","options":{"frameNameMode":"label","frameNameLabel":"query"}}`,
			`[{"type":"concatenate","nameMode":"label","nameLabel":"query"}]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := reshapePanel(t, tc.transformation)
			if line.Status != statusImported || len(line.Paths) != 0 {
				t.Fatalf("status %s, paths %v", line.Status, line.Paths)
			}
			wantJSON(t, panel.Transform, tc.want)
		})
	}
}

func TestReshapeTransformsReport(t *testing.T) {
	cases := []struct {
		name, transformation, path string
		kept                       int
	}{
		{"labelsToFields unknown mode", `{"id":"labelsToFields","options":{"mode":"grid"}}`, "transformations[0].options.mode", 1},
		{"labelsToFields other option", `{"id":"labelsToFields","options":{"sort":true}}`, "transformations[0].options.sort", 1},
		{"joinByLabels without a value label", `{"id":"joinByLabels","options":{"join":["a"]}}`, "transformations[0]", 0},
		{"partitionByValues without fields", `{"id":"partitionByValues","options":{"keepFields":true}}`, "transformations[0]", 0},
		{"partitionByValues label naming with append", `{"id":"partitionByValues","options":{"fields":["a"],"naming":{"asLabels":true,"append":true}}}`, "transformations[0].options.naming.append", 1},
		{"partitionByValues other naming option", `{"id":"partitionByValues","options":{"fields":["a"],"naming":{"separator":"-"}}}`, "transformations[0].options.naming.separator", 1},
		{"prepareTimeSeries unknown format", `{"id":"prepareTimeSeries","options":{"format":"tall"}}`, "transformations[0].options.format", 0},
		{"timeSeriesTable other time field", `{"id":"timeSeriesTable","options":{"A":{"timeField":"ts"}}}`, "transformations[0].options.A.timeField", 1},
		{"timeSeriesTable unknown stat", `{"id":"timeSeriesTable","options":{"A":{"stat":"p99"}}}`, "transformations[0].options.A.stat", 1},
		{"timeSeriesTable stat for a query the panel lacks", `{"id":"timeSeriesTable","options":{"Z":{"stat":"max"}}}`, "transformations[0].options.Z.stat", 1},
		{"rowsToFields threshold handler", `{"id":"rowsToFields","options":{"mappings":[{"fieldName":"T","handlerKey":"threshold1"}]}}`, "transformations[0].options.mappings[0].handlerKey", 1},
		{"rowsToFields reducer", `{"id":"rowsToFields","options":{"mappings":[{"fieldName":"V","handlerKey":"field.value","reducerId":"max"}]}}`, "transformations[0].options.mappings[0].reducerId", 1},
		{"configFromData without a known config query", `{"id":"configFromData","options":{"configRefId":"Z","mappings":[{"fieldName":"a","handlerKey":"max"}]}}`, "transformations[0].options.configRefId", 0},
		{"configFromData with no usable mapping", `{"id":"configFromData","options":{"configRefId":"config","mappings":[{"fieldName":"a","handlerKey":"threshold1"}]}}`, "transformations[0].options.mappings", 0},
		{"configFromData unknown reducer", `{"id":"configFromData","options":{"configRefId":"config","mappings":[{"fieldName":"a","handlerKey":"max","reducerId":"p99"}]}}`, "transformations[0].options.mappings[0].reducerId", 1},
		{"configFromData unsupported matcher", `{"id":"configFromData","options":{"configRefId":"config","mappings":[{"fieldName":"a","handlerKey":"max"}],"applyTo":{"id":"byValue","options":{}}}}`, "transformations[0].options.applyTo", 1},
		{"concatenate unknown mode", `{"id":"concatenate","options":{"frameNameMode":"suffix"}}`, "transformations[0].options.frameNameMode", 1},
		{"concatenate other option", `{"id":"concatenate","options":{"seriesToColumns":true}}`, "transformations[0].options.seriesToColumns", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := reshapePanel(t, tc.transformation)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v, want %s reported", line.Status, line.Paths, tc.path)
			}
			if len(panel.Transform) != tc.kept {
				t.Fatalf("kept %d transformations, want %d: %+v", len(panel.Transform), tc.kept, panel.Transform)
			}
		})
	}
}
