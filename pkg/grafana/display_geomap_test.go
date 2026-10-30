package grafana

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func geomapPanelWith(t *testing.T, options string) (spec.Display, PanelLine) {
	t.Helper()
	panel, line := convertedPanel(t, panelDoc("geomap", upTarget+`,"options":`+options))
	return displayOf(t, panel), line
}

func layersJSON(t *testing.T, d spec.Display) string {
	t.Helper()
	data, err := json.Marshal(d.Layers)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGeomapBasemapTypes(t *testing.T) {
	cases := []struct {
		name, basemap string
		want          spec.Display
		paths         []string
	}{
		{"absent basemap is CARTO", ``, spec.Display{Basemap: "carto"}, nil},
		{"default type is CARTO", `{"type":"default","config":{}}`, spec.Display{Basemap: "carto"}, nil},
		{"carto theme and labels", `{"type":"carto","config":{"theme":"dark","showLabels":false}}`, spec.Display{Basemap: "carto", BasemapTheme: "dark", BasemapLabels: ptr(false)}, nil},
		{"opacity", `{"type":"carto","opacity":0.5}`, spec.Display{Basemap: "carto", BasemapOpacity: ptr(0.5)}, nil},
		{"osm", `{"type":"osm-standard"}`, spec.Display{Basemap: "osm"}, nil},
		{"esri world imagery", `{"type":"esri-xyz","config":{"server":"world-imagery"}}`, spec.Display{Basemap: "esri", EsriServer: "world-imagery"}, nil},
		{"esri custom server becomes xyz", `{"type":"esri-xyz","config":{"server":"custom","url":"https://tiles.example.com/{z}/{x}/{y}.png","attribution":"Example"}}`, spec.Display{Basemap: "xyz", BasemapURL: "https://tiles.example.com/{z}/{x}/{y}.png", BasemapAttribution: "Example"}, nil},
		{"xyz", `{"type":"xyz","config":{"url":"https://tiles.example.com/{z}/{x}/{y}.png"}}`, spec.Display{Basemap: "xyz", BasemapURL: "https://tiles.example.com/{z}/{x}/{y}.png"}, nil},
		{"xyz without an address draws nothing", `{"type":"xyz","config":{}}`, spec.Display{Basemap: "none"}, nil},
		{"xyz over http is upgraded and reported", `{"type":"xyz","config":{"url":"http://tiles.example.com/{z}/{x}/{y}.png"}}`, spec.Display{Basemap: "xyz", BasemapURL: "https://tiles.example.com/{z}/{x}/{y}.png"}, []string{"options.basemap.config.url"}},
		{"xyz with a variable host is reported", `{"type":"xyz","config":{"url":"https://${host}/{z}/{x}/{y}.png"}}`, spec.Display{Basemap: "carto"}, []string{"options.basemap.config.url", "options.basemap"}},
		{"xyz without coordinates is reported", `{"type":"xyz","config":{"url":"https://tiles.example.com/tile.png"}}`, spec.Display{Basemap: "carto"}, []string{"options.basemap.config.url", "options.basemap"}},
		{"maplibre is reported", `{"type":"maplibre","config":{"url":"https://example.com/style.json","accessToken":"secret"}}`, spec.Display{Basemap: "carto"}, []string{"options.basemap.config"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := `{"view":{"id":"fit"}}`
			if tc.basemap != "" {
				options = `{"view":{"id":"fit"},"basemap":` + tc.basemap + `}`
			}
			d, line := geomapPanelWith(t, options)
			got := spec.Display{Basemap: d.Basemap, BasemapTheme: d.BasemapTheme, BasemapLabels: d.BasemapLabels, EsriServer: d.EsriServer, BasemapURL: d.BasemapURL, BasemapAttribution: d.BasemapAttribution, BasemapOpacity: d.BasemapOpacity}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("basemap = %s, want %s", gotJSON, wantJSON)
			}
			slices.Sort(line.Paths)
			want := slices.Clone(tc.paths)
			slices.Sort(want)
			if !slices.Equal(line.Paths, want) {
				t.Errorf("paths = %v, want %v", line.Paths, want)
			}
		})
	}
}

func TestGeomapBasemapNeverImportsAnAccessToken(t *testing.T) {
	d, line := geomapPanelWith(t, `{"basemap":{"type":"maplibre","config":{"url":"https://example.com/style.json","accessToken":"pk.secret"}}}`)
	data, _ := json.Marshal(d)
	if strings.Contains(string(data), "pk.secret") || line.Status != statusPartial {
		t.Fatalf("display %s status %s", data, line.Status)
	}
}

