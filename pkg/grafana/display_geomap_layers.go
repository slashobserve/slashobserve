package grafana

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	geomapLocateReason      = "This way of locating points is not supported yet."
	geomapLayerReason       = "This layer type is not supported yet."
	geomapAirportReason     = "Airport lookup is not available."
	geomapGazetteerReason   = "Only the bundled country and state gazetteers are available."
	geomapGeojsonReason     = "Only the bundled country and state maps and https addresses are available."
	geomapSymbolReason      = "This marker symbol is not supported yet."
	geomapPhotoKindReason   = "Photos are drawn as plain squares."
	geomapQueryRefReason    = "A layer can only be fed by one of the panel queries."
	geomapRuleReason        = "This GeoJSON rule is not supported yet."
	geomapTileURLReason     = "A tile address must be an https address written out in full with {z}, {x} and {y}."
	geomapTileHTTPReason    = "Tiles are loaded over https, so the http address was changed."
	geomapBasemapReason     = "This basemap is not supported yet, so the CARTO map is used."
	geomapServerReason      = "This Esri server is not supported yet."
	maxGeomapRules          = 20
	maxGeomapAddress        = 2000
	maxLayerLineWidth       = 20
	maxHeatmapRadius        = 100
	maxPhotoBorder          = 20
	minTextSize             = 6
	maxMarkerText           = 48
	geomapDefaultRadius     = 15
	geomapDefaultBlur       = 15
	geomapDefaultBorder     = 2
	geomapDefaultTextSize   = 12
	geomapDefaultMarkerSize = 5
	geomapDefaultMarkerMax  = 15
	routeDefaultWidth       = 2
	networkDefaultWidth     = 1
	markerPrefix            = "img/icons/marker/"
)

var httpsAddress = regexp.MustCompile(`^https://[A-Za-z0-9.-]+(:443)?(/[^\s]*)?$`)

var geomapLayerTypes = map[string]string{
	"markers":      "markers",
	"heatmap":      "heatmap",
	"geojson":      "geojson",
	"dayNight":     "dayNight",
	"route":        "route",
	"photos":       "photos",
	"network":      "network",
	"carto":        "carto",
	"osm-standard": "osm",
	"esri-xyz":     "esri",
	"xyz":          "xyz",
}

var geomapSymbols = []string{"circle", "square", "triangle", "star", "cross", "x"}

var geomapGazetteers = map[string]string{
	"countries":  "countries",
	"usa-states": "usa-states",
	"airports":   "airports",
}

var geomapGeojsonSources = map[string]string{
	"public/maps/countries.geojson":  "countries",
	"public/maps/usa-states.geojson": "usa-states",
}

var geomapEsriServers = []string{"streets", "world-imagery", "world-physical", "topo", "usa-topo", "ocean"}

var geomapArrows = map[int]string{0: "none", 1: "forward", -1: "reverse", 2: "both"}

var geomapRuleOperations = []string{"eq", "neq", "lt", "lte", "gt", "gte"}

func literalHTTPS(address string) bool {
	return len(address) <= maxGeomapAddress && httpsAddress.MatchString(address)
}

func tileAddress(address string) bool {
	return literalHTTPS(address) && !slices.ContainsFunc([]string{"{z}", "{x}", "{y}"}, func(part string) bool {
		return !strings.Contains(address, part)
	})
}

type tileSettings struct {
	kind        string
	theme       string
	labels      *bool
	server      string
	address     string
	attribution string
}

func (b *panelBuild) geomapBasemap(d *spec.Display) {
	const base = "options.basemap"
	kind := asString(lookupValue(b.raw, base+".type"))
	b.consume(base + ".type")
	switch kind {
	case "", "default":
		kind = "carto"
	case "maplibre":
		b.partial(base+".config", geomapBasemapReason)
		kind = "carto"
	}
	tile, ok := b.tileSettings(base+".config", kind)
	if !ok {
		tile = tileSettings{kind: "carto"}
		b.partial(base, geomapBasemapReason)
	}
	d.Basemap = tile.kind
	d.BasemapTheme = tile.theme
	d.BasemapLabels = tile.labels
	d.EsriServer = tile.server
	d.BasemapURL = tile.address
	d.BasemapAttribution = tile.attribution
	d.BasemapOpacity = b.boundedOption(base+".opacity", 0, 1)
}

