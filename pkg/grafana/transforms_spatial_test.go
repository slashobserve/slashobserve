package grafana

import (
	"slices"
	"strings"
	"testing"
)

func TestSpatialTransformsConvert(t *testing.T) {
	cases := []struct {
		name, transformation, want string
	}{
		{
			"spatial without options prepares from automatic detection",
			`{"id":"spatial","options":{}}`,
			`[{"type":"spatial"}]`,
		},
		{
			"spatial prepare in automatic mode is the default and ignores field names",
			`{"id":"spatial","options":{"action":"prepare","source":{"mode":"auto","latitude":"y","longitude":"x"}}}`,
			`[{"type":"spatial"}]`,
		},
		{
			"coords names the latitude and longitude fields",
			`{"id":"spatial","options":{"source":{"mode":"coords","latitude":"lat","longitude":"lon"}}}`,
			`[{"type":"spatial","source":{"mode":"coords","latitude":"lat","longitude":"lon"}}]`,
		},
		{
			"geohash names its field and drops names that belong to other modes",
			`{"id":"spatial","options":{"source":{"mode":"geohash","geohash":"hash","latitude":"y"}}}`,
			`[{"type":"spatial","source":{"mode":"geohash","geohash":"hash"}}]`,
		},
		{
			"lookup maps the countries gazetteer file to its built-in name",
			`{"id":"spatial","options":{"source":{"mode":"lookup","lookup":"country","gazetteer":"public/gazetteer/countries.json"}}}`,
			`[{"type":"spatial","source":{"mode":"lookup","lookup":"country","gazetteer":"countries"}}]`,
		},
		{
			"lookup in the US states gazetteer",
			`{"id":"spatial","options":{"source":{"mode":"lookup","lookup":"state","gazetteer":"public/gazetteer/usa-states.json"}}}`,
			`[{"type":"spatial","source":{"mode":"lookup","lookup":"state","gazetteer":"usa-states"}}]`,
		},
		{
			"calculate action alone",
			`{"id":"spatial","options":{"action":"calculate"}}`,
			`[{"type":"spatial","action":"calculate"}]`,
		},
		{
			"calculate a distance into a named field",
			`{"id":"spatial","options":{"action":"calculate","calculate":{"calc":"distance","field":"Leg"}}}`,
			`[{"type":"spatial","field":"Leg","action":"calculate","calc":"distance"}]`,
		},
		{
			"calculate options are ignored by other actions",
			`{"id":"spatial","options":{"action":"prepare","calculate":{"calc":"area"}}}`,
			`[{"type":"spatial"}]`,
		},
		{
			"modify as a line",
			`{"id":"spatial","options":{"action":"modify","modify":{"op":"asLine"}}}`,
			`[{"type":"spatial","action":"modify","modify":"asLine"}]`,
		},
		{
			"modify as a line ignores a target",
			`{"id":"spatial","options":{"action":"modify","modify":{"op":"asLine","target":{"mode":"coords","latitude":"a","longitude":"b"}}}}`,
			`[{"type":"spatial","action":"modify","modify":"asLine"}]`,
		},
		{
			"line builder with a coords target",
			`{"id":"spatial","options":{"action":"modify","modify":{"op":"lineBuilder","target":{"mode":"coords","latitude":"lat2","longitude":"lon2"}}}}`,
			`[{"type":"spatial","action":"modify","modify":"lineBuilder","target":{"mode":"coords","latitude":"lat2","longitude":"lon2"}}]`,
		},
		{
			"line builder with a lookup target",
			`{"id":"spatial","options":{"action":"modify","modify":{"op":"lineBuilder","target":{"mode":"lookup","lookup":"to","gazetteer":"public/gazetteer/countries.json"}}}}`,
			`[{"type":"spatial","action":"modify","modify":"lineBuilder","target":{"mode":"lookup","lookup":"to","gazetteer":"countries"}}]`,
		},
		{
			"fieldLookup without options",
			`{"id":"fieldLookup","options":{}}`,
			`[{"type":"fieldLookup"}]`,
		},
		{
			"fieldLookup names its field and gazetteer",
			`{"id":"fieldLookup","options":{"lookupField":"country","gazetteer":"public/gazetteer/countries.json"}}`,
			`[{"type":"fieldLookup","lookupField":"country","gazetteer":"countries"}]`,
		},
		{
			"fieldLookup in the US states gazetteer",
			`{"id":"fieldLookup","options":{"lookupField":"state","gazetteer":"public/gazetteer/usa-states.json"}}`,
			`[{"type":"fieldLookup","lookupField":"state","gazetteer":"usa-states"}]`,
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

func TestSpatialTransformsReport(t *testing.T) {
	longName := strings.Repeat("a", maxNameMatch+1)
	cases := []struct {
		name, transformation, path, reason string
		kept                               int
	}{
		{
			"airport lookup in a spatial source",
			`{"id":"spatial","options":{"source":{"mode":"lookup","lookup":"code","gazetteer":"public/gazetteer/airports.geojson"}}}`,
			"transformations[0].options.source.gazetteer", airportsReason, 0,
		},
		{
			"airport lookup in a line target",
			`{"id":"spatial","options":{"action":"modify","modify":{"op":"lineBuilder","target":{"mode":"lookup","gazetteer":"public/gazetteer/airports.geojson"}}}}`,
			"transformations[0].options.modify.target.gazetteer", airportsReason, 0,
		},
		{
			"airport lookup in fieldLookup",
			`{"id":"fieldLookup","options":{"lookupField":"code","gazetteer":"public/gazetteer/airports.geojson"}}`,
			"transformations[0].options.gazetteer", airportsReason, 0,
		},
		{
			"gazetteer by URL",
			`{"id":"fieldLookup","options":{"gazetteer":"https://example.com/places.json"}}`,
			"transformations[0].options.gazetteer", gazetteerReason, 0,
		},
		{
			"unknown action",
			`{"id":"spatial","options":{"action":"merge"}}`,
			"transformations[0].options.action", "", 0,
		},
		{
			"unknown location mode",
			`{"id":"spatial","options":{"source":{"mode":"plus"}}}`,
			"transformations[0].options.source.mode", "", 1,
		},
		{
			"other location option",
			`{"id":"spatial","options":{"source":{"mode":"coords","altitude":"alt"}}}`,
			"transformations[0].options.source.altitude", "", 1,
		},
		{
			"field name that is too long",
			`{"id":"spatial","options":{"source":{"mode":"coords","latitude":"` + longName + `"}}}`,
			"transformations[0].options.source.latitude", "", 1,
		},
		{
			"unknown calculation",
			`{"id":"spatial","options":{"action":"calculate","calculate":{"calc":"speed"}}}`,
			"transformations[0].options.calculate.calc", "", 1,
		},
		{
			"other calculation option",
			`{"id":"spatial","options":{"action":"calculate","calculate":{"calc":"heading","endpoint":"last"}}}`,
			"transformations[0].options.calculate.endpoint", "", 1,
		},
		{
			"unknown modification",
			`{"id":"spatial","options":{"action":"modify","modify":{"op":"smooth"}}}`,
			"transformations[0].options.modify.op", "", 1,
		},
		{
			"other spatial option",
			`{"id":"spatial","options":{"precision":3}}`,
			"transformations[0].options.precision", "", 1,
		},
		{
			"other fieldLookup option",
			`{"id":"fieldLookup","options":{"outputName":"x"}}`,
			"transformations[0].options.outputName", "", 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			panel, line := reshapePanel(t, tc.transformation)
			if line.Status != statusPartial || !slices.Contains(line.Paths, tc.path) {
				t.Fatalf("status %s, paths %v, want %s reported", line.Status, line.Paths, tc.path)
			}
			if tc.reason != "" && !slices.Contains(line.Reasons, tc.reason) {
				t.Fatalf("reasons %v, want %q", line.Reasons, tc.reason)
			}
			if len(panel.Transform) != tc.kept {
				t.Fatalf("kept %d transformations, want %d: %+v", len(panel.Transform), tc.kept, panel.Transform)
			}
		})
	}
}
