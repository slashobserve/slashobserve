package grafana

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	geomapRegionReason  = "Named map regions are shown fitted to the data."
	geomapViewReason    = "This map view is not supported yet."
	geomapMarkerReason  = "Only marker sizes from 1 to 100 are supported."
	geomapQueryReason   = "This table query type is not supported yet."
	worldmapValueReason = "Only the current value is shown."
	worldmapPlaceReason = "This way of locating data is not supported yet."
	worldmapSource      = "grafana-worldmap-panel"
	worldmapGeohashData = "geohash"
	worldmapTableData   = "table"
	minMarkerSize       = 1
	maxMarkerSize       = 100
	minGeomapZoom       = 1
	maxGeomapZoom       = 18
	maxGeomapLat        = 85
	maxGeomapLon        = 180
	maxFitPadding       = 50
	maxGeomapName       = 100
	maxGeomapLayers     = 10
)

var geomapRegions = []string{"north-america", "south-america", "europe", "africa", "west-asia", "south-asia", "south-east-asia", "east-asia", "australia", "oceania"}

func displayGeomap(b *panelBuild) {
	d := b.disp()
	if b.n.sourceType == worldmapSource {
		b.worldmap(d)
		return
	}
	b.geomapBasemap(d)
	b.geomapView(d)
	b.geomapControls(d)
	b.geomapTooltip(d)
	b.geomapLayers(d)
}

func (b *panelBuild) geomapView(d *spec.Display) {
	id := asString(lookupValue(b.raw, "options.view.id"))
	switch {
	case id == "":
	case id == "fit":
		d.View = "fit"
	case id == "zero":
		d.View = "world"
	case id == "coords":
		d.View = "coords"
		d.Lat = b.boundedOption("options.view.lat", -maxGeomapLat, maxGeomapLat)
		d.Lon = b.boundedOption("options.view.lon", -maxGeomapLon, maxGeomapLon)
		d.Zoom = b.boundedOption("options.view.zoom", minGeomapZoom, maxGeomapZoom)
	case slices.Contains(geomapRegions, id):
		d.View = id
	default:
		b.partial("options.view.id", geomapViewReason)
	}
	b.consume("options.view.id", "options.view.lat", "options.view.lon", "options.view.zoom")
	d.MinZoom = b.boundedOption("options.view.minZoom", 0, maxGeomapZoom)
	d.MaxZoom = b.boundedOption("options.view.maxZoom", 0, maxGeomapZoom)
	if padding, ok := b.boundedWhole("options.view.padding", 0, maxFitPadding); ok {
		d.FitPadding = &padding
	}
	if shared, ok := b.boolOption("options.view.shared"); ok && *shared {
		d.SharedView = shared
	}
	if repeat, ok := b.boolOption("options.view.noRepeat"); ok && *repeat {
		d.NoRepeat = repeat
	}
	b.geomapFitLayers(d)
}

func (b *panelBuild) geomapFitLayers(d *spec.Display) {
	all, hasAll := b.boolOption("options.view.allLayers")
	name := asString(lookupValue(b.raw, "options.view.layer"))
	switch {
	case hasAll && !*all && name != "" && validLength(name, maxGeomapName):
		d.FitLayer = name
		b.consume("options.view.layer")
	case !hasAll || *all:
		b.consume("options.view.layer")
	}
	if last, ok := b.boolOption("options.view.lastOnly"); ok && *last {
		d.FitLastOnly = last
	}
}

func (b *panelBuild) geomapControls(d *spec.Display) {
	d.ShowZoom, _ = b.boolOption("options.controls.showZoom")
	d.ShowAttribution, _ = b.boolOption("options.controls.showAttribution")
	d.ShowScale, _ = b.boolOption("options.controls.showScale")
	d.ShowMeasure, _ = b.boolOption("options.controls.showMeasure")
	d.ShowDebug, _ = b.boolOption("options.controls.showDebug")
	d.MouseWheelZoom = ptr(true)
	if wheel, ok := b.boolOption("options.controls.mouseWheelZoom"); ok {
		d.MouseWheelZoom = wheel
	}
}

func (b *panelBuild) geomapTooltip(d *spec.Display) {
	switch mode := asString(lookupValue(b.raw, "options.tooltip.mode")); mode {
	case "none":
		d.Tooltip = "none"
		b.consume("options.tooltip.mode")
	case "details":
		b.consume("options.tooltip.mode")
	}
}

func (b *panelBuild) boundedWhole(path string, low, high int) (int, bool) {
	f := b.boundedOption(path, float64(low), float64(high))
	if f == nil {
		return 0, false
	}
	return int(math.Round(*f)), true
}

func (b *panelBuild) boundedOption(path string, low, high float64) *float64 {
	v, ok := b.take(path)
	if !ok {
		return nil
	}
	f, isNumber := numberValue(v)
	if !isNumber || f < low || f > high {
		b.partial(path, fmt.Sprintf("Only values from %g to %g are supported.", low, high))
		return nil
	}
	return &f
}

func (b *panelBuild) markerSizeOption(path string) int {
	v, ok := b.take(path)
	if !ok {
		return 0
	}
	f, isNumber := numberValue(v)
	n := int(math.Round(f))
	if !isNumber || n < minMarkerSize || n > maxMarkerSize {
		b.partial(path, geomapMarkerReason)
		return 0
	}
	return n
}

var worldmapGazetteers = map[string]string{
	"countries":         "countries",
	"countries_3letter": "countries",
	"states":            "usa-states",
}