func (b *panelBuild) tileSettings(config, kind string) (tileSettings, bool) {
	switch kind {
	case "carto":
		return b.cartoSettings(config), true
	case "osm-standard", "osm":
		return tileSettings{kind: "osm"}, true
	case "esri-xyz", "esri":
		return b.esriSettings(config)
	case "xyz":
		return b.xyzSettings(config)
	}
	return tileSettings{}, false
}

func (b *panelBuild) cartoSettings(config string) tileSettings {
	tile := tileSettings{kind: "carto"}
	tile.theme = b.enumOption(config+".theme", "auto", "light", "dark")
	tile.labels, _ = b.boolOption(config + ".showLabels")
	return tile
}

func (b *panelBuild) esriSettings(config string) (tileSettings, bool) {
	server := asString(lookupValue(b.raw, config+".server"))
	switch {
	case server == "custom":
		b.consume(config + ".server")
		return b.xyzSettings(config)
	case server == "":
		return tileSettings{kind: "esri"}, true
	case slices.Contains(geomapEsriServers, server):
		b.consume(config + ".server")
		return tileSettings{kind: "esri", server: server}, true
	}
	b.partial(config+".server", geomapServerReason)
	return tileSettings{kind: "esri"}, true
}

func (b *panelBuild) xyzSettings(config string) (tileSettings, bool) {
	if asString(lookupValue(b.raw, config+".url")) == "" {
		return tileSettings{kind: noBasemap}, true
	}
	address, ok := b.tileAddress(config + ".url")
	if !ok {
		return tileSettings{}, false
	}
	tile := tileSettings{kind: "xyz", address: address}
	if v, has := b.take(config + ".attribution"); has {
		if text := asString(v); text != "" && validLength(text, maxGeomapAttribution) {
			tile.attribution = text
		}
	}
	return tile, true
}

const (
	maxGeomapAttribution = 200
	noBasemap            = "none"
)

func (b *panelBuild) tileAddress(at string) (string, bool) {
	address := strings.TrimSpace(asString(lookupValue(b.raw, at)))
	if rest, upgraded := strings.CutPrefix(address, "http://"); upgraded {
		address = "https://" + rest
		b.issues.add(at, geomapTileHTTPReason)
	}
	if !tileAddress(address) {
		b.partial(at, geomapTileURLReason)
		return "", false
	}
	b.consume(at)
	return address, true
}

func (b *panelBuild) geomapLayers(d *spec.Display) {
	items, _ := asArray(lookupValue(b.raw, "options.layers"))
	var layers []spec.GeomapLayer
	for i, item := range items {
		at := fmt.Sprintf("options.layers[%d]", i)
		if len(layers) >= maxGeomapLayers {
			b.partial(at, "Only the first 10 layers are drawn.")
			continue
		}
		if _, isObject := asObject(item); !isObject {
			continue
		}
		if layer, ok := b.geomapLayer(at); ok {
			layers = append(layers, layer)
		}
	}
	if len(layers) == 1 && flatMarkers(layers[0]) {
		d.Location = layers[0].Location
		d.LatField = layers[0].LatField
		d.LonField = layers[0].LonField
		d.GeohashField = layers[0].GeohashField
		d.SizeField = layers[0].SizeField
		d.ColorField = layers[0].ColorField
		d.MarkerSize = layers[0].MarkerSize
		d.MarkerSizeMax = layers[0].MarkerSizeMax
		return
	}
	d.Layers = layers
}

func flatMarkers(layer spec.GeomapLayer) bool {
	rest := layer
	rest.Name = ""
	rest.Location, rest.LatField, rest.LonField, rest.GeohashField = "", "", "", ""
	rest.SizeField, rest.ColorField, rest.MarkerSize, rest.MarkerSizeMax = "", "", 0, 0
	return layer.Type == "markers" && layer.Location != "lookup" && reflect.DeepEqual(rest, spec.GeomapLayer{Type: "markers"})
}

