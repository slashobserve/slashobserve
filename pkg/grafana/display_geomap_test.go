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
		{"maplibre is reported", `{"type":"maplibre","config":{"url":"https://example.com/style.json","accessToken":"secret"}}`, spec.Display{Basemap: "carto"}, []string{"options.basemap"}},
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
	if strings.Contains(string(data), "pk.secret") || len(line.Paths) == 0 {
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

func TestGeomapSingleMarkersLayerStaysFlatOnlyWithoutAFillOpacityAndALegend(t *testing.T) {
	const layer = `{"type":"markers","name":"Markers","tooltip":true,"location":{"mode":"coords","latitude":"lat","longitude":"lon"},"config":{"showLegend":false,"style":{"color":{"field":"v","fixed":"dark-green"},"size":{"field":"v","min":3,"max":20},"opacity":%s}}}`
	flat, line := geomapPanelWith(t, `{"view":{"id":"fit"},"layers":[`+strings.Replace(layer, "%s", "9", 1)+`]}`)
	if len(flat.Layers) != 0 || flat.Location != "coords" || flat.LatField != "lat" || flat.SizeField != "v" || flat.ColorField != "v" || flat.MarkerSize != 3 || flat.MarkerSizeMax != 20 {
		t.Fatalf("display = %+v", flat)
	}
	if !slices.Equal(line.Paths, []string{"options.layers[0].config.style.opacity"}) {
		t.Fatalf("paths %v", line.Paths)
	}
	layered, line := geomapPanelWith(t, `{"view":{"id":"fit"},"layers":[`+strings.Replace(layer, "%s", "0.4", 1)+`]}`)
	want := `[{"type":"markers","name":"Markers","location":"coords","latField":"lat","lonField":"lon","sizeField":"v","colorField":"v","markerSize":3,"markerSizeMax":20,"fillOpacity":0.4}]`
	if got := layersJSON(t, layered); got != want {
		t.Fatalf("layers = %s, want %s", got, want)
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
			`[{"type":"markers","name":"Hosts","query":"A","opacity":0.5,"tooltip":false,"color":"#F2495C","fillOpacity":0.4,"symbol":"square","textField":"label","textSize":14,"rotationField":"angle","showLegend":true}]`, nil},
		{"markers fill opacity",
			`{"type":"markers","name":"M","config":{"style":{"opacity":0.8}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.8,"showLegend":true}]`, nil},
		{"markers without a fill opacity import Grafana's own 0.4",
			`{"type":"markers","name":"M","config":{"style":{}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, nil},
		{"markers fill opacity past 1 is reported and left unwritten",
			`{"type":"markers","name":"M","config":{"style":{"opacity":3}}}`,
			`[{"type":"markers","name":"M","showLegend":true}]`, []string{"options.layers[0].config.style.opacity"}},
		{"markers symbol alignment",
			`{"type":"markers","name":"M","config":{"style":{"symbolAlign":{"horizontal":"left","vertical":"top"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"symbolAlign":"left","symbolVerticalAlign":"top","showLegend":true}]`, nil},
		{"markers symbol alignment of Grafana's default is silent",
			`{"type":"markers","name":"M","config":{"style":{"symbolAlign":{"horizontal":"center","vertical":"center"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, nil},
		{"markers outline width",
			`{"type":"markers","name":"M","location":{"mode":"auto"},"config":{"style":{"lineWidth":3}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"lineWidth":3,"showLegend":true}]`, nil},
		{"markers outline width of Grafana's default is silent",
			`{"type":"markers","name":"M","location":{"mode":"auto"},"config":{"style":{"lineWidth":1}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, nil},
		{"markers outline width past 20 is reported",
			`{"type":"markers","name":"M","config":{"style":{"lineWidth":25}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].config.style.lineWidth"}},
		{"fixed label with offsets, alignment and baseline",
			`{"type":"markers","name":"M","location":{"mode":"auto"},"config":{"style":{"text":{"mode":"fixed","fixed":"Pod"},"textConfig":{"fontSize":14,"offsetX":-5,"offsetY":8,"textAlign":"left","textBaseline":"top"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"text":"Pod","textSize":14,"textOffsetX":-5,"textOffsetY":8,"textAlign":"left","textBaseline":"top","showLegend":true}]`, nil},
		{"label placement of Grafana's defaults is silent",
			`{"type":"markers","name":"M","location":{"mode":"auto"},"config":{"style":{"text":{"mode":"fixed","fixed":""},"textConfig":{"offsetX":0,"offsetY":0,"textAlign":"center","textBaseline":"middle"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, nil},
		{"label offset past 100 is reported",
			`{"type":"markers","name":"M","config":{"style":{"textConfig":{"offsetX":150,"offsetY":-101}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].config.style.textConfig.offsetX", "options.layers[0].config.style.textConfig.offsetY"}},
		{"label alignment Grafana has but the Spec lacks is reported",
			`{"type":"markers","name":"M","config":{"style":{"textConfig":{"textAlign":"justify","textBaseline":"alphabetic"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].config.style.textConfig.textAlign", "options.layers[0].config.style.textConfig.textBaseline"}},
		{"fixed label past 200 characters is reported",
			`{"type":"markers","name":"M","config":{"style":{"text":{"fixed":"` + strings.Repeat("a", 201) + `"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].config.style.text.fixed"}},
		{"symbol from a field",
			`{"type":"markers","name":"M","location":{"mode":"auto"},"config":{"style":{"symbol":{"mode":"field","field":"icon","fixed":"img/icons/marker/circle.svg"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"symbolField":"icon","showLegend":true}]`, nil},
		{"a fixed symbol next to a symbol field is reported, never written with it",
			`{"type":"markers","name":"M","config":{"style":{"symbol":{"mode":"field","field":"icon","fixed":"img/icons/marker/square.svg"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"symbolField":"icon","showLegend":true}]`, []string{"options.layers[0].config.style.symbol.fixed"}},
		{"symbol field mode without a field is reported",
			`{"type":"markers","name":"M","config":{"style":{"symbol":{"mode":"field"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].config.style.symbol.mode"}},
		{"fixed rotation",
			`{"type":"markers","name":"M","location":{"mode":"auto"},"config":{"style":{"rotation":{"fixed":45,"mode":"mod","min":-360,"max":360}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"rotation":45,"showLegend":true}]`, nil},
		{"fixed rotation past 360 is reported",
			`{"type":"markers","name":"M","config":{"style":{"rotation":{"fixed":400}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].config.style.rotation.fixed"}},
		{"clamped rotation field",
			`{"type":"markers","name":"M","location":{"mode":"auto"},"config":{"style":{"rotation":{"field":"angle","mode":"clamped"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"rotationField":"angle","rotationMode":"clamp","showLegend":true}]`, nil},
		{"a fixed rotation next to a rotation field is reported, never written with it",
			`{"type":"markers","name":"M","config":{"style":{"rotation":{"field":"angle","fixed":45}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"rotationField":"angle","showLegend":true}]`, []string{"options.layers[0].config.style.rotation.fixed"}},
		{"clamped rotation without a field is reported",
			`{"type":"markers","name":"M","config":{"style":{"rotation":{"mode":"clamped"}}}}`,
			`[{"type":"markers","name":"M","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].config.style.rotation.mode"}},
		{"WKT location",
			`{"type":"markers","name":"M","location":{"mode":"wkt","wkt":"geometry"}}`,
			`[{"type":"markers","name":"M","location":"wkt","wktField":"geometry","fillOpacity":0.4,"showLegend":true}]`, nil},
		{"WKT location on a heatmap",
			`{"type":"heatmap","name":"H","location":{"mode":"wkt","wkt":"geometry"}}`,
			`[{"type":"heatmap","name":"H","location":"wkt","wktField":"geometry"}]`, nil},
		{"lookup with a gazetteer",
			`{"type":"markers","name":"M","location":{"mode":"lookup","lookup":"country","gazetteer":"public/gazetteer/usa-states.json"}}`,
			`[{"type":"markers","name":"M","location":"lookup","lookupField":"country","gazetteer":"usa-states","fillOpacity":0.4,"showLegend":true}]`, nil},
		{"airports lookup is imported and reported",
			`{"type":"markers","name":"M","location":{"mode":"lookup","lookup":"code","gazetteer":"public/gazetteer/airports.json"}}`,
			`[{"type":"markers","name":"M","location":"lookup","lookupField":"code","gazetteer":"airports","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].location.gazetteer"}},
		{"a custom gazetteer is reported",
			`{"type":"markers","name":"M","location":{"mode":"lookup","lookup":"code","gazetteer":"https://example.com/places.json"}}`,
			`[{"type":"markers","name":"M","location":"lookup","lookupField":"code","fillOpacity":0.4,"showLegend":true}]`, []string{"options.layers[0].location.gazetteer"}},
		{"heatmap",
			`{"type":"heatmap","name":"Heat","location":{"mode":"geohash","geohash":"hash"},"config":{"blur":20,"radius":10,"weight":{"field":"w","fixed":1,"min":0,"max":1}}}`,
			`[{"type":"heatmap","name":"Heat","location":"geohash","geohashField":"hash","weightField":"w","radius":10,"blur":20}]`, nil},
		{"heatmap with a fixed weight",
			`{"type":"heatmap","name":"H","config":{"weight":{"fixed":0.5,"min":0,"max":1}}}`,
			`[{"type":"heatmap","name":"H","weight":0.5}]`, nil},
		{"heatmap fixed weight above 1 is the same as 1 and stays silent",
			`{"type":"heatmap","name":"H","config":{"weight":{"fixed":2}}}`,
			`[{"type":"heatmap","name":"H"}]`, nil},
		{"heatmap fixed weight below 0 is reported",
			`{"type":"heatmap","name":"H","config":{"weight":{"fixed":-1}}}`,
			`[{"type":"heatmap","name":"H"}]`, []string{"options.layers[0].config.weight.fixed"}},
		{"heatmap weight range with a weight field",
			`{"type":"heatmap","name":"H","config":{"weight":{"field":"w","fixed":1,"min":0.2,"max":5}}}`,
			`[{"type":"heatmap","name":"H","weightField":"w","weightMin":0.2,"weightMax":5}]`, nil},
		{"heatmap weight range past 100 is reported",
			`{"type":"heatmap","name":"H","config":{"weight":{"field":"w","max":500}}}`,
			`[{"type":"heatmap","name":"H","weightField":"w"}]`, []string{"options.layers[0].config.weight.max"}},
		{"heatmap weight range without a weight field imports nothing and reports nothing",
			`{"type":"heatmap","name":"H","config":{"weight":{"min":0.2,"max":5}}}`,
			`[{"type":"heatmap","name":"H"}]`, nil},
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
		{"day and night showing the start of the range",
			`{"type":"dayNight","name":"N","config":{"show":"from"}}`,
			`[{"type":"dayNight","name":"N","nightAt":"from"}]`, nil},
		{"day and night showing something else is reported",
			`{"type":"dayNight","name":"N","config":{"show":"noon"}}`,
			`[{"type":"dayNight","name":"N"}]`, []string{"options.layers[0].config.show"}},
		{"day and night defaults are silent",
			`{"type":"dayNight","name":"N","config":{"show":"to","sun":false,"nightColor":"#a7a6ba4D"}}`,
			`[{"type":"dayNight","name":"N"}]`, nil},
		{"route",
			`{"type":"route","name":"R","location":{"mode":"auto"},"config":{"arrow":-1,"style":{"lineWidth":4}}}`,
			`[{"type":"route","name":"R","arrow":"reverse","lineWidth":4}]`, nil},
		{"photos",
			`{"type":"photos","name":"P","location":{"mode":"auto"},"config":{"src":"photo","kind":"circle","border":3}}`,
			`[{"type":"photos","name":"P","imageField":"photo","shape":"circle","border":3}]`, nil},
		{"photo border color and size",
			`{"type":"photos","name":"P","location":{"mode":"auto"},"config":{"src":"photo","color":"red","radius":20}}`,
			`[{"type":"photos","name":"P","color":"#F2495C","radius":20,"imageField":"photo"}]`, nil},
		{"photos cropped with a shadow",
			`{"type":"photos","name":"P","config":{"src":"photo","crop":true,"shadow":true}}`,
			`[{"type":"photos","name":"P","imageField":"photo","crop":true,"shadow":true}]`, nil},
		{"photos left whole and flat are silent",
			`{"type":"photos","name":"P","config":{"src":"photo","crop":false,"shadow":false}}`,
			`[{"type":"photos","name":"P","imageField":"photo"}]`, nil},
		{"photo size past 100 is reported",
			`{"type":"photos","name":"P","config":{"src":"photo","radius":150}}`,
			`[{"type":"photos","name":"P","imageField":"photo"}]`, []string{"options.layers[0].config.radius"}},
		{"photo frames are reported",
			`{"type":"photos","name":"P","config":{"kind":"folio"}}`,
			`[{"type":"photos","name":"P"}]`, []string{"options.layers[0].config.kind"}},
		{"network",
			`{"type":"network","name":"Net","location":{"mode":"auto"},"config":{"arrow":2,"edgeStyle":{"size":{"fixed":3},"color":{"fixed":"red"}},"style":{"color":{"fixed":"red"}}}}`,
			`[{"type":"network","name":"Net","color":"#F2495C","edgeColor":"#F2495C","arrow":"both","lineWidth":3}]`, nil},
		{"network edges in their own color",
			`{"type":"network","name":"Net","location":{"mode":"auto"},"config":{"edgeStyle":{"color":{"fixed":"blue"}},"style":{"color":{"fixed":"red"}}}}`,
			`[{"type":"network","name":"Net","color":"#F2495C","edgeColor":"#5794F2"}]`, nil},
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
	const secondLayer = `{"type":"markers","name":"Second","fillOpacity":0.4,"showLegend":true}`
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := geomapPanelWith(t, `{"view":{"id":"fit"},"layers":[`+tc.layer+`,`+second+`]}`)
			want := "[" + secondLayer + "]"
			if tc.want != "null" {
				want = strings.TrimSuffix(tc.want, "]") + "," + secondLayer + "]"
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

func TestGeomapMarkersLayerWithAWKTLocationKeepsItsLayer(t *testing.T) {
	d, line := geomapPanelWith(t, `{"layers":[{"type":"markers","name":"M","location":{"mode":"wkt","wkt":"geometry"}}]}`)
	if d.Location != "" || len(d.Layers) != 1 || d.Layers[0].Location != "wkt" || d.Layers[0].WktField != "geometry" || line.Status != statusImported {
		t.Fatalf("display %+v paths %v", d, line.Paths)
	}
}

func TestGeomapSymbolFieldAndFixedSymbolAreNeverBothWritten(t *testing.T) {
	for _, style := range []string{
		`{"symbol":{"mode":"field","field":"icon","fixed":"img/icons/marker/square.svg"}}`,
		`{"symbol":{"mode":"fixed","field":"icon","fixed":"img/icons/marker/square.svg"}}`,
		`{"symbol":{"mode":"field","field":"icon"}}`,
	} {
		d, _ := geomapPanelWith(t, `{"layers":[{"type":"markers","name":"M","config":{"style":`+style+`}},{"type":"markers","name":"N"}]}`)
		if layer := d.Layers[0]; layer.Symbol != "" && layer.SymbolField != "" {
			t.Errorf("%s wrote symbol %q and symbolField %q", style, layer.Symbol, layer.SymbolField)
		}
	}
}

func TestGeomapLayersAreCappedAtTen(t *testing.T) {
	layer := `{"type":"heatmap","name":"H"}`
	d, line := geomapPanelWith(t, `{"layers":[`+strings.Repeat(layer+",", 11)+layer+`]}`)
	if len(d.Layers) != maxGeomapLayers || !slices.Contains(line.Paths, "options.layers[10]") {
		t.Fatalf("layers %d paths %v", len(d.Layers), line.Paths)
	}
}

func TestGeomapViewNoRepeat(t *testing.T) {
	cases := []struct {
		name, view string
		want       *bool
	}{
		{"true is imported", `{"id":"fit","noRepeat":true}`, ptr(true)},
		{"false is Grafana's default and silent", `{"id":"fit","noRepeat":false}`, nil},
		{"absent", `{"id":"fit"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := geomapPanelWith(t, `{"view":`+tc.view+`}`)
			if (d.NoRepeat == nil) != (tc.want == nil) || (tc.want != nil && *d.NoRepeat != *tc.want) {
				t.Errorf("noRepeat = %v, want %v", d.NoRepeat, tc.want)
			}
			if line.Status != statusImported {
				t.Errorf("paths %v", line.Paths)
			}
		})
	}
}

func TestGeomapXYZBasemapTileZoomLimits(t *testing.T) {
	const url = `"url":"https://tiles.example.com/{z}/{x}/{y}.png"`
	cases := []struct {
		name, basemap string
		low, high     *int
		paths         []string
	}{
		{"both limits", `{"type":"xyz","config":{` + url + `,"minZoom":1,"maxZoom":12}}`, ptr(1), ptr(12), nil},
		{"only the highest", `{"type":"xyz","config":{` + url + `,"maxZoom":9}}`, nil, ptr(9), nil},
		{"zero is a limit", `{"type":"xyz","config":{` + url + `,"minZoom":0}}`, ptr(0), nil, nil},
		{"above the supported range is reported", `{"type":"xyz","config":{` + url + `,"maxZoom":25}}`, nil, nil, []string{"options.basemap.config.maxZoom"}},
		{"a minimum above the maximum reports both", `{"type":"xyz","config":{` + url + `,"minZoom":8,"maxZoom":3}}`, nil, nil, []string{"options.basemap.config.maxZoom", "options.basemap.config.minZoom"}},
		{"a custom Esri server is an xyz basemap", `{"type":"esri-xyz","config":{"server":"custom",` + url + `,"minZoom":2,"maxZoom":6}}`, ptr(2), ptr(6), nil},
		{"CARTO has no tile limits to import", `{"type":"carto","config":{"minZoom":2}}`, nil, nil, []string{"options.basemap.config.minZoom"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := geomapPanelWith(t, `{"view":{"id":"fit"},"basemap":`+tc.basemap+`}`)
			if !equalInts(d.BasemapMinZoom, tc.low) || !equalInts(d.BasemapMaxZoom, tc.high) {
				t.Errorf("limits = %v, %v, want %v, %v", d.BasemapMinZoom, d.BasemapMaxZoom, tc.low, tc.high)
			}
			slices.Sort(line.Paths)
			if !slices.Equal(line.Paths, tc.paths) {
				t.Errorf("paths = %v, want %v", line.Paths, tc.paths)
			}
		})
	}
}

func equalInts(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func TestGeomapTileLayerZoomLimitsStayReported(t *testing.T) {
	_, line := geomapPanelWith(t, `{"layers":[{"type":"xyz","name":"T","config":{"url":"https://tiles.example.com/{z}/{x}/{y}.png","minZoom":3}}]}`)
	if !slices.Contains(line.Paths, "options.layers[0].config.minZoom") {
		t.Fatalf("paths %v", line.Paths)
	}
}

func TestGeomapMarkersLayerLegend(t *testing.T) {
	cases := []struct {
		name, config string
		want         *bool
	}{
		{"absent is Grafana's default of shown", `{}`, ptr(true)},
		{"true", `{"showLegend":true}`, ptr(true)},
		{"false", `{"showLegend":false}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, line := geomapPanelWith(t, `{"layers":[{"type":"markers","name":"M","config":`+tc.config+`}]}`)
			if len(d.Layers) != 1 {
				t.Fatalf("display = %+v", d)
			}
			if got := d.Layers[0].ShowLegend; (got == nil) != (tc.want == nil) || (tc.want != nil && *got != *tc.want) {
				t.Errorf("showLegend = %v, want %v", got, tc.want)
			}
			if line.Status != statusImported {
				t.Errorf("paths %v", line.Paths)
			}
		})
	}
}

func TestGeomapMarkersLayerWithALegendIsNotFlat(t *testing.T) {
	layer := spec.GeomapLayer{Type: "markers", ColorField: "v", ShowLegend: ptr(true)}
	if flatMarkers(layer) {
		t.Fatal("a layer with a legend is flat")
	}
	layer.ShowLegend = nil
	if !flatMarkers(layer) {
		t.Fatal("a layer without a legend or fill opacity is not flat")
	}
}
