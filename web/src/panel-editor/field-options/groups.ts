import {
  fieldOptionSchemas,
  panelOptionSchemas,
  type DataPanelType,
} from "@/spec"

export const optionSchemas = { ...fieldOptionSchemas, ...panelOptionSchemas }

export type OptionKey = keyof typeof optionSchemas

type OptionSchema = (typeof optionSchemas)[OptionKey]

type FormKind = Exclude<OptionSchema["kind"], "json" | "unit" | "thresholds">

export const ownEditorKeys = [
  "unit",
  "thresholds",
] as const satisfies readonly OptionKey[]

export const editInJsonKeys = [
  "mappings",
  "links",
  "actions",
  "cell",
  "lineDash",
  "spanNulls",
  "insertNulls",
] as const satisfies readonly OptionKey[]

export type FormKey = Exclude<
  OptionKey,
  (typeof ownEditorKeys)[number] | (typeof editInJsonKeys)[number]
>

export type OptionGroup = {
  id: GroupId
  title: string
  keys: readonly FormKey[]
}

const hideFrom = ["hideInLegend", "hideInTooltip", "hidden"] as const

const axisKeys = [
  "axis",
  "axisLabel",
  "axisSoftMin",
  "axisSoftMax",
  "scale",
  "logBase",
  "linearThreshold",
  "axisCenteredZero",
  "axisWidth",
  "axisGrid",
  "axisBorder",
  "axisColor",
] as const satisfies readonly FormKey[]

const seriesKeys = [
  "style",
  "lineWidth",
  "lineInterpolation",
  "lineStyle",
  "fill",
  "points",
  "pointSize",
  "stack",
  "stackGroup",
  "showValues",
  "lineColor",
  "fillColor",
  "pointColor",
  "barAlignment",
  "barWidthFactor",
  "barMaxWidth",
  "thresholdsStyle",
  "valueTransform",
  "fillBelowTo",
  "nullAsZero",
  ...hideFrom,
] as const satisfies readonly FormKey[]

const panelKeys = [
  "description",
  "transparent",
  "titleHidden",
  "timeFrom",
  "timeShift",
  "hideTimeOverride",
  "interval",
  "maxDataPoints",
] as const satisfies readonly FormKey[]

const groupKeys = {
  standard: [
    "decimals",
    "min",
    "max",
    "displayName",
    "noValue",
    "colorMode",
    "colorScheme",
    "color",
    "thresholdMode",
  ],
  values: ["decimals", "min", "max", "displayName", "noValue", "thresholdMode"],
  decimals: ["decimals"],
  range: ["fieldMinMax"],
  gaugeRange: ["fieldMinMax", "neutral"],
  series: seriesKeys,
  seriesWithColorBy: [...seriesKeys, "colorBy"],
  axis: axisKeys,
  bars: ["fill", "lineWidth", "stack", "thresholdsStyle", ...hideFrom],
  histogramBars: ["fill", "lineWidth", "stack", "stackGroup", ...hideFrom],
  timeline: ["fill", "lineWidth", "hideInTooltip", "hidden"],
  xy: [
    "lineWidth",
    "lineStyle",
    "pointShape",
    "pointStrokeWidth",
    "fill",
    ...hideFrom,
  ],
  legend: hideFrom,
  hiddenOnly: ["hidden"],
  column: [
    "width",
    "minWidth",
    "align",
    "filterable",
    "inspect",
    "hideHeader",
    "wrapHeader",
    "fieldDescription",
    "tooltipField",
    "tooltipPlacement",
  ],
  panel: panelKeys,
  zonedPanel: [...panelKeys, "timezone"],
  repeat: ["repeat", "repeatDirection", "maxPerRow"],
} as const satisfies Record<string, readonly FormKey[]>

type GroupId = keyof typeof groupKeys

const groupTitles: Record<GroupId, string> = {
  standard: "Standard options",
  values: "Standard options",
  decimals: "Standard options",
  range: "Range",
  gaugeRange: "Range",
  series: "Series",
  seriesWithColorBy: "Series",
  axis: "Axis",
  bars: "Bars",
  histogramBars: "Bars",
  timeline: "Cells",
  xy: "Points and lines",
  legend: "Series visibility",
  hiddenOnly: "Series visibility",
  column: "Table columns",
  panel: "Panel",
  zonedPanel: "Panel",
  repeat: "Repeat",
}

const plot = ["standard", "series", "axis"] as const
const ending = ["panel", "repeat"] as const
const zonedEnding = ["zonedPanel", "repeat"] as const