func (b *panelBuild) geomapLayer(at string) (spec.GeomapLayer, bool) {
	kind := asString(lookupValue(b.raw, at+".type"))
	typ, known := geomapLayerTypes[kind]
	if !known {
		b.partial(at, geomapLayerReason)
		return spec.GeomapLayer{}, false
	}
	b.consume(at + ".type")
	layer := spec.GeomapLayer{Type: typ}
	b.layerCommon(&layer, at)
	config := at + ".config"
	switch typ {
	case "markers":
		b.layerLocation(&layer, at+".location")
		b.markerStyle(&layer, config+".style")
	case "heatmap":
		b.layerLocation(&layer, at+".location")
		b.heatmapConfig(&layer, config)
	case "route":
		b.layerLocation(&layer, at+".location")
		b.routeConfig(&layer, config)
	case "photos":
		b.layerLocation(&layer, at+".location")
		b.photosConfig(&layer, config)
	case "network":
		b.layerLocation(&layer, at+".location")
		b.networkConfig(&layer, config)
	case "geojson":
		b.geojsonConfig(&layer, config)
	case "dayNight":
		b.dayNightConfig(&layer, config)
	default:
		if !b.tileLayer(&layer, typ, config) {
			return spec.GeomapLayer{}, false
		}
	}
	if !slices.Contains(locatingLayerTypes, typ) {
		b.consumeAutoLocation(at + ".location")
	}
	trimLayerDefaults(&layer)
	return layer, true
}

var locatingLayerTypes = []string{"markers", "heatmap", "route", "photos", "network"}

func (b *panelBuild) consumeAutoLocation(at string) {
	if mode := asString(lookupValue(b.raw, at+".mode")); mode == "auto" || mode == "" {
		b.consume(at)
	}
}

func (b *panelBuild) layerCommon(layer *spec.GeomapLayer, at string) {
	if name := asString(lookupValue(b.raw, at+".name")); validLength(name, maxGeomapName) {
		layer.Name = name
	}
	b.consume(at + ".name")
	layer.Opacity = b.boundedOption(at+".opacity", 0, 1)
	if shown, ok := b.boolOption(at + ".tooltip"); ok && !*shown {
		layer.Tooltip = shown
	}
	b.layerQuery(layer, at+".filterData")
}

func (b *panelBuild) layerQuery(layer *spec.GeomapLayer, at string) {
	raw, has := b.value(at)
	if !has {
		return
	}
	filter, _ := asObject(raw)
	ref, isText := filter["options"].(string)
	if known, found := b.refs[ref]; asString(filter["id"]) == "byRefId" && isText && found {
		layer.Query = known
		b.consume(at)
		return
	}
	b.partial(at, geomapQueryRefReason)
}

func (b *panelBuild) layerLocation(layer *spec.GeomapLayer, at string) {
	mode := asString(lookupValue(b.raw, at+".mode"))
	switch mode {
	case "coords":
		layer.Location = mode
		layer.LatField = b.fieldNameOption(at + ".latitude")
		layer.LonField = b.fieldNameOption(at + ".longitude")
	case "geohash":
		layer.Location = mode
		layer.GeohashField = b.fieldNameOption(at + ".geohash")
	case "lookup":
		layer.Location = mode
		layer.LookupField = b.fieldNameOption(at + ".lookup")
		layer.Gazetteer = b.gazetteer(at + ".gazetteer")
	case "auto", "":
	default:
		b.partial(at+".mode", geomapLocateReason)
	}
	b.consume(at + ".mode")
	b.consume(at+".latitude", at+".longitude", at+".geohash")
	if mode != "lookup" {
		b.consume(at+".lookup", at+".gazetteer")
	}
}

func (b *panelBuild) gazetteer(at string) string {
	v, has := b.value(at)
	if !has {
		return ""
	}
	name := strings.TrimSuffix(path.Base(asString(v)), ".json")
	kind, bundled := geomapGazetteers[name]
	if !bundled || path.Dir(asString(v)) != "public/gazetteer" {
		b.partial(at, geomapGazetteerReason)
		return ""
	}
	b.consume(at)
	if kind == "airports" {
		b.issues.add(at, geomapAirportReason)
	}
	return kind
}

func (b *panelBuild) layerColor(at string) string {
	v, has := b.take(at)
	name := asString(v)
	if !has || name == "" || isGrafanaDefault(b.typ, at, v) {
		return ""
	}
	color, ok := seriesColor(name)
	if !ok {
		b.partial(at, fmt.Sprintf("The color %q is not supported yet.", name))
	}
	return color
}

