import { sameJson } from "@/lib/json"
import { joinNames } from "@/observe/conversation"
import {
  isDashboardQuery,
  isDataPanel,
  isPanelQuery,
  queryRef,
  type DataQuery,
  type Threshold,
} from "@/spec"

import type { EditablePanel } from "./draft"

export type Change = { key: string; label: string }

type Fields = Record<string, unknown>

const summaryLabelCount = 3
const maxSummaryCodePoints = 500

const semanticColors = ["ok", "info", "warning", "critical"]

const displayLabels: Record<string, string> = {
  legend: "legend",
  legendValues: "legend values",
  tooltip: "tooltip",
  reduce: "calculation",
  colorMode: "color mode",
  graph: "graph",
  textMode: "text mode",
  orientation: "orientation",
  thresholdMarkers: "threshold markers",
  header: "table header",
  footer: "table footer",
  footerFields: "footer fields",
  footerCountRows: "footer row count",
  cellHeight: "cell height",
  pagination: "pagination",
  frozenColumns: "frozen columns",
  sortBy: "sort column",
  sortDesc: "sort direction",
  typeIcons: "type icons",
  pieType: "pie type",
  calculate: "heatmap calculation",
  yScale: "y scale",
  colorScheme: "color scheme",
  colorSteps: "color steps",
  colorScale: "opacity scale",
  colorExponent: "opacity exponent",
  colorFill: "opacity color",
  colorMin: "color range start",
  colorMax: "color range end",
  colorReverse: "reversed colors",
  cellGap: "cell gap",
  cellRadius: "cell radius",
  cellUnit: "cell value unit",
  cellDecimals: "cell value decimals",
  hideBelow: "lower value filter",
  hideAbove: "upper value filter",
  bucketSize: "bucket size",
  bucketCount: "bucket count",
  mergeValues: "merged values",
  showValues: "value labels",
  wrap: "line wrap",
  showTime: "time column",
  order: "order",
  xField: "x field",
  barWidth: "bar width",
  groupWidth: "group width",
  show: "series display",
  pointSize: "point size",
  mode: "mode",
  candleStyle: "candle style",
  colorStrategy: "color strategy",
  upColor: "up color",
  downColor: "down color",
  open: "open field",
  high: "high field",
  low: "low field",
  close: "close field",
  volume: "volume field",
  layout: "layout",
  wheelZoom: "wheel zoom",
  spanSearch: "span search",
  serviceNames: "service filter",
  excludeServices: "excluded services",
  spanNames: "operation filter",
  excludeSpanNames: "excluded operations",
  minSpanDuration: "shortest span",
  maxSpanDuration: "longest span",
  matchesOnly: "matching spans only",
  criticalPathOnly: "critical path only",
  view: "map view",
  lat: "latitude",
  lon: "longitude",
  zoom: "zoom",
  location: "location source",
  latField: "latitude field",
  lonField: "longitude field",
  geohashField: "geohash field",
  sizeField: "size field",
  colorField: "color field",
  markerSize: "marker size",
  markerSizeMax: "largest marker size",
  basemap: "basemap",
  basemapTheme: "basemap theme",
  basemapLabels: "basemap labels",
  esriServer: "Esri map",
  basemapUrl: "basemap URL",
  basemapAttribution: "basemap attribution",
  basemapOpacity: "basemap opacity",
  minZoom: "lowest zoom",
  maxZoom: "highest zoom",
  fitPadding: "fit padding",
  showZoom: "zoom buttons",
  mouseWheelZoom: "mouse wheel zoom",
  showAttribution: "attribution",
  showScale: "scale bar",
  showMeasure: "measure tool",
  showDebug: "zoom and center readout",
  elements: "canvas elements",
  connections: "canvas connections",
  limit: "limit",
  sort: "sort order",
  viewMode: "view",
  query: "search",
  tags: "tags",
  keepTime: "time range",
  includeVars: "variable values",
}

const fieldLabels: Record<string, string> = {
  description: "description",
  decimals: "decimals",
  min: "minimum",
  max: "maximum",
  thresholdMode: "threshold mode",
  color: "color",
  displayName: "display name",
  noValue: "no value text",
  mappings: "value mappings",
  links: "links",
  style: "draw style",
  fill: "fill",
  lineStyle: "line style",
  stack: "stacking",
  axis: "axis",
  hidden: "hidden",
  width: "column width",
  minWidth: "minimum column width",
  align: "alignment",
  filterable: "column filter",
  inspect: "cell inspection",
  hideHeader: "column header",
  overrides: "overrides",
  transform: "transformations",
  repeat: "repeat",
  repeatDirection: "repeat direction",
  maxPerRow: "repeat width",
  timeFrom: "relative time",
  timeShift: "time shift",
  interval: "minimum interval",
  maxDataPoints: "max data points",
  gridPos: "position",
}

