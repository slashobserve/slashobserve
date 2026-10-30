package grafana

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	genericPartialReason = "Some Grafana options of this panel are not supported yet."
	legendTableReason    = "Legend tables are not supported; the legend is a plain list."
)

var grafanaDefaults = map[string]any{
	"fieldConfig.defaults.custom.lineWidth":              1,
	"fieldConfig.defaults.custom.fillOpacity":            0,
	"fieldConfig.defaults.custom.gradientMode":           "none",
	"fieldConfig.defaults.custom.showPoints":             "auto",
	"fieldConfig.defaults.custom.pointSize":              5,
	"fieldConfig.defaults.custom.spanNulls":              false,
	"fieldConfig.defaults.custom.insertNulls":            false,
	"fieldConfig.defaults.custom.lineInterpolation":      "linear",
	"fieldConfig.defaults.custom.barAlignment":           0,
	"fieldConfig.defaults.custom.axisPlacement":          "auto",
	"fieldConfig.defaults.custom.axisLabel":              "",
	"fieldConfig.defaults.custom.axisCenteredZero":       false,
	"fieldConfig.defaults.custom.axisColorMode":          "text",
	"fieldConfig.defaults.custom.axisBorderShow":         false,
	"fieldConfig.defaults.custom.scaleDistribution.type": "linear",
	"fieldConfig.defaults.custom.hideFrom.legend":        false,
	"fieldConfig.defaults.custom.hideFrom.tooltip":       false,
	"fieldConfig.defaults.custom.hideFrom.viz":           false,
	"fieldConfig.defaults.custom.stacking.mode":          "none",
	"fieldConfig.defaults.custom.stacking.group":         "A",
	"fieldConfig.defaults.custom.thresholdsStyle.mode":   "off",
	"fieldConfig.defaults.custom.drawStyle":              "line",
	"fieldConfig.defaults.custom.lineStyle.fill":         "solid",
	"fieldConfig.defaults.custom.align":                  "auto",
	"fieldConfig.defaults.custom.cellOptions.type":       "auto",
	"fieldConfig.defaults.custom.inspect":                false,
	"fieldConfig.defaults.custom.filterable":             false,
	"fieldConfig.defaults.fieldMinMax":                   false,
	"options.tooltip.mode":                               "single",
	"options.tooltip.sort":                               "none",
	"options.tooltip.hideZeros":                          false,
	"options.legend.showLegend":                          true,
	"options.legend.displayMode":                         "list",
	"options.legend.placement":                           "bottom",
	"options.legend.calcs":                               []any{},
	"options.reduceOptions.values":                       false,
	"options.reduceOptions.fields":                       "",
	"options.reduceOptions.calcs":                        []any{"lastNotNull"},
	"options.text":                                       map[string]any{},
	"options.justifyMode":                                "auto",
	"options.wideLayout":                                 true,
	"options.showPercentChange":                          false,
	"options.percentChangeColorMode":                     "standard",
	"options.sizing":                                     "auto",
	"options.maxVizHeight":                               300,
	"options.namePlacement":                              "auto",
	"options.valueMode":                                  "color",
	"options.showUnfilled":                               true,
	"options.cellHeight":                                 "sm",
	"options.showHeader":                                 true,
	"options.footer.show":                                false,
	"options.footer.reducer":                             []any{"sum"},
	"options.footer.countRows":                           false,
	"options.footer.fields":                              "",
	"options.colorMode":                                  "value",
	"options.graphMode":                                  "area",
	"options.textMode":                                   "auto",
	"options.orientation":                                "auto",
	"options.showThresholdLabels":                        false,
	"options.showThresholdMarkers":                       true,
	"options.showTime":                                   false,
	"options.wrapLogMessage":                             false,
	"options.enableInfiniteScrolling":                    false,
	"options.sortOrder":                                  "Descending",
	"options.mergeValues":                                true,
	"options.showValue":                                  "auto",
	"options.rowHeight":                                  0.9,
	"options.alignValue":                                 "left",
	"options.pieType":                                    "pie",
	"options.displayLabels":                              []any{},
	"options.code.language":                              "plaintext",
	"options.code.showLineNumbers":                       false,
	"options.code.showMiniMap":                           false,
	"fieldConfig.defaults.custom.barWidthFactor":         0.6,
	"fieldConfig.defaults.custom.showValues":             false,
	"renderer":                                           "flot",
	"fillGradient":                                       0,
	"linewidth":                                          1,
	"hiddenSeries":                                       false,
	"dashLength":                                         10,
	"spaceLength":                                        10,
	"points":                                             false,
	"stack":                                              false,
	"percentage":                                         false,
	"nullPointMode":                                      "null",
	"steppedLine":                                        false,
	"tooltip.value_type":                                 "individual",
	"tooltip.sort":                                       0,
	"xaxis.show":                                         true,
	"xaxis.mode":                                         "time",
	"yaxis.align":                                        false,
	"options.alertThreshold":                             true,
}

