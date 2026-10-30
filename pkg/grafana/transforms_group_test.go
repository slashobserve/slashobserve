package grafana

import (
	"slices"
	"testing"
)

func TestGroupTransformsConvert(t *testing.T) {
	cases := []struct {
		name, transformation, want string
	}{
		{
			"groupBy with a group field and aggregates",
			`{"id":"groupBy","options":{"fields":{
				"host":{"operation":"groupby","aggregations":[]},
				"latency":{"operation":"aggregate","aggregations":["min","max","lastNotNull"]},
				"errors":{"operation":"aggregate","aggregations":["sum"]},
				"note":{"operation":null,"aggregations":[]}}}}`,
			`[{"type":"groupBy","by":["host"],"aggregate":[{"field":"errors","reducers":["sum"]},{"field":"latency","reducers":["min","max","lastNotNull"]}]}]`,
		},
		{
			"groupBy with only ignored fields keeps no field",
			`{"id":"groupBy","options":{"fields":{"latency":{"operation":null,"aggregations":[]}}}}`,
			`[{"type":"groupBy"}]`,
		},
		{
			"groupBy without fields is the identity and is dropped",
			`{"id":"groupBy","options":{}}`,
			`null`,
		},
		{
			"groupToNestedTable",
			`{"id":"groupToNestedTable","options":{"fields":{
				"host":{"operation":"groupby","aggregations":[]},
				"latency":{"operation":"aggregate","aggregations":["mean"]}}}}`,
			`[{"type":"groupToNestedTable","by":["host"],"aggregate":[{"field":"latency","reducers":["mean"]}]}]`,
		},
		{
			"groupToNestedTable without subframe headers",
			`{"id":"groupToNestedTable","options":{"showSubframeHeaders":false,"fields":{"host":{"operation":"groupby"}}}}`,
			`[{"type":"groupToNestedTable","by":["host"],"hideHeaders":true}]`,
		},
		{
			"groupToNestedTable with subframe headers is the default",
			`{"id":"groupToNestedTable","options":{"showSubframeHeaders":true,"fields":{"host":{"operation":"groupby"}}}}`,
			`[{"type":"groupToNestedTable","by":["host"]}]`,
		},
		{
			"groupingToMatrix defaults",
			`{"id":"groupingToMatrix","options":{"emptyValue":"null"}}`,
			`[{"type":"groupingToMatrix"}]`,
		},
		{
			"groupingToMatrix with fields and a fill",
			`{"id":"groupingToMatrix","options":{"columnField":"host","rowField":"service","valueField":"count","emptyValue":"zero"}}`,
			`[{"type":"groupingToMatrix","value":"count","column":"host","row":"service","empty":"zero"}]`,
		},
		{
			"histogram defaults",
			`{"id":"histogram","options":{}}`,
			`[{"type":"histogram"}]`,
		},
		{
			"histogram with buckets",
			`{"id":"histogram","options":{"bucketCount":10,"bucketSize":5,"bucketOffset":1.5,"combine":true}}`,
			`[{"type":"histogram","bucketCount":10,"bucketSize":5,"bucketOffset":1.5,"combine":true}]`,
		},
		{
			"regression defaults to a line",
			`{"id":"regression","options":{"modelType":"linear"}}`,
			`[{"type":"regression"}]`,
		},
		{
			"regression polynomial of the default degree",
			`{"id":"regression","options":{"modelType":"polynomial","degree":2}}`,
			`[{"type":"regression","model":"polynomial"}]`,
		},
		{
			"regression polynomial with fields and a prediction count",
			`{"id":"regression","options":{"modelType":"polynomial","degree":3,"xFieldName":"Time","yFieldName":"Value","predictionCount":100}}`,
			`[{"type":"regression","model":"polynomial","degree":3,"x":"Time","y":"Value","predictions":100}]`,
		},
		{
			"regression ignores the degree of a line",
			`{"id":"regression","options":{"modelType":"linear","degree":4}}`,
			`[{"type":"regression"}]`,
		},
		{
			"transpose defaults",
			`{"id":"transpose","options":{"firstFieldName":"Field","restFieldsName":"Value"}}`,
			`[{"type":"transpose"}]`,
		},
		{
			"transpose with names and a fill",
			`{"id":"transpose","options":{"firstFieldName":"Metric","restFieldsName":"Series","emptyValue":"false"}}`,
			`[{"type":"transpose","empty":"false","firstName":"Metric","restName":"Series"}]`,
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

func TestGroupTransformsReport(t *testing.T) {
	cases := []struct {
		name, transformation, path string
		kept                       int
	}{
		{"groupBy unsupported reducer", `{"id":"groupBy","options":{"fields":{"a":{"operation":"aggregate","aggregations":["p95"]},"b":{"operation":"groupby"}}}}`, "transformations[0].options.fields.a.aggregations", 1},
		{"groupBy other field option", `{"id":"groupBy","options":{"fields":{"a":{"operation":"groupby","aggregation":"x"}}}}`, "transformations[0].options.fields.a.aggregation", 1},
		{"groupBy other option", `{"id":"groupBy","options":{"fields":{"a":{"operation":"groupby"}},"sort":true}}`, "transformations[0].options.sort", 1},
		{"groupToNestedTable hidden headers without fields", `{"id":"groupToNestedTable","options":{"showSubframeHeaders":false}}`, "transformations[0].options.showSubframeHeaders", 0},
		{"groupingToMatrix unknown fill", `{"id":"groupingToMatrix","options":{"emptyValue":"nan"}}`, "transformations[0].options.emptyValue", 1},
		{"histogram fields matcher", `{"id":"histogram","options":{"fields":{"byNames":["a"]}}}`, "transformations[0].options.fields", 1},
		{"histogram bucket count out of range", `{"id":"histogram","options":{"bucketCount":5000}}`, "transformations[0].options.bucketCount", 1},
		{"histogram bucket size not positive", `{"id":"histogram","options":{"bucketSize":-1}}`, "transformations[0].options.bucketSize", 1},
		{"regression unknown model", `{"id":"regression","options":{"modelType":"exponential"}}`, "transformations[0].options.modelType", 0},
		{"regression degree out of range", `{"id":"regression","options":{"modelType":"polynomial","degree":9}}`, "transformations[0].options.degree", 1},
		{"regression prediction count out of range", `{"id":"regression","options":{"predictionCount":0.5}}`, "transformations[0].options.predictionCount", 1},
		{"transpose unknown fill", `{"id":"transpose","options":{"emptyValue":"nan"}}`, "transformations[0].options.emptyValue", 1},
		{"transpose other option", `{"id":"transpose","options":{"sort":true}}`, "transformations[0].options.sort", 1},
		{"heatmap stays reported", `{"id":"heatmap","options":{}}`, "transformations[0]", 0},
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