const coveredKeys = new Set([
  "queries",
  "datasource",
  "title",
  "type",
  "unit",
  "content",
  "thresholds",
  "display",
])

function labelFor(labels: Record<string, string>, key: string): string {
  return Object.hasOwn(labels, key) ? labels[key] : key
}

function sortedUnion(a: Fields, b: Fields): string[] {
  return [...new Set([...Object.keys(a), ...Object.keys(b)])].sort()
}

function trimmed(value: unknown): unknown {
  return typeof value === "string" ? value.trim() : value
}

function queriesOf(panel: Fields): DataQuery[] {
  return Array.isArray(panel.queries) ? (panel.queries as DataQuery[]) : []
}

function withoutRef(query: DataQuery): Fields {
  const rest: Fields = { ...query }
  delete rest.ref
  return rest
}

function normaliseQuery(query: DataQuery): DataQuery {
  if (!isPanelQuery(query)) return query
  const next = { ...query }
  const legend = next.legend?.trim()
  if (legend) next.legend = legend
  else delete next.legend
  return next
}

export function normalisePanel(panel: EditablePanel): EditablePanel {
  const next = structuredClone(panel)
  if (next.title !== undefined) next.title = next.title.trim()
  if (isDataPanel(next)) next.queries = next.queries.map(normaliseQuery)
  return next
}

export function queryChanged(base: DataQuery, draft: DataQuery): boolean {
  return !sameJson(
    withoutRef(normaliseQuery(base)),
    withoutRef(normaliseQuery(draft))
  )
}

function sameQueries(a: unknown, b: unknown): boolean {
  if (!Array.isArray(a) || !Array.isArray(b)) return sameJson(a, b)
  const before = a as DataQuery[]
  const after = b as DataQuery[]
  return (
    before.length === after.length &&
    before.every(
      (query, index) =>
        queryRef(query, index) === queryRef(after[index], index) &&
        !queryChanged(query, after[index])
    )
  )
}

const sameValueOf = new Map<string, (a: unknown, b: unknown) => boolean>([
  ["title", (a, b) => sameJson(trimmed(a), trimmed(b))],
  ["queries", sameQueries],
])

function changedTopKeys(base: Fields, draft: Fields): string[] {
  return sortedUnion(base, draft).filter(
    (key) => !(sameValueOf.get(key) ?? sameJson)(base[key], draft[key])
  )
}

function orKey(key: string, changes: Change[]): Change[] {
  return changes.length > 0 ? changes : [{ key, label: key }]
}

function queryChanges(base: Fields, draft: Fields): Change[] {
  const baseQueries = queriesOf(base)
  const draftQueries = queriesOf(draft)
  const single = draftQueries.length === 1
  const baseByRef = new Map(
    baseQueries.map((query, index) => [queryRef(query, index), query])
  )
  const draftRefs = new Set<string>()
  const changes: Change[] = []

  draftQueries.forEach((query, index) => {
    const ref = queryRef(query, index)
    draftRefs.add(ref)
    const before = baseByRef.get(ref)
    const name = single ? "query" : `query ${ref}`
    if (before === undefined) {
      changes.push({ key: `queries.${ref}.new`, label: `new query ${ref}` })
      return
    }
    if (!isPanelQuery(before) || !isPanelQuery(query)) {
      if (!sameJson(withoutRef(before), withoutRef(query))) {
        const reuses = isDashboardQuery(before) || isDashboardQuery(query)
        changes.push({
          key: `queries.${ref}.${reuses ? "source" : "expression"}`,
          label: `${reuses ? "reused panel" : "expression"} ${ref}`,
        })
      }
      return
    }
    if (!sameJson(before.query, query.query)) {
      changes.push({ key: `queries.${ref}.query`, label: name })
    }
    if (
      !sameJson(
        before.legend?.trim() || undefined,
        query.legend?.trim() || undefined
      )
    ) {
      changes.push({
        key: `queries.${ref}.legend`,
        label: single ? "legend" : `legend ${ref}`,
      })
    }
    if (!sameJson(before.datasource, query.datasource)) {
      changes.push({
        key: `queries.${ref}.datasource`,
        label: `${name} datasource`,
      })
    }
    const options = (value: DataQuery) => {
      const rest = withoutRef(value)
      for (const key of ["query", "legend", "datasource"]) {
        delete rest[key]
      }
      return rest
    }
    if (!sameJson(options(before), options(query))) {
      changes.push({ key: `queries.${ref}.options`, label: `${name} options` })
    }
  })

  baseQueries.forEach((query, index) => {
    const ref = queryRef(query, index)
    if (!draftRefs.has(ref)) {
      changes.push({
        key: `queries.${ref}.removed`,
        label: `removed query ${ref}`,
      })
    }
  })
  return changes
}