const groupsByType: Record<DataPanelType, readonly GroupId[]> = {
  timeseries: ["standard", "seriesWithColorBy", "axis", ...zonedEnding],
  trend: [...plot, ...ending],
  candlestick: [...plot, ...ending],
  stat: ["standard", ...ending],
  gauge: ["standard", "gaugeRange", ...ending],
  bargauge: ["standard", "range", ...ending],
  table: ["standard", "column", ...ending],
  piechart: ["standard", "legend", ...ending],
  heatmap: ["values", "hiddenOnly", ...ending],
  histogram: ["standard", "histogramBars", "axis", ...ending],
  barchart: ["standard", "bars", "axis", ...ending],
  "state-timeline": ["standard", "timeline", ...zonedEnding],
  "status-history": ["standard", "timeline", ...zonedEnding],
  xychart: ["standard", "xy", "axis", ...ending],
  geomap: ["standard", "hiddenOnly", ...ending],
  canvas: ["standard", "hiddenOnly", ...ending],
  flamegraph: ["decimals", ...ending],
  logs: ending,
  traces: ending,
  nodeGraph: ending,
  alertlist: ending,
}

export function optionGroupsFor(type: DataPanelType): OptionGroup[] {
  return groupsByType[type].map((id) => ({
    id,
    title: groupTitles[id],
    keys: groupKeys[id],
  }))
}

export function formSchema(key: FormKey) {
  return optionSchemas[key] as Extract<OptionSchema, { kind: FormKind }>
}

export const optionLabels: Record<FormKey, string> = {
  decimals: "Decimals",
  min: "Min",
  max: "Max",
  displayName: "Display name",
  noValue: "No value text",
  colorMode: "Color mode",
  colorScheme: "Color scheme",
  color: "Color",
  thresholdMode: "Threshold mode",
  fieldMinMax: "Range each field by its own values",
  neutral: "Neutral value",
  style: "Draw style",
  lineWidth: "Line width",
  lineInterpolation: "Line interpolation",
  lineStyle: "Line style",
  fill: "Fill opacity",
  points: "Show points",
  pointSize: "Point size",
  pointShape: "Point shape",
  pointStrokeWidth: "Point stroke width",
  stack: "Stack series",
  stackGroup: "Stack group",
  showValues: "Value labels",
  lineColor: "Line color",
  fillColor: "Fill color",
  pointColor: "Point color",
  colorBy: "Color by value",
  barAlignment: "Bar alignment",
  barWidthFactor: "Bar width factor",
  barMaxWidth: "Max bar width",
  thresholdsStyle: "Show thresholds",
  valueTransform: "Transform",
  fillBelowTo: "Fill below to",
  nullAsZero: "Treat nulls as zero",
  hideInLegend: "Hide in legend",
  hideInTooltip: "Hide in tooltip",
  hidden: "Hide in chart",
  axis: "Axis placement",
  axisLabel: "Axis label",
  axisSoftMin: "Soft min",
  axisSoftMax: "Soft max",
  scale: "Scale",
  logBase: "Log base",
  linearThreshold: "Linear threshold",
  axisCenteredZero: "Center zero",
  axisWidth: "Axis width",
  axisGrid: "Grid lines",
  axisBorder: "Axis border",
  axisColor: "Axis color",
  width: "Column width",
  minWidth: "Min column width",
  align: "Cell alignment",
  filterable: "Column filter",
  inspect: "Cell inspect",
  hideHeader: "Hide header",
  wrapHeader: "Wrap header",
  fieldDescription: "Field description",
  tooltipField: "Cell tooltip field",
  tooltipPlacement: "Cell tooltip placement",
  description: "Description",
  transparent: "Transparent background",
  titleHidden: "Hide title",
  timeFrom: "Relative time",
  timeShift: "Time shift",
  hideTimeOverride: "Hide time info",
  interval: "Min interval",
  maxDataPoints: "Max data points",
  timezone: "Time zone",
  repeat: "Repeat for variable",
  repeatDirection: "Repeat direction",
  maxPerRow: "Max per row",
}

export const optionPlaceholders: Partial<Record<FormKey, string>> = {
  displayName: "Series name",
  noValue: "No data",
  stackGroup: "A",
  fillBelowTo: "Series name",
  axisLabel: "None",
  tooltipField: "Field name",
  timeFrom: "now-1h",
  timeShift: "1h",
  interval: "1m",
  timezone: "Dashboard time zone",
  repeat: "Variable name",
  color: "ok or #rrggbb",
  lineColor: "ok or #rrggbb",
  fillColor: "ok or #rrggbb",
  pointColor: "ok or #rrggbb",
}

const valueLabelOverrides: Record<string, string> = {
  h: "Horizontal",
  v: "Vertical",
  "line+area": "Line and area",
  "dashed+area": "Dashed and area",
  negativeY: "Negative Y",
  symlog: "Symmetric log",
  paletteByName: "Palette by name",
}

export function valueLabel(value: string | number): string {
  const text = String(value)
  const override = valueLabelOverrides[text]
  if (override !== undefined) return override
  const words = text.replace(/([a-z])([A-Z])/g, "$1 $2").toLowerCase()
  return words.charAt(0).toUpperCase() + words.slice(1)
}
