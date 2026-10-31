package grafana

import (
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type decision struct {
	id     string
	reason string
}

type excludedOption struct {
	scope    optionScope
	path     string
	key      string
	values   []any
	when     func(value any) bool
	decision decision
}

var (
	decisionAlertRules               = decision{"alert-rules", "Alert rules are not part of SlashObserve."}
	decisionAnnotations              = decision{"annotations", "Annotations are not part of SlashObserve."}
	decisionLive                     = decision{"live", "Grafana Live streaming is not part of SlashObserve."}
	decisionLogContext               = decision{"log-context", "Log context is not part of SlashObserve."}
	decisionScopes                   = decision{"scopes", "Grafana scopes are not part of SlashObserve."}
	decisionGrafanaImages            = decision{"grafana-images", "Images served by a Grafana server are not part of SlashObserve."}
	decisionInfinity                 = decision{"infinity", "The Infinity datasource is not part of SlashObserve."}
	decisionExploreLinks             = decision{"explore-links", "Explore links are not part of SlashObserve."}
	decisionDataEditing              = decision{"data-editing", "Editing data from a panel is not part of SlashObserve."}
	decisionFieldCSS                 = decision{"field-css", "CSS taken from a field is not part of SlashObserve."}
	decisionDashboardListHistory     = decision{"dashboard-list-history", "Recently viewed dashboards and Grafana folder filters in dashboard lists are not part of SlashObserve."}
	decisionHeatmapSelection         = decision{"heatmap-selection", "The heatmap selection modes xy and y are not part of SlashObserve."}
	decisionValueAxisPlacement       = decision{"value-axis-placement", "Value axes at the top or bottom and ordinal value scales are not part of SlashObserve."}
	decisionTimeCompare              = decision{"time-compare", "The time comparison and horizontal orientation of time series are not part of SlashObserve."}
	decisionBarGaugeLegend           = decision{"bar-gauge-legend", "Bar gauge legends are not part of SlashObserve."}
	decisionGaugeLayout              = decision{"gauge-layout", "The gauge layout options are not part of SlashObserve."}
	decisionLogScrolling             = decision{"log-scrolling", "Infinite log scrolling and the log field selector are not part of SlashObserve."}
	decisionTableMapCells            = decision{"table-map-cells", "Map cells in tables are not part of SlashObserve."}
	decisionOverrideMatchers         = decision{"override-matchers", "This override matcher is not part of SlashObserve."}
	decisionServerAnimation          = decision{"server-animation", "Server element animation is not part of SlashObserve."}
	decisionCanvasConnections        = decision{"canvas-connections", "Data-driven canvas connections are not part of SlashObserve."}
	decisionCanvasZoomToContent      = decision{"canvas-zoom-to-content", "Canvas zoom to content is not part of SlashObserve."}
	decisionCanvasOneClickTooltip    = decision{"canvas-one-click-tooltip", "The one-click canvas tooltip is not part of SlashObserve."}
	decisionCanvasImageMapping       = decision{"canvas-image-mapping", "The canvas background image mapping mode is not part of SlashObserve."}
	decisionAltinityLogLabels        = decision{"altinity-log-labels", "Hidden log labels in Altinity logs settings are not part of SlashObserve."}
	decisionGroupByClickHouse        = decision{"group-by-clickhouse", "A group-by variable on ClickHouse is not part of SlashObserve."}
	decisionAltinityAdHocValues      = decision{"altinity-ad-hoc-values", "The Altinity ad hoc values query is not part of SlashObserve."}
	decisionSQLExpressions           = decision{"sql-expressions", "SQL expressions are not part of SlashObserve."}
	decisionSnapshots                = decision{"snapshots", "Snapshots are not part of SlashObserve."}
	decisionReadOnlyDashboards       = decision{"read-only-dashboards", "Read-only dashboards are not part of SlashObserve."}
	decisionLinkPlacement            = decision{"link-placement", "Link icons and link placement in the controls menu are not part of SlashObserve."}
	decisionControlsMenu             = decision{"controls-menu", "The variable controls menu is not part of SlashObserve."}
	decisionMultiDatasourceVariables = decision{"multi-datasource-variables", "Several datasources, All and custom values in a datasource variable are not part of SlashObserve."}
	decisionMappingIcons             = decision{"mapping-icons", "Value mapping icons are not part of SlashObserve."}
	decisionMapLibre                 = decision{"maplibre", "MapLibre basemaps are not part of SlashObserve."}
	decisionDynamicGeoJSON           = decision{"dynamic-geojson", "Dynamic GeoJSON layers are not part of SlashObserve."}
	decisionLastPointTracker         = decision{"last-point-tracker", "The last-point tracker layer is not part of SlashObserve."}
	decisionPhotoFrames              = decision{"photo-frames", "Photo frames are not part of SlashObserve."}
	decisionTransformationOptions    = decision{"transformation-options", "This transformation option is not part of SlashObserve."}
	decisionDebugPanel               = decision{"debug-panel", "Grafana's debug panel modes are not part of SlashObserve; the panel shows its queries as a table, or as text when it has none."}
)

var decisions = []decision{
	decisionAlertRules, decisionAnnotations, decisionLive, decisionLogContext, decisionScopes,
	decisionGrafanaImages, decisionInfinity, decisionExploreLinks, decisionDataEditing, decisionFieldCSS,
	decisionDashboardListHistory, decisionHeatmapSelection, decisionValueAxisPlacement, decisionTimeCompare,
	decisionBarGaugeLegend, decisionGaugeLayout, decisionLogScrolling, decisionTableMapCells,
	decisionOverrideMatchers, decisionServerAnimation, decisionCanvasConnections, decisionCanvasZoomToContent,
	decisionCanvasOneClickTooltip, decisionCanvasImageMapping, decisionAltinityLogLabels,
	decisionGroupByClickHouse, decisionAltinityAdHocValues, decisionSQLExpressions, decisionSnapshots,
	decisionReadOnlyDashboards, decisionLinkPlacement, decisionControlsMenu, decisionMultiDatasourceVariables,
	decisionMappingIcons, decisionMapLibre, decisionDynamicGeoJSON, decisionLastPointTracker,
	decisionPhotoFrames, decisionTransformationOptions, decisionDebugPanel,
}

var valueAxisPanels = []spec.PanelType{spec.Timeseries, spec.Trend, spec.Candlestick, spec.Histogram, spec.BarChart, spec.XYChart}

func onPanels(row excludedOption, types ...spec.PanelType) []excludedOption {
	out := make([]excludedOption, len(types))
	for i, t := range types {
		row.scope = panelScope(t)
		out[i] = row
	}
	return out
}

func anyPanel(row excludedOption) excludedOption {
	row.scope = optionScope("panel:*")
	return row
}

func onTransform(id string, row excludedOption) excludedOption {
	row.scope = transformScope(id)
	return row
}

func onVariable(typ string, row excludedOption) excludedOption {
	row.scope = variableScope(typ)
	return row
}

func onDashboard(row excludedOption) excludedOption {
	row.scope = scopeDashboard
	return row
}

func onQuery(kind queryKind, row excludedOption) excludedOption {
	row.scope = queryScope(kind)
	return row
}

func anyTransform(row excludedOption) excludedOption {
	row.scope = optionScope("transform:*")
	return row
}

func anyVariable(row excludedOption) excludedOption {
	row.scope = optionScope("variable:*")
	return row
}

var excludedOptions = slices.Concat(
	onPanels(excludedOption{path: "fieldConfig.defaults.custom.axisPlacement", values: []any{"top", "bottom"}, decision: decisionValueAxisPlacement}, valueAxisPanels...),
	onPanels(excludedOption{path: "fieldConfig.defaults.custom.scaleDistribution.type", values: []any{"ordinal"}, decision: decisionValueAxisPlacement}, valueAxisPanels...),
	[]excludedOption{
		{scope: panelScope(spec.AlertList), path: "options.groupMode", values: []any{"custom"}, decision: decisionAlertRules},
		{scope: panelScope(spec.AlertList), path: "options.groupBy", decision: decisionAlertRules},
		{scope: panelScope(spec.AlertList), path: "options.showInstances", values: []any{true}, decision: decisionAlertRules},
		{scope: panelScope(spec.AlertList), path: "options.showInactiveAlerts", values: []any{true}, decision: decisionAlertRules},
		{scope: panelScope(spec.AlertList), path: "options.stateFilter.normal", values: []any{true}, decision: decisionAlertRules},
		{scope: panelScope(spec.AlertList), path: "options.dashboardAlerts", values: []any{true}, decision: decisionAlertRules},
		{scope: panelScope(spec.AlertList), path: "options.folder", decision: decisionAlertRules},

		{scope: optionScope("panel:graph"), path: "alert", decision: decisionAlertRules},

		{scope: panelScope(spec.BarGauge), path: "options.legend", decision: decisionBarGaugeLegend},
		{scope: panelScope(spec.Gauge), path: "options.orientation", values: []any{"vertical", "horizontal"}, decision: decisionGaugeLayout},
		{scope: panelScope(spec.Heatmap), path: "options.selectionMode", values: []any{"xy", "y"}, decision: decisionHeatmapSelection},
		{scope: panelScope(spec.DashList), path: "options.showRecentlyViewed", values: []any{true}, decision: decisionDashboardListHistory},
		{scope: panelScope(spec.DashList), path: "options.folderUID", decision: decisionDashboardListHistory},
		{scope: panelScope(spec.DashList), path: "options.folderId", decision: decisionDashboardListHistory},
		{scope: panelScope(spec.Timeseries), path: "options.timeCompare", decision: decisionTimeCompare},
		{scope: panelScope(spec.Timeseries), path: "options.orientation", values: []any{"vertical", "horizontal"}, decision: decisionTimeCompare},
		{scope: panelScope(spec.Logs), path: "options.enableInfiniteScrolling", values: []any{true}, decision: decisionLogScrolling},
		{scope: panelScope(spec.Logs), path: "options.showFieldSelector", values: []any{true}, decision: decisionLogScrolling},
		{scope: panelScope(spec.Logs), path: "options.showLogContextToggle", values: []any{true}, decision: decisionLogContext},
		{scope: panelScope(spec.Table), path: "fieldConfig.defaults.custom.styleField", decision: decisionFieldCSS},
		{scope: panelScope(spec.Table), path: "fieldConfig.defaults.custom.cellOptions", key: "type", values: []any{"geo"}, decision: decisionTableMapCells},

		{scope: panelScope(debugSource), path: "type", decision: decisionDebugPanel},
		{scope: panelScope(debugSource), path: "options", decision: decisionDebugPanel},

		{scope: panelScope(spec.Canvas), path: "options.zoomToContent", values: []any{true}, decision: decisionCanvasZoomToContent},
		{scope: panelScope(spec.Canvas), path: "options.tooltip.disableForOneClick", values: []any{true}, decision: decisionCanvasOneClickTooltip},
		{scope: panelScope(spec.Canvas), path: "options.root.elements[].background.image.fixed", when: servedByGrafana, decision: decisionGrafanaImages},
		{scope: panelScope(spec.Canvas), path: "options.root.elements[].background.image.mode", values: []any{"mapping"}, decision: decisionCanvasImageMapping},
		{scope: panelScope(spec.Canvas), path: "options.root.elements[].actions[]", key: "type", values: []any{"infinity"}, decision: decisionInfinity},
		{scope: panelScope(spec.Canvas), path: "options.root.elements[].config.blinkRate", decision: decisionServerAnimation},
		{scope: panelScope(spec.Canvas), path: "options.root.elements[].config.bulbColor", decision: decisionServerAnimation},
		{scope: panelScope(spec.Canvas), path: "options.root.elements[].connections[].size.mode", when: followsRule, decision: decisionCanvasConnections},
		{scope: panelScope(spec.Canvas), path: "options.root.elements[].connections[].direction", key: "mode", values: []any{"field"}, decision: decisionCanvasConnections},

		{scope: panelScope(spec.Geomap), path: "options.basemap", key: "type", values: []any{"maplibre"}, decision: decisionMapLibre},
		{scope: panelScope(spec.Geomap), path: "options.layers[]", key: "type", values: []any{"last-point-tracker"}, decision: decisionLastPointTracker},
		{scope: panelScope(spec.Geomap), path: "options.layers[]", key: "type", values: []any{"dynamic-geojson"}, decision: decisionDynamicGeoJSON},
		{scope: panelScope(spec.Geomap), path: "options.layers[].config.kind", values: []any{"anchored", "folio"}, decision: decisionPhotoFrames},

		anyPanel(excludedOption{path: "fieldConfig.defaults.writeable", values: []any{true}, decision: decisionDataEditing}),
		anyPanel(excludedOption{path: "fieldConfig.defaults.mappings[]", when: hasIcon, decision: decisionMappingIcons}),
		anyPanel(excludedOption{path: "fieldConfig.defaults.links[]", when: isExploreLink, decision: decisionExploreLinks}),
		anyPanel(excludedOption{path: "fieldConfig.defaults.actions[]", key: "type", values: []any{"infinity"}, decision: decisionInfinity}),
		anyPanel(excludedOption{path: "fieldConfig.overrides[]", when: hasExcludedMatcher, decision: decisionOverrideMatchers}),

		onQuery(queryAltinity, excludedOption{path: "adHocValuesQuery", decision: decisionAltinityAdHocValues}),
		onQuery(queryAltinity, excludedOption{path: "contextWindowSize", decision: decisionLogContext}),
		onQuery(queryAltinity, excludedOption{path: "logsFieldConfig", key: "labels.mode", values: []any{"hide"}, decision: decisionAltinityLogLabels}),
		onQuery(queryAltinity, excludedOption{path: "streaming", decision: decisionLive}),
		onQuery(queryAltinity, excludedOption{path: "streamingInterval", decision: decisionLive}),
		onQuery(queryAltinity, excludedOption{path: "streamingMode", decision: decisionLive}),
		onQuery(queryAltinity, excludedOption{path: "streamingLookback", decision: decisionLive}),
		onQuery(queryPrometheus, excludedOption{path: "scopes", decision: decisionScopes}),

		anyTransform(excludedOption{path: "topic", values: []any{"annotations"}, decision: decisionAnnotations}),
		onTransform("convertFrameType", excludedOption{key: "options.targetType", values: []any{"annotation", "timeRegion", "exemplar"}, decision: decisionTransformationOptions}),
		onTransform("calculateField", excludedOption{path: "options.unary.operator", values: []any{"percent"}, decision: decisionTransformationOptions}),
		onTransform("calculateField", excludedOption{path: "options.window.reducer", values: []any{"max"}, decision: decisionTransformationOptions}),
		onTransform("calculateField", excludedOption{path: "options.reduce.nullValueMode", decision: decisionTransformationOptions}),
		onTransform("calculateField", excludedOption{path: "options.timeSeries", values: []any{false}, decision: decisionTransformationOptions}),
		onTransform("convertFieldType", excludedOption{path: "options.conversions[].timezone", decision: decisionTransformationOptions}),
		onTransform("convertFieldType", excludedOption{path: "options.conversions[].enumConfig", decision: decisionTransformationOptions}),
		onTransform("order", excludedOption{path: "options.orderByMode", values: []any{"auto"}, decision: decisionTransformationOptions}),
		onTransform("order", excludedOption{path: "options.orderBy", decision: decisionTransformationOptions}),
		onTransform("partitionByValues", excludedOption{path: "options.naming.withNames", decision: decisionTransformationOptions}),
		onTransform("partitionByValues", excludedOption{path: "options.naming.separator1", decision: decisionTransformationOptions}),
		onTransform("partitionByValues", excludedOption{path: "options.naming.separator2", decision: decisionTransformationOptions}),
		onTransform("rowsToFields", excludedOption{path: "options.nameField", decision: decisionTransformationOptions}),
		onTransform("rowsToFields", excludedOption{path: "options.valueField", decision: decisionTransformationOptions}),
		onTransform("rowsToFields", excludedOption{path: "options.mappings[]", when: isIgnoreOrThresholdHandler, decision: decisionTransformationOptions}),
		onTransform("timeSeriesTable", excludedOption{path: "options.*.inlineStat", values: []any{true}, decision: decisionTransformationOptions}),

		onVariable("datasource", excludedOption{key: "multi", values: []any{true}, decision: decisionMultiDatasourceVariables}),
		onVariable("datasource", excludedOption{key: "includeAll", values: []any{true}, decision: decisionMultiDatasourceVariables}),
		onVariable("datasource", excludedOption{path: "allowCustomValue", values: []any{false}, decision: decisionMultiDatasourceVariables}),
		onVariable("groupby", excludedOption{path: "datasource", key: "type", values: []any{"grafana-clickhouse-datasource", "vertamedia-clickhouse-datasource"}, decision: decisionGroupByClickHouse}),
		anyVariable(excludedOption{path: "hide", values: []any{3}, decision: decisionControlsMenu}),

		onDashboard(excludedOption{path: "annotations.list[]", decision: decisionAnnotations}),
		onDashboard(excludedOption{path: "snapshot", decision: decisionSnapshots}),
		onDashboard(excludedOption{path: "editable", values: []any{false}, decision: decisionReadOnlyDashboards}),
		onDashboard(excludedOption{path: "links[].icon", decision: decisionLinkPlacement}),
		onDashboard(excludedOption{path: "links[].placement", decision: decisionLinkPlacement}),
	},
)

func servedByGrafana(value any) bool {
	address, ok := value.(string)
	address = strings.TrimSpace(address)
	return ok && address != "" && !strings.ContainsAny(address, ":$")
}

func followsRule(value any) bool {
	mode, ok := value.(string)
	return ok && mode != "" && mode != "fixed"
}

func hasIcon(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if icon, ok := v["icon"].(string); ok && icon != "" {
			return true
		}
		for _, member := range v {
			if hasIcon(member) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(v, hasIcon)
	}
	return false
}

func isExploreLink(value any) bool {
	link, ok := asObject(value)
	return ok && present(link["internal"])
}

func hasExcludedMatcher(value any) bool {
	override, _ := asObject(value)
	matcher, ok := asObject(override["matcher"])
	if !ok {
		return false
	}
	switch asString(matcher["id"]) {
	case "anyMatch", "allMatch", "invertMatch":
		return true
	case "byType":
		return asString(matcher["options"]) == "other"
	}
	return false
}

func isIgnoreOrThresholdHandler(value any) bool {
	mapping, _ := asObject(value)
	handler := asString(mapping["handlerKey"])
	return handler == "__ignore" || strings.HasPrefix(handler, "threshold")
}

func (c *converter) exclusionAt(path string) (decision, bool) {
	scoped := c.src.scopeOf(path)
	return exclusionFor(scoped, func(rel string) (any, bool) { return c.src.valueAt(scoped.root, rel) })
}

func exclusionFor(p scopedPath, valueAt func(rel string) (any, bool)) (decision, bool) {
	return exclusionIn(excludedOptions, p, valueAt)
}

func exclusionIn(table []excludedOption, p scopedPath, valueAt func(rel string) (any, bool)) (decision, bool) {
	normalized := normalizePath(p.rel)
	for _, row := range table {
		if row.scope != p.scope && row.scope != p.scope.wildcard() {
			continue
		}
		if !coversPath(row.path, normalized) {
			continue
		}
		if row.key == "" && len(row.values) == 0 && row.when == nil || valueMatches(row, p.rel, valueAt) {
			return row.decision, true
		}
	}
	return decision{}, false
}

func coversPath(rowPath, normalized string) bool {
	if rowPath == "" {
		return true
	}
	row, got := strings.Split(rowPath, "."), strings.Split(normalized, ".")
	if len(got) < len(row) {
		return false
	}
	for i, segment := range row {
		if !segmentCovers(segment, got[i], i == len(row)-1) {
			return false
		}
	}
	return true
}

func segmentCovers(rowSegment, got string, last bool) bool {
	return rowSegment == "*" || rowSegment == got || last && strings.HasPrefix(got, rowSegment+"[")
}

func valueMatches(row excludedOption, rel string, valueAt func(rel string) (any, bool)) bool {
	value, ok := valueAt(prefixSegments(rel, row.path))
	if !ok {
		return false
	}
	if row.key != "" {
		object, isObject := asObject(value)
		if !isObject {
			return false
		}
		if value, ok = lookup(object, row.key); !ok {
			return false
		}
	}
	if len(row.values) > 0 && !slices.ContainsFunc(row.values, func(v any) bool { return sameJSON(value, v) }) {
		return false
	}
	return row.when == nil || row.when(value)
}

func prefixSegments(rel, rowPath string) string {
	if rowPath == "" {
		return ""
	}
	segments := strings.Split(rel, ".")
	return strings.Join(segments[:strings.Count(rowPath, ".")+1], ".")
}

func isDecisionReason(reason string) bool {
	return slices.ContainsFunc(decisions, func(d decision) bool { return d.reason == reason })
}