func TestGeomapViewControlsAndTooltip(t *testing.T) {
	d, line := geomapPanelWith(t, `{"view":{"id":"south-east-asia","minZoom":2,"maxZoom":9,"allLayers":false,"layer":"Layer 1","lastOnly":true,"padding":20,"shared":true},"controls":{"showZoom":false,"mouseWheelZoom":false,"showAttribution":false,"showScale":true,"showMeasure":true,"showDebug":true},"tooltip":{"mode":"none"}}`)
	if d.View != "south-east-asia" || *d.MinZoom != 2 || *d.MaxZoom != 9 || d.FitLayer != "Layer 1" || !*d.FitLastOnly || *d.FitPadding != 20 || !*d.SharedView {
		t.Errorf("view keys = %+v", d)
	}
	if d.ShowZoom == nil || *d.ShowZoom || d.MouseWheelZoom != nil || d.ShowAttribution == nil || *d.ShowAttribution || !*d.ShowScale || !*d.ShowMeasure || !*d.ShowDebug {
		t.Errorf("controls = %+v", d)
	}
	if d.Tooltip != "none" || line.Status != statusImported {
		t.Errorf("tooltip %q status %s paths %v", d.Tooltip, line.Status, line.Paths)
	}
}

func TestGeomapWheelZoomIsOnUnlessTheDashboardTurnsItOff(t *testing.T) {
	d, _ := geomapPanelWith(t, `{"view":{"id":"fit"}}`)
	if d.MouseWheelZoom == nil || !*d.MouseWheelZoom {
		t.Fatalf("mouseWheelZoom = %v, want true", d.MouseWheelZoom)
	}
}

func TestGeomapSingleMarkersLayerStaysFlat(t *testing.T) {
	d, line := geomapPanelWith(t, `{"view":{"id":"fit"},"layers":[{"type":"markers","name":"Markers","tooltip":true,"location":{"mode":"coords","latitude":"lat","longitude":"lon"},"config":{"style":{"color":{"field":"v","fixed":"dark-green"},"size":{"field":"v","min":3,"max":20}}}}]}`)
	if len(d.Layers) != 0 || d.Location != "coords" || d.LatField != "lat" || d.SizeField != "v" || d.ColorField != "v" || d.MarkerSize != 3 || d.MarkerSizeMax != 20 {
		t.Fatalf("display = %+v", d)
	}
	if line.Status != statusImported {
		t.Fatalf("paths %v", line.Paths)
	}
}