var typeDefaults = map[spec.PanelType]map[string]any{
	spec.Text: {
		"options.mode": "markdown",
	},
	spec.StatusHistory: {
		"options.colWidth": 0.9,
	},
	spec.XYChart: {
		"options.mapping":                        "auto",
		"fieldConfig.defaults.custom.pointShape": "circle",
	},
	spec.Candlestick: {
		"options.colors.flat": "gray",
	},
	spec.StateTimeline: {
		"display":              "timeline",
		"rowHeight":            50,
		"metricNameColor":      "#000000",
		"valueTextColor":       "#000000",
		"timeTextColor":        "#d8d9da",
		"crosshairColor":       "#8F070C",
		"backgroundColor":      "rgba(128,128,128,0.1)",
		"lineColor":            "rgba(0,0,0,0.1)",
		"textSize":             24,
		"textSizeTime":         12,
		"extendLastValue":      true,
		"writeLastValue":       true,
		"writeAllValues":       false,
		"writeMetricNames":     false,
		"showTimeAxis":         true,
		"showLegend":           true,
		"showLegendNames":      true,
		"showLegendValues":     true,
		"showLegendPercent":    true,
		"highlightOnMouseover": true,
		"expandFromQueryS":     0,
		"legendSortBy":         "-ms",
		"timePrecision.name":   "Minutes",
		"timePrecision.value":  "minutes",
		"useTimePrecision":     false,
		"use12HourClock":       false,
	},
	spec.Geomap: {
		"options.tooltip.mode":                                  "details",
		"options.controls.mouseWheelZoom":                       true,
		"options.controls.showAttribution":                      true,
		"options.controls.showZoom":                             true,
		"options.controls.showDebug":                            false,
		"options.controls.showMeasure":                          false,
		"options.controls.showScale":                            false,
		"options.view.allLayers":                                true,
		"options.layers[].tooltip":                              true,
		"options.layers[].config.showLegend":                    true,
		"options.layers[].config.style.opacity":                 0.4,
		"options.layers[].config.style.symbol.fixed":            "img/icons/marker/circle.svg",
		"options.layers[].config.style.symbol.mode":             "fixed",
		"options.layers[].config.style.rotation.fixed":          0,
		"options.layers[].config.style.rotation.min":            -360,
		"options.layers[].config.style.rotation.max":            360,
		"options.layers[].config.style.rotation.mode":           "mod",
		"options.layers[].config.style.text.fixed":              "",
		"options.layers[].config.style.text.mode":               "fixed",
		"options.layers[].config.style.textConfig.fontSize":     12,
		"options.layers[].config.style.textConfig.offsetX":      0,
		"options.layers[].config.style.textConfig.offsetY":      0,
		"options.layers[].config.style.textConfig.textAlign":    "center",
		"options.layers[].config.style.textConfig.textBaseline": "middle",
		"esMetric":                     "Count",
		"hideEmpty":                    false,
		"hideZero":                     false,
		"showLegend":                   true,
		"stickyLabels":                 false,
		"mouseWheelZoom":               false,
		"unitSingle":                   "",
		"unitPlural":                   "",
		"tableQueryOptions.labelField": "",
	},
	spec.Canvas: {
		"options.panZoom":     false,
		"options.infinitePan": false,
	},
	spec.AlertList: {
		"options.dashboardAlerts":    false,
		"options.groupMode":          "default",
		"options.showInstances":      false,
		"options.showInactiveAlerts": false,
		"options.statColorMode":      "none",
	},
}

var partialReasons = map[string]string{
	"fieldConfig.defaults.custom.scaleDistribution": "Logarithmic axes are not supported yet.",
	"fieldConfig.defaults.custom.transform":         "Value transforms are drawn on time series and trend panels only.",
	"fieldConfig.defaults.custom.fillBelowTo":       "Filling down to another series is drawn on time series and trend panels only.",
	"fieldConfig.overrides":                         "Some overrides use matchers or properties that are not supported yet.",
	"transformations":                               "Some transformations are not supported yet.",
	"options.legend.displayMode":                    legendTableReason,
	"options.legend.asTable":                        legendTableReason,
	"rowHeight":                                     "A row height in pixels is not supported; rows share the height of the panel.",
	"options.controls":                              "Map controls are not configurable.",
	"options.layers":                                "Some layer options are not supported yet.",
	"options.panZoom":                               "Panning and zooming the canvas is not supported.",
	"options.infinitePan":                           "Panning and zooming the canvas is not supported.",
	"options.enableInfiniteScrolling":               "Infinite scrolling of logs is not supported yet.",
	"options.dashboardAlerts":                       "Listing only the alerts of this dashboard is not supported.",
	"options.groupMode":                             "Grouping alerts is not supported.",
	"options.groupBy":                               "Grouping alerts is not supported.",
	"options.folder":                                "Folders are not supported.",
	"options.showInstances":                         "Alert rules are not listed apart from their instances.",
	"options.showInactiveAlerts":                    "Inactive alerts are not listed.",
	"options.statColorMode":                         "The colors of the stat view are not supported yet.",
	"options.statValueMappings":                     "The colors of the stat view are not supported yet.",
}