func (b *panelBuild) markerStyle(layer *spec.GeomapLayer, style string) {
	layer.SizeField = b.fieldNameOption(style + ".size.field")
	if layer.SizeField != "" {
		layer.MarkerSize = b.markerSizeOption(style + ".size.min")
		layer.MarkerSizeMax = b.markerSizeOption(style + ".size.max")
		b.consume(style + ".size.fixed")
	} else {
		layer.MarkerSize = b.markerSizeOption(style + ".size.fixed")
		b.consume(style+".size.min", style+".size.max")
	}
	layer.ColorField = b.fieldNameOption(style + ".color.field")
	if fixed := b.layerColor(style + ".color.fixed"); layer.ColorField == "" {
		layer.Color = fixed
	}
	layer.Symbol = b.markerSymbol(style + ".symbol")
	layer.RotationField = b.fieldNameOption(style + ".rotation.field")
	b.markerText(layer, style)
}

func (b *panelBuild) markerSymbol(at string) string {
	if asString(lookupValue(b.raw, at+".mode")) == "field" {
		return ""
	}
	v, has := b.value(at + ".fixed")
	if !has {
		return ""
	}
	icon := asString(v)
	name := strings.TrimSuffix(strings.TrimPrefix(icon, markerPrefix), ".svg")
	if strings.HasPrefix(icon, markerPrefix) && slices.Contains(geomapSymbols, name) {
		b.consume(at + ".fixed")
		return name
	}
	if icon != "" {
		b.partial(at+".fixed", geomapSymbolReason)
	}
	return ""
}

func (b *panelBuild) markerText(layer *spec.GeomapLayer, style string) {
	if asString(lookupValue(b.raw, style+".text.mode")) == "field" {
		layer.TextField = b.fieldNameOption(style + ".text.field")
		b.consume(style + ".text.mode")
	}
	if size, ok := b.boundedWhole(style+".textConfig.fontSize", minTextSize, maxMarkerText); ok {
		layer.TextSize = size
	}
}

var heatmapWeightDefaults = map[string]float64{"fixed": 1, "min": 0, "max": 1}

func (b *panelBuild) heatmapConfig(layer *spec.GeomapLayer, config string) {
	layer.WeightField = b.fieldNameOption(config + ".weight.field")
	for key, standard := range heatmapWeightDefaults {
		if v, has := b.value(config + ".weight." + key); has && sameJSON(v, standard) {
			b.consume(config + ".weight." + key)
		}
	}
	if radius, ok := b.boundedWhole(config+".radius", 1, maxHeatmapRadius); ok {
		layer.Radius = radius
	}
	if blur, ok := b.boundedWhole(config+".blur", 0, maxHeatmapRadius); ok {
		layer.Blur = &blur
	}
}

func (b *panelBuild) arrow(at string) string {
	v, has := b.value(at)
	if !has {
		return ""
	}
	n, isInt := asInt(v)
	arrow, known := geomapArrows[n]
	if !isInt || !known {
		b.partial(at, "This arrow setting is not supported yet.")
		return ""
	}
	b.consume(at)
	return arrow
}

func (b *panelBuild) routeConfig(layer *spec.GeomapLayer, config string) {
	layer.Arrow = b.arrow(config + ".arrow")
	if width, ok := b.boundedWhole(config+".style.lineWidth", 1, maxLayerLineWidth); ok {
		layer.LineWidth = width
	}
	layer.Color = b.layerColor(config + ".style.color.fixed")
}

func (b *panelBuild) networkConfig(layer *spec.GeomapLayer, config string) {
	layer.Arrow = b.arrow(config + ".arrow")
	if width, ok := b.boundedWhole(config+".edgeStyle.size.fixed", 1, maxLayerLineWidth); ok {
		layer.LineWidth = width
	}
	layer.Color = b.layerColor(config + ".style.color.fixed")
	if edge := b.layerColor(config + ".edgeStyle.color.fixed"); edge != "" && edge != layer.Color {
		b.issues.add(config+".edgeStyle.color.fixed", "Edges are drawn in the color of the nodes.")
	}
}

func (b *panelBuild) photosConfig(layer *spec.GeomapLayer, config string) {
	layer.ImageField = b.fieldNameOption(config + ".src")
	switch kind := asString(lookupValue(b.raw, config+".kind")); kind {
	case "circle":
		layer.Shape = kind
		b.consume(config + ".kind")
	case "square":
		b.consume(config + ".kind")
	case "anchored", "folio":
		b.partial(config+".kind", geomapPhotoKindReason)
	}
	if border, ok := b.boundedWhole(config+".border", 0, maxPhotoBorder); ok {
		layer.Border = &border
	}
}