func TestGeomapLayerTypes(t *testing.T) {
	cases := []struct {
		name, layer, want string
		paths             []string
	}{
		{"markers with a fixed color, symbol, text and rotation",
			`{"type":"markers","name":"Hosts","opacity":0.5,"tooltip":false,"filterData":{"id":"byRefId","options":"A"},"location":{"mode":"auto"},"config":{"style":{"color":{"fixed":"red"},"symbol":{"mode":"fixed","fixed":"img/icons/marker/square.svg"},"rotation":{"field":"angle","mode":"mod","fixed":0,"min":-360,"max":360},"text":{"mode":"field","field":"label"},"textConfig":{"fontSize":14}}}}`,
			`[{"type":"markers","name":"Hosts","query":"A","opacity":0.5,"tooltip":false,"color":"#F2495C","symbol":"square","textField":"label","textSize":14,"rotationField":"angle"}]`, nil},
		{"lookup with a gazetteer",
			`{"type":"markers","name":"M","location":{"mode":"lookup","lookup":"country","gazetteer":"public/gazetteer/usa-states.json"}}`,
			`[{"type":"markers","name":"M","location":"lookup","lookupField":"country","gazetteer":"usa-states"}]`, nil},
		{"airports lookup is imported and reported",
			`{"type":"markers","name":"M","location":{"mode":"lookup","lookup":"code","gazetteer":"public/gazetteer/airports.json"}}`,
			`[{"type":"markers","name":"M","location":"lookup","lookupField":"code","gazetteer":"airports"}]`, []string{"options.layers[0].location.gazetteer"}},
		{"a custom gazetteer is reported",
			`{"type":"markers","name":"M","location":{"mode":"lookup","lookup":"code","gazetteer":"https://example.com/places.json"}}`,
			`[{"type":"markers","name":"M","location":"lookup","lookupField":"code"}]`, []string{"options.layers[0].location.gazetteer"}},
		{"heatmap",
			`{"type":"heatmap","name":"Heat","location":{"mode":"geohash","geohash":"hash"},"config":{"blur":20,"radius":10,"weight":{"field":"w","fixed":1,"min":0,"max":1}}}`,
			`[{"type":"heatmap","name":"Heat","location":"geohash","geohashField":"hash","weightField":"w","radius":10,"blur":20}]`, nil},
		{"geojson from the bundled countries",
			`{"type":"geojson","name":"G","location":{"mode":"auto"},"config":{"src":"public/maps/countries.geojson","rules":[{"property":"name","operation":"eq","value":"Peru","style":{"color":{"fixed":"red"},"opacity":0.7}}]}}`,
			`[{"type":"geojson","name":"G","source":"countries","rules":[{"property":"name","operation":"eq","value":"Peru","color":"#F2495C","opacity":0.7}]}]`, nil},
		{"geojson from an https address and a numeric rule",
			`{"type":"geojson","name":"G","config":{"src":"https://example.com/shapes.geojson","rules":[{"property":"pop","operation":"gt","value":10}]}}`,
			`[{"type":"geojson","name":"G","url":"https://example.com/shapes.geojson","rules":[{"property":"pop","operation":"gt","value":10}]}]`, nil},
		{"geojson from a relative path is reported",
			`{"type":"geojson","name":"G","config":{"src":"public/maps/other.geojson"}}`,
			`[{"type":"geojson","name":"G"}]`, []string{"options.layers[0].config.src"}},
		{"day and night with the sun",
			`{"type":"dayNight","name":"N","location":{"mode":"auto"},"config":{"show":"to","sun":true,"nightColor":"#000000"}}`,
			`[{"type":"dayNight","name":"N","showSun":true,"nightColor":"#000000"}]`, nil},
		{"day and night defaults are silent",
			`{"type":"dayNight","name":"N","config":{"show":"to","sun":false,"nightColor":"#a7a6ba4D"}}`,
			`[{"type":"dayNight","name":"N"}]`, nil},
		{"route",
			`{"type":"route","name":"R","location":{"mode":"auto"},"config":{"arrow":-1,"style":{"lineWidth":4}}}`,
			`[{"type":"route","name":"R","arrow":"reverse","lineWidth":4}]`, nil},
		{"photos",
			`{"type":"photos","name":"P","location":{"mode":"auto"},"config":{"src":"photo","kind":"circle","border":3}}`,
			`[{"type":"photos","name":"P","imageField":"photo","shape":"circle","border":3}]`, nil},
		{"photo frames are reported",
			`{"type":"photos","name":"P","config":{"kind":"folio"}}`,
			`[{"type":"photos","name":"P"}]`, []string{"options.layers[0].config.kind"}},
		{"network",
			`{"type":"network","name":"Net","location":{"mode":"auto"},"config":{"arrow":2,"edgeStyle":{"size":{"fixed":3},"color":{"fixed":"red"}},"style":{"color":{"fixed":"red"}}}}`,
			`[{"type":"network","name":"Net","color":"#F2495C","arrow":"both","lineWidth":3}]`, nil},
		{"carto tiles",
			`{"type":"carto","name":"Tiles","config":{"theme":"dark","showLabels":false}}`,
			`[{"type":"carto","name":"Tiles","theme":"dark","labels":false}]`, nil},
		{"osm tiles",
			`{"type":"osm-standard","name":"Tiles"}`,
			`[{"type":"osm","name":"Tiles"}]`, nil},
		{"esri tiles",
			`{"type":"esri-xyz","name":"Tiles","config":{"server":"topo"}}`,
			`[{"type":"esri","name":"Tiles","server":"topo"}]`, nil},
		{"xyz tiles",
			`{"type":"xyz","name":"Tiles","config":{"url":"https://tiles.example.com/{z}/{x}/{y}.png","attribution":"Example"}}`,
			`[{"type":"xyz","name":"Tiles","url":"https://tiles.example.com/{z}/{x}/{y}.png","attribution":"Example"}]`, nil},
		{"an unknown layer type is left out and reported",
			`{"type":"last-point-tracker","name":"T"}`,
			`null`, []string{"options.layers[0]"}},
	}
	const second = `{"type":"markers","name":"Second","location":{"mode":"auto"}}`
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := geomapPanelWith(t, `{"view":{"id":"fit"},"layers":[`+tc.layer+`,`+second+`]}`)
			want := tc.want
			if want != "null" {
				want = strings.TrimSuffix(want, "]") + `,{"type":"markers","name":"Second"}]`
			}
			if got := layersJSON(t, d); got != want {
				t.Errorf("layers = %s\nwant     %s", got, want)
			}
			slices.Sort(line.Paths)
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v (reasons %v)", line.Paths, tc.paths, line.Reasons)
			}
		})
	}
}

func TestGeomapLayersAreCappedAtTen(t *testing.T) {
	layer := `{"type":"heatmap","name":"H"}`
	d, line := geomapPanelWith(t, `{"layers":[`+strings.Repeat(layer+",", 11)+layer+`]}`)
	if len(d.Layers) != maxGeomapLayers || !slices.Contains(line.Paths, "options.layers[10]") {
		t.Fatalf("layers %d paths %v", len(d.Layers), line.Paths)
	}
}
