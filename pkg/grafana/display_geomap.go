package grafana

import (
	"fmt"
	"math"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	geomapRegionReason  = "Named map regions are shown fitted to the data."
	geomapLookupReason  = "Looking up locations by name is not supported yet."
	geomapBasemapReason = "Map tiles are not loaded; the map shows country outlines."
	geomapLayerReason   = "Only the first markers layer is drawn; other layers are left out."
	geomapMarkerReason  = "Only marker sizes from 1 to 100 are supported."
	geomapQueryReason   = "This table query type is not supported yet."
	worldmapValueReason = "Only the current value is shown."
	worldmapSource      = "grafana-worldmap-panel"
	worldmapGeohashData = "geohash"
	worldmapTableData   = "table"
	minMarkerSize       = 1
	maxMarkerSize       = 100
	minGeomapZoom       = 1
	maxGeomapZoom       = 18
	maxGeomapLat        = 85
	maxGeomapLon        = 180
)

func displayGeomap(b *panelBuild) {
	d := b.disp()
	if b.n.sourceType == worldmapSource {
		b.worldmap(d)
		return
	}
	b.geomapView(d)
	b.geomapLayers(d)
	if present(lookupValue(b.raw, "options.basemap")) {
		b.partial("options.basemap", geomapBasemapReason)
	}
}

func (b *panelBuild) geomapView(d *spec.Display) {
	switch id := asString(lookupValue(b.raw, "options.view.id")); id {
	case "":
		return
	case "fit":
		d.View = "fit"
	case "zero":
		d.View = "world"
	case "coords":
		d.View = "coords"
		d.Lat = b.boundedOption("options.view.lat", -maxGeomapLat, maxGeomapLat)
		d.Lon = b.boundedOption("options.view.lon", -maxGeomapLon, maxGeomapLon)
		d.Zoom = b.boundedOption("options.view.zoom", minGeomapZoom, maxGeomapZoom)
	default:
		b.partial("options.view.id", geomapRegionReason)
	}
	b.consume("options.view.id", "options.view.lat", "options.view.lon", "options.view.zoom")
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

func (b *panelBuild) geomapLayers(d *spec.Display) {
	layers, _ := asArray(lookupValue(b.raw, "options.layers"))
	drawn := false
	for i, item := range layers {
		layer, _ := asObject(item)
		path := fmt.Sprintf("options.layers[%d]", i)
		if drawn || asString(layer["type"]) != "markers" {
			b.partial(path, geomapLayerReason)
			continue
		}
		drawn = true
		b.consume(path + ".type")
		b.geomapLocation(d, path+".location")
		b.geomapStyle(d, path+".config.style")
	}
}

func (b *panelBuild) geomapLocation(d *spec.Display, path string) {
	switch mode := asString(lookupValue(b.raw, path+".mode")); mode {
	case "auto":
		d.Location = mode
	case "coords":
		d.Location = mode
		d.LatField = b.fieldNameOption(path + ".latitude")
		d.LonField = b.fieldNameOption(path + ".longitude")
	case "geohash":
		d.Location = mode
		d.GeohashField = b.fieldNameOption(path + ".geohash")
	case "lookup":
		b.partial(path, geomapLookupReason)
	}
	b.consume(path+".mode", path+".latitude", path+".longitude", path+".geohash")
	if asString(lookupValue(b.raw, path+".mode")) != "lookup" {
		b.consume(path+".lookup", path+".gazetteer")
	}
}

func (b *panelBuild) geomapStyle(d *spec.Display, style string) {
	d.SizeField = b.fieldNameOption(style + ".size.field")
	if d.SizeField != "" {
		d.MarkerSize = b.markerSizeOption(style + ".size.min")
		d.MarkerSizeMax = b.markerSizeOption(style + ".size.max")
		b.consume(style + ".size.fixed")
	} else {
		d.MarkerSize = b.markerSizeOption(style + ".size.fixed")
		b.consume(style+".size.min", style+".size.max")
	}
	d.ColorField = b.fieldNameOption(style + ".color.field")
	if v, ok := b.take(style + ".color.fixed"); ok && d.ColorField == "" {
		b.markerColor(style+".color.fixed", asString(v))
	}
}

func (b *panelBuild) markerColor(path, name string) {
	color, ok := seriesColor(name)
	switch {
	case !ok:
		b.partial(path, fmt.Sprintf("The marker color %q is not supported yet.", name))
	case b.panel.Color == "":
		b.panel.Color = color
	}
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

func (b *panelBuild) worldmap(d *spec.Display) {
	b.worldmapLocation(d)
	b.worldmapView(d)
	d.MarkerSize = b.markerSizeOption("circleMinSize")
	d.MarkerSizeMax = b.markerSizeOption("circleMaxSize")
	b.worldmapThresholds()
	b.legacyDecimals(&b.panel.FieldProps, b.raw["decimals"])
	b.consume("decimals")
	if name := asString(b.raw["valueName"]); name != "" && name != "current" {
		b.partial("valueName", worldmapValueReason)
	}
	b.consume("valueName")
}

func (b *panelBuild) worldmapLocation(d *spec.Display) {
	source := asString(b.raw["locationData"])
	b.consume("locationData")
	switch source {
	case worldmapTableData:
		b.worldmapTable(d)
		return
	case worldmapGeohashData:
		d.Location = "geohash"
	case "":
	default:
		b.partial("locationData", geomapLookupReason)
	}
	b.consume("tableQueryOptions")
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