function stepsOf(panel: Fields): Threshold[] {
  return Array.isArray(panel.thresholds)
    ? (panel.thresholds as Threshold[])
    : []
}

function thresholdChanges(base: Fields, draft: Fields): Change[] {
  const before = stepsOf(base)
  const after = stepsOf(draft)
  const valued = (steps: Threshold[]) =>
    steps.filter((step) => step.value !== undefined)
  const changes: Change[] = []

  for (const color of semanticColors) {
    const values = (steps: Threshold[]) =>
      valued(steps)
        .filter((step) => step.color === color)
        .map((step) => step.value)
    if (!sameJson(values(before), values(after))) {
      changes.push({ key: `thresholds.${color}`, label: `${color} threshold` })
    }
  }

  const hex = (steps: Threshold[]) =>
    valued(steps).filter((step) => !semanticColors.includes(step.color))
  if (!sameJson(hex(before), hex(after))) {
    changes.push({ key: "thresholds.hex", label: "threshold" })
  }

  const baseStep = (steps: Threshold[]) =>
    steps[0]?.value === undefined ? steps[0] : undefined
  if (!sameJson(baseStep(before), baseStep(after))) {
    changes.push({ key: "thresholds.base", label: "base color" })
  }
  return changes
}

function displayOf(panel: Fields): Fields {
  return typeof panel.display === "object" && panel.display !== null
    ? (panel.display as Fields)
    : {}
}

function displayChanges(base: Fields, draft: Fields): Change[] {
  const before = displayOf(base)
  const after = displayOf(draft)
  return sortedUnion(before, after)
    .filter((key) => !sameJson(before[key], after[key]))
    .map((key) => ({
      key: `display.${key}`,
      label: labelFor(displayLabels, key),
    }))
}

export function panelChanges(
  base: EditablePanel,
  draft: EditablePanel
): Change[] {
  const before: Fields = { ...base }
  const after: Fields = { ...draft }
  const changed = new Set(changedTopKeys(before, after))
  const changes: Change[] = []

  if (changed.has("queries")) {
    changes.push(...orKey("queries", queryChanges(before, after)))
  }

  const simple: [string, string][] = [
    ["datasource", "datasource"],
    ["title", "title"],
    ["type", "panel type"],
    ["unit", "unit"],
    ["content", "content"],
  ]
  for (const [key, label] of simple) {
    if (changed.has(key)) changes.push({ key, label })
  }

  if (changed.has("thresholds")) {
    changes.push(...orKey("thresholds", thresholdChanges(before, after)))
  }
  if (changed.has("display")) {
    changes.push(...orKey("display", displayChanges(before, after)))
  }

  for (const key of changed) {
    if (!coveredKeys.has(key)) {
      changes.push({ key, label: labelFor(fieldLabels, key) })
    }
  }
  return changes
}

export function changeSummary(changes: readonly Change[]): string {
  const n = changes.length
  if (n === 0) return "No changes"
  const shown = changes
    .slice(0, summaryLabelCount)
    .map((change) => change.label)
    .join(", ")
  const more =
    n > summaryLabelCount ? `, and ${n - summaryLabelCount} more` : ""
  return `${n} ${n === 1 ? "change" : "changes"}: ${shown}${more}`
}

export function defaultSaveSummary(
  changes: readonly Change[],
  title: string
): string {
  const names = joinNames(
    changes.slice(0, summaryLabelCount).map((change) => change.label)
  )
  const more = changes.length > summaryLabelCount ? " and more" : ""
  return Array.from(`Changed ${names}${more} in "${title.trim()}"`)
    .slice(0, maxSummaryCodePoints)
    .join("")
}

export function rebasePanel(
  base: EditablePanel,
  draft: EditablePanel,
  latest: EditablePanel
): EditablePanel {
  const before: Fields = { ...base }
  const after: Fields = { ...draft }
  const result: Fields = { ...structuredClone(latest) }
  for (const key of changedTopKeys(before, after)) {
    if (Object.hasOwn(after, key)) result[key] = structuredClone(after[key])
    else delete result[key]
  }
  return result as unknown as EditablePanel
}