type cosmeticOption struct {
	values []any
	why    string
}

var defaultDash = []any{10, 10}

var cosmeticOptions = map[string]cosmeticOption{
	"fieldConfig.defaults.fieldMinMax": {
		values: []any{true, false},
		why:    "only gauges and bar gauges share one range across fields, and every other panel ranges each field by its own values or by none",
	},
	"fieldConfig.defaults.custom.lineStyle.dash": {
		values: []any{defaultDash},
		why:    "10, 10 is the pattern Grafana draws for a dashed line, and the renderer draws every dashed line with one pattern",
	},
	"fieldConfig.defaults.custom.axisPlacement": {
		values: []any{"left"},
		why:    "the renderer draws every axis on the left unless the Spec says right or hidden",
	},
	"fieldConfig.defaults.custom.gradientMode": {
		why: "the renderer draws every fill flat by the no-gradients rule (decision 8)",
	},
	"fillGradient": {
		why: "the renderer draws every fill flat by the no-gradients rule (decision 8)",
	},
	"fieldConfig.defaults.custom.cellOptions.mode": {
		values: []any{"gradient", "basic"},
		why:    "table cells are drawn flat by the no-gradients rule (decision 8)",
	},
	"fieldConfig.defaults.custom.cellOptions.gradientMode": {
		why: "table cells are drawn flat by the no-gradients rule (decision 8)",
	},
	"options.displayMode": {
		values: []any{"gradient", "basic"},
		why:    "bar gauges are drawn flat by the no-gradients rule (decision 8)",
	},
	"options.layers[].name": {
		why: "the map shows no layer names",
	},
	"options.color.scale": {
		values: []any{"exponential"},
		why:    "exponential is Grafana's own default scale and an absent scale is imported as exponential, so writing it changes nothing",
	},
	"options.inlineEditing": {
		why: "the canvas has no drag editor",
	},
	"options.showAdvancedTypes": {
		why: "the canvas has no drag editor, so its element picker is not shown",
	},
	"options.showHeadings": {
		why: "the dashboard list shows plain links without headings",
	},
	"options.showFolderNames": {
		why: "the dashboard list shows plain links without folder names",
	},
	"seriesOverrides[].zindex": {
		why: "series are drawn in query order",
	},
	"fieldConfig.overrides[].__systemRef": {
		why: "bookkeeping marker Grafana writes when a legend click hides a series; the hideFrom property beside it is the converted part",
	},
}

func isCosmetic(path string, value any) bool {
	option, ok := cosmeticOptions[normalizePath(path)]
	if !ok {
		return false
	}
	return len(option.values) == 0 || slices.ContainsFunc(option.values, func(v any) bool { return sameJSON(value, v) })
}

var indexPattern = regexp.MustCompile(`\[[0-9]+\]`)

func normalizePath(path string) string {
	open := strings.IndexByte(path, '[')
	if open < 0 {
		return path
	}
	var out strings.Builder
	out.Grow(len(path))
	for open >= 0 {
		out.WriteString(path[:open])
		path = path[open:]
		digits := 1
		for digits < len(path) && path[digits] >= '0' && path[digits] <= '9' {
			digits++
		}
		if digits > 1 && digits < len(path) && path[digits] == ']' {
			out.WriteString("[]")
			path = path[digits+1:]
		} else {
			out.WriteByte('[')
			path = path[1:]
		}
		open = strings.IndexByte(path, '[')
	}
	out.WriteString(path)
	return out.String()
}

func partialReason(path string) string {
	normalized := normalizePath(path)
	for normalized != "" {
		if reason, ok := partialReasons[normalized]; ok {
			return reason
		}
		cut := strings.LastIndexAny(normalized, ".[")
		if cut < 0 {
			break
		}
		normalized = normalized[:cut]
	}
	return genericPartialReason
}

func isGrafanaDefault(typ spec.PanelType, path string, value any) bool {
	normalized := normalizePath(path)
	if def, ok := typeDefaults[typ][normalized]; ok {
		return sameJSON(value, def)
	}
	def, ok := grafanaDefaults[normalized]
	return ok && sameJSON(value, def)
}

func sameJSON(a, b any) bool {
	return reflect.DeepEqual(canonicalJSON(a), canonicalJSON(b))
}

func canonicalJSON(v any) any {
	switch x := v.(type) {
	case json.Number:
		f, _ := x.Float64()
		return f
	case int:
		return float64(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = canonicalJSON(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = canonicalJSON(item)
		}
		return out
	}
	return v
}