func (b *panelBuild) worldmap(d *spec.Display) {
	source := b.worldmapLocation(d)
	b.worldmapView(d)
	d.MarkerSize = b.markerSizeOption("circleMinSize")
	d.MarkerSizeMax = b.markerSizeOption("circleMaxSize")
	if on, ok := b.boolOption("mouseWheelZoom"); ok && *on {
		d.MouseWheelZoom = on
	}
	b.worldmapThresholds()
	b.legacyDecimals(&b.panel.FieldProps, b.raw["decimals"])
	b.consume("decimals")
	b.worldmapValueName(source)
	b.worldmapLookupLayer(d, source)
}

func (b *panelBuild) worldmapLocation(d *spec.Display) string {
	source := asString(b.raw["locationData"])
	b.consume("locationData")
	_, lookup := worldmapGazetteers[source]
	switch {
	case source == worldmapTableData:
		b.worldmapTable(d)
		return source
	case source == worldmapGeohashData:
		d.Location = "geohash"
	case source == "" || lookup:
	default:
		b.partial("locationData", worldmapPlaceReason)
	}
	b.consume("tableQueryOptions")
	return source
}

func (b *panelBuild) worldmapLookupLayer(d *spec.Display, source string) {
	gazetteer, ok := worldmapGazetteers[source]
	if !ok {
		return
	}
	fill := grafanaMarkerFillOpacity
	d.Layers = []spec.GeomapLayer{{
		Type: "markers", Location: "lookup", Gazetteer: gazetteer,
		MarkerSize: d.MarkerSize, MarkerSizeMax: d.MarkerSizeMax,
		FillOpacity: &fill, ShowLegend: ptr(true),
	}}
	d.MarkerSize, d.MarkerSizeMax = 0, 0
}

func (b *panelBuild) worldmapValueName(source string) {
	name := asString(b.raw["valueName"])
	b.consume("valueName")
	_, lookup := worldmapGazetteers[source]
	tableRows := b.hasTableQuery()
	switch {
	case name == "":
	case lookup && !tableRows:
		b.worldmapReduce(name)
	case source == worldmapTableData || lookup:
		b.recordImplicit("valueName", outcomeNoEffect)
	case name != "current":
		b.partial("valueName", worldmapValueReason)
	}
}

func (b *panelBuild) worldmapReduce(name string) {
	reducer, ok := singlestatReducers[name]
	switch {
	case !ok:
		b.partial("valueName", fmt.Sprintf("The value %s is not supported yet.", name))
	case len(b.panel.Transform) >= maxTransforms:
		b.partial("valueName", "A panel holds at most 20 transformations.")
	default:
		b.panel.Transform = append(b.panel.Transform, spec.Transform{Type: "reduce", Reducers: []string{reducer}})
	}
}

func (b *panelBuild) hasTableQuery() bool {
	return slices.ContainsFunc(b.built, func(q *builtQuery) bool { return q.q.Format == "table" })
}

func (b *panelBuild) worldmapTable(d *spec.Display) {
	const options = "tableQueryOptions."
	switch kind := asString(lookupValue(b.raw, options+"queryType")); kind {
	case "coordinates":
		d.Location = "coords"
		d.LatField = b.fieldNameOption(options + "latitudeField")
		d.LonField = b.fieldNameOption(options + "longitudeField")
	case "geohash", "":
		d.Location = "geohash"
		d.GeohashField = b.fieldNameOption(options + "geohashField")
	default:
		b.partial(options+"queryType", geomapQueryReason)
	}
	d.SizeField = b.fieldNameOption(options + "metricField")
	d.ColorField = d.SizeField
	b.consume(options+"queryType", options+"latitudeField", options+"longitudeField", options+"geohashField")
}

func (b *panelBuild) worldmapView(d *spec.Display) {
	center := asString(b.raw["mapCenter"])
	b.consume("mapCenter")
	switch center {
	case "custom":
		d.View = "coords"
		d.Lat = b.boundedOption("mapCenterLatitude", -maxGeomapLat, maxGeomapLat)
		d.Lon = b.boundedOption("mapCenterLongitude", -maxGeomapLon, maxGeomapLon)
		d.Zoom = b.boundedOption("initialZoom", minGeomapZoom, maxGeomapZoom)
	case "(0°, 0°)":
		d.View = "world"
	case "":
	default:
		b.partial("mapCenter", geomapRegionReason)
	}
	b.consume("mapCenterLatitude", "mapCenterLongitude", "initialZoom")
}

func (b *panelBuild) worldmapThresholds() {
	raw := strings.TrimSpace(asString(b.raw["thresholds"]))
	colors := stringList(b.raw["colors"])
	b.consume("thresholds", "colors")
	if raw == "" {
		return
	}
	parts := strings.Split(raw, ",")
	if len(colors) != len(parts)+1 {
		b.partial("colors", "The thresholds and colors do not match, so neither is kept.")
		return
	}
	steps := []thresholdStep{{index: 0, color: colors[0]}}
	for i, part := range parts {
		value, ok := numberValue(part)
		if !ok {
			b.partial("thresholds", "The thresholds are not numbers, so they are not kept.")
			return
		}
		steps = append(steps, thresholdStep{index: i + 1, value: &value, color: colors[i+1]})
	}
	b.panel.Thresholds = b.finishThresholds(steps, func(i int) string { return fmt.Sprintf("colors[%d]", i) })
}