func (b *panelBuild) geojsonConfig(layer *spec.GeomapLayer, config string) {
	b.geojsonSource(layer, config+".src")
	b.geojsonRules(layer, config+".rules")
}

func (b *panelBuild) geojsonSource(layer *spec.GeomapLayer, at string) {
	v, has := b.value(at)
	src := strings.TrimSpace(asString(v))
	if !has || src == "" {
		return
	}
	if source, bundled := geomapGeojsonSources[src]; bundled {
		layer.Source = source
		b.consume(at)
		return
	}
	if literalHTTPS(src) {
		layer.URL = src
		b.consume(at)
		return
	}
	b.partial(at, geomapGeojsonReason)
}

func (b *panelBuild) geojsonRules(layer *spec.GeomapLayer, at string) {
	items, has := asArray(lookupValue(b.raw, at))
	if !has {
		return
	}
	for i, item := range items {
		ruleAt := fmt.Sprintf("%s[%d]", at, i)
		rule, ok := b.geojsonRule(ruleAt, item)
		switch {
		case !ok:
		case len(layer.Rules) >= maxGeomapRules:
			b.partial(ruleAt, geomapRuleReason)
		default:
			layer.Rules = append(layer.Rules, rule)
		}
	}
}

func (b *panelBuild) geojsonRule(at string, item any) (spec.GeomapRule, bool) {
	raw, _ := asObject(item)
	rule := spec.GeomapRule{
		Property:  asString(raw["property"]),
		Operation: asString(raw["operation"]),
	}
	switch value := raw["value"].(type) {
	case string:
		rule.Value = spec.RuleValue{Text: value}
	default:
		number, isNumber := numberValue(value)
		rule.Value = spec.RuleValue{Number: number, IsNumber: isNumber}
		if !isNumber {
			b.partial(at, geomapRuleReason)
			return rule, false
		}
	}
	if rule.Property == "" || !validLength(rule.Property, maxGeomapName) || !slices.Contains(geomapRuleOperations, rule.Operation) {
		b.partial(at, geomapRuleReason)
		return rule, false
	}
	b.consume(at+".property", at+".operation", at+".value")
	rule.Color = b.layerColor(at + ".style.color.fixed")
	rule.Opacity = b.boundedOption(at+".style.opacity", 0, 1)
	return rule, true
}

func (b *panelBuild) dayNightConfig(layer *spec.GeomapLayer, config string) {
	if sun, ok := b.boolOption(config + ".sun"); ok && *sun {
		layer.ShowSun = sun
	}
	layer.NightColor = b.layerColor(config + ".nightColor")
}

func (b *panelBuild) tileLayer(layer *spec.GeomapLayer, typ, config string) bool {
	tile, ok := b.tileSettings(config, typ)
	if !ok || tile.kind == noBasemap {
		return false
	}
	layer.Theme = tile.theme
	layer.Labels = tile.labels
	layer.Server = tile.server
	layer.URL = tile.address
	layer.Attribution = tile.attribution
	layer.Type = tile.kind
	return true
}

func clearDefault[T comparable](field *T, standard T) {
	if *field == standard {
		var zero T
		*field = zero
	}
}

func clearDefaultPointer[T comparable](field **T, standard T) {
	if *field != nil && **field == standard {
		*field = nil
	}
}

func trimLayerDefaults(layer *spec.GeomapLayer) {
	clearDefaultPointer(&layer.Opacity, 1)
	clearDefaultPointer(&layer.Blur, geomapDefaultBlur)
	clearDefaultPointer(&layer.Border, geomapDefaultBorder)
	clearDefaultPointer(&layer.Labels, true)
	clearDefault(&layer.Location, "auto")
	clearDefault(&layer.Radius, geomapDefaultRadius)
	clearDefault(&layer.TextSize, geomapDefaultTextSize)
	clearDefault(&layer.Symbol, "circle")
	clearDefault(&layer.MarkerSize, geomapDefaultMarkerSize)
	clearDefault(&layer.MarkerSizeMax, geomapDefaultMarkerMax)
	clearDefault(&layer.Arrow, "none")
	clearDefault(&layer.Shape, "square")
	clearDefault(&layer.Theme, "auto")
	switch layer.Type {
	case "route":
		clearDefault(&layer.LineWidth, routeDefaultWidth)
	case "network":
		clearDefault(&layer.LineWidth, networkDefaultWidth)
	}
}
