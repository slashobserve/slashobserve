import { axisDecimals, formatValue } from "@/lib/units"
import { compileNamePattern, displayWithDefaults, type Reducer } from "@/spec"

import { rangePosition } from "../color-scheme"
import { styleForSlot } from "../colors"
import {
  displayValue,
  effectiveColorMode,
  fieldRange,
  type FieldConfig,
  type FieldResolver,
} from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import type { SeriesLegendEntry } from "../legend/use-series-legend"
import { isNumber } from "../reducers"
import { thresholdColor } from "../thresholds"
import { tooltipRows, type TooltipSort } from "../timeseries/tooltip-rows"
import {
  matchesName,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"
import {
  linearScale,
  logTicks,
  scaleOf,
  scalePosition,
  symlogTicks,
  valueAxisSettings,
  valueRange,
  type ValueAxisSettings,
  type ValueScale,
} from "../value-axis"

export type XyShow = "points" | "lines" | "points+lines"
type XyLegend = "bottom" | "right" | "hidden"
type XyTooltip = "single" | "all" | "hidden"

export type XyOptions = {
  show: XyShow
  pointSize: number
  pointSizeMin: number
  pointSizeMax: number
  legend: XyLegend
  legendMode?: "list" | "table"
  legendValues: Reducer[]
  legendSortBy?: "name" | Reducer
  legendSortDesc: boolean
  legendWidth?: number
  tooltip: XyTooltip
  tooltipSort: TooltipSort
  tooltipHideZeros: boolean
  tooltipMaxWidth?: number
  tooltipMaxHeight?: number
}

type XyPoint = readonly [
  x: number,
  y: number,
  color: string,
  diameter: number,
  size: number | null,
  tone: number | null,
]

type XyChannel = { name: string; config: FieldConfig }

export type XySeries = {
  key: string
  name: string
  color: string
  xName: string
  yName: string
  xConfig: FieldConfig
  yConfig: FieldConfig
  size: XyChannel | null
  tone: XyChannel | null
  points: XyPoint[]
  lineWidth: number
  dash: number[]
  square: boolean
  strokeWidth: number
  fillAlpha: number
  hideInLegend: boolean
  hideInTooltip: boolean
}

export type XyRange = { min: number; max: number }

export type XyAxis = {
  name: string
  config: FieldConfig | undefined
  settings: ValueAxisSettings
  scale: ValueScale
  range: XyRange
  color: string | null
}

export type XyData = { series: XySeries[]; reason: string | null }

export type XyModel = {
  series: XySeries[]
  x: XyAxis
  y: XyAxis
  reason: string | null
}

export type XyScale = { width: number; height: number; radius: number }

export type XyHit = { series: number; index: number; distance: number }

export type XyTick = { value: number; text: string }

export type XyPointSummary = {
  x: string
  y: string
  extras: { name: string; text: string }[]
}

type SeriesMapping = {
  query?: string
  frame?: string
  x?: string
  y?: string
  size?: string
  color?: string
  name?: string
}

export const missingFieldsReason =
  "An XY chart needs an x field and another number field in the same frame; join queries with a joinByField transform."

const rangePadding = 0.05
const emptyRange: XyRange = { min: 0, max: 1 }
const emptyLogRange: XyRange = { min: 1, max: 10 }
const defaultLineWidth = 1.5
const dashedPattern = [8, 4]
const dottedPattern = [1.5, 3]
const autoMapping: readonly SeriesMapping[] = [{}]
const noneHidden: ReadonlySet<string> = new Set()
const baseAxisSettings = valueAxisSettings({
  axis: "left",
  axisBorder: false,
  axisColor: "text",
  scale: "linear",
  logBase: 10,
  axisCenteredZero: false,
})

function numberOf(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined
}

function textOf(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

export function xyOptions(display: Record<string, unknown>): XyOptions {
  const merged = displayWithDefaults("xychart", display)
  const sortBy = merged.legendSortBy
  const sizes = [
    numberOf(merged.pointSizeMin) ?? 1,
    numberOf(merged.pointSizeMax) ?? 1,
  ]
  return {
    show:
      merged.show === "lines" || merged.show === "points+lines"
        ? merged.show
        : "points",
    pointSize: numberOf(merged.pointSize) ?? 5,
    pointSizeMin: Math.min(...sizes),
    pointSizeMax: Math.max(...sizes),
    legend:
      merged.legend === "right" || merged.legend === "hidden"
        ? merged.legend
        : "bottom",
    legendMode:
      merged.legendMode === "list" || merged.legendMode === "table"
        ? merged.legendMode
        : undefined,
    legendValues: (merged.legendValues ?? []) as Reducer[],
    legendSortBy:
      typeof sortBy === "string" ? (sortBy as "name" | Reducer) : undefined,
    legendSortDesc: merged.legendSortDesc === true,
    legendWidth: numberOf(merged.legendWidth),
    tooltip:
      merged.tooltip === "all" || merged.tooltip === "hidden"
        ? merged.tooltip
        : "single",
    tooltipSort:
      merged.tooltipSort === "asc" || merged.tooltipSort === "desc"
        ? merged.tooltipSort
        : "none",
    tooltipHideZeros: merged.tooltipHideZeros === true,
    tooltipMaxWidth: numberOf(merged.tooltipMaxWidth),
    tooltipMaxHeight: numberOf(merged.tooltipMaxHeight),
  }
}

function seriesMappings(
  display: Record<string, unknown>
): readonly SeriesMapping[] {
  const series = display.series
  if (!Array.isArray(series) || series.length === 0) return autoMapping
  return series.map((entry: unknown) => {
    const given = (entry ?? {}) as Record<string, unknown>
    return {
      query: textOf(given.query),
      frame: textOf(given.frame),
      x: textOf(given.x),
      y: textOf(given.y),
      size: textOf(given.size),
      color: textOf(given.color),
      name: textOf(given.name),
    }
  })
}

type NameTest = (name: string) => boolean

function nameTest(pattern: string): NameTest {
  if (!pattern.startsWith("/")) return (name) => name === pattern
  const regex = compileNamePattern(pattern)
  return regex ? (name) => regex.test(name) : () => false
}

function frameMatches(frame: PreparedFrame, mapping: SeriesMapping): boolean {
  if (mapping.query !== undefined && frame.ref !== mapping.query) return false
  return (
    mapping.frame === undefined || nameTest(mapping.frame)(frame.name ?? "")
  )
}

function numberFieldNamed(
  frame: PreparedFrame,
  name: string | undefined
): PreparedField | undefined {
  if (name === undefined) return frame.fields.find((f) => f.type === "number")
  return frame.fields.find(
    (field) => field.type === "number" && matchesName(frame, field, name)
  )
}

function yFields(
  frame: PreparedFrame,
  mapping: SeriesMapping,
  excluded: readonly (PreparedField | undefined)[]
): PreparedField[] {
  const test = mapping.y === undefined ? null : nameTest(mapping.y)
  return frame.fields.filter(
    (field) =>
      field.type === "number" &&
      !excluded.includes(field) &&
      (test === null || test(field.name) || test(seriesName(frame, field)))
  )
}

function paints(config: FieldConfig): boolean {
  return (
    (config.thresholds?.length ?? 0) > 0 ||
    config.mappings.length > 0 ||
    effectiveColorMode(config) === "scheme"
  )
}

type Sizing = {
  field: PreparedField
  min: number
  max: number
  smallest: number
  largest: number
}

function sizingOf(
  field: PreparedField | undefined,
  config: FieldConfig | undefined,
  options: XyOptions
): Sizing | null {
  if (!field || !config) return null
  const range = fieldRange(field)
  const min = config.min ?? range.min
  const max = config.max ?? range.max
  if (min === undefined || max === undefined) return null
  return {
    field,
    min,
    max,
    smallest: options.pointSizeMin,
    largest: options.pointSizeMax,
  }
}

function diameterOf(
  value: number | null,
  sizing: Sizing | null,
  fixed: number
) {
  if (sizing === null || value === null) return fixed
  const share = rangePosition(value, sizing.min, sizing.max)
  return sizing.smallest + share * (sizing.largest - sizing.smallest)
}

type PointSource = {
  x: PreparedField
  y: PreparedField
  yConfig: FieldConfig
  seriesColor: string
  sizing: Sizing | null
  tone: { field: PreparedField; config: FieldConfig } | null
  fixed: number
}

function numberAt(field: PreparedField | undefined, index: number) {
  const value = field?.values[index]
  return isNumber(value) ? value : null
}

function pointsOf(source: PointSource): XyPoint[] {
  const { x, y, yConfig, seriesColor, sizing, tone, fixed } = source
  const colorByY = tone === null && paints(yConfig)
  const { min, max } = fieldRange(y)
  const toneRange = tone ? fieldRange(tone.field) : {}
  const length = Math.min(x.values.length, y.values.length)
  const points: XyPoint[] = []
  for (let i = 0; i < length; i++) {
    const px = x.values[i]
    const py = y.values[i]
    if (!isNumber(px) || !isNumber(py)) continue
    const size = numberAt(sizing?.field, i)
    const toneValue = numberAt(tone?.field, i)
    let color = seriesColor
    if (tone !== null && toneValue !== null) {
      color =
        displayValue(toneValue, tone.config, {
          fieldMin: toneRange.min,
          fieldMax: toneRange.max,
        }).color ?? seriesColor
    } else if (colorByY) {
      color =
        displayValue(py, yConfig, { fieldMin: min, fieldMax: max }).color ??
        seriesColor
    }
    points.push([
      px,
      py,
      color,
      diameterOf(size, sizing, fixed),
      size,
      toneValue,
    ])
  }
  return points
}

function dashOf(config: FieldConfig): number[] {
  if (config.lineStyle === "dashed") return dashedPattern
  return config.lineStyle === "dotted" ? dottedPattern : []
}

export function xyData(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): XyData {
  const options = xyOptions(display)
  const seen = new Map<string, number>()
  const sorted = options.show !== "points"
  const series: XySeries[] = []
  const defaultX = textOf(display.xField)
  let qualified = false
  for (const mapping of seriesMappings(display)) {
    const built: XySeries[] = []
    for (const frame of frames) {
      if (!frameMatches(frame, mapping)) continue
      const x = numberFieldNamed(frame, mapping.x ?? defaultX)
      if (!x) continue
      const sizeField = mapping.size
        ? numberFieldNamed(frame, mapping.size)
        : undefined
      const toneField = mapping.color
        ? numberFieldNamed(frame, mapping.color)
        : undefined
      const ys = yFields(frame, mapping, [x, sizeField, toneField])
      if (ys.length === 0) continue
      qualified = true
      const xConfig = fields.configFor(frame, x)
      const sizeConfig = sizeField && fields.configFor(frame, sizeField)
      const toneConfig = toneField && fields.configFor(frame, toneField)
      const sizing = sizingOf(sizeField, sizeConfig, options)
      for (const y of ys) {
        const config = fields.configFor(frame, y)
        if (config.hidden) continue
        const color = config.color
          ? thresholdColor(config.color)
          : styleForSlot(series.length + built.length).color
        const points = pointsOf({
          x,
          y,
          yConfig: config,
          seriesColor: color,
          sizing,
          tone:
            toneField && toneConfig
              ? { field: toneField, config: toneConfig }
              : null,
          fixed: options.pointSize,
        })
        if (sorted) points.sort((a, b) => a[0] - b[0])
        built.push({
          key: uniqueKey(seen, seriesKey(frame, y.name)),
          name: fields.displayName(frame, y),
          color,
          xName: x.name,
          yName: y.name,
          xConfig,
          yConfig: config,
          size:
            sizeField && sizeConfig
              ? { name: sizeField.name, config: sizeConfig }
              : null,
          tone:
            toneField && toneConfig
              ? { name: toneField.name, config: toneConfig }
              : null,
          points,
          lineWidth: fields.isSet(frame, y, "lineWidth")
            ? config.lineWidth
            : defaultLineWidth,
          dash: dashOf(config),
          square: config.pointShape === "square",
          strokeWidth: config.pointStrokeWidth ?? 0,
          fillAlpha: fields.isSet(frame, y, "fill") ? config.fill / 100 : 1,
          hideInLegend: config.hideInLegend,
          hideInTooltip: config.hideInTooltip,
        })
      }
    }
    const named = mapping.name
    series.push(
      ...built.map((item) =>
        named === undefined
          ? item
          : {
              ...item,
              name: built.length === 1 ? named : `${named} ${item.name}`,
            }
      )
    )
  }
  return { series, reason: qualified ? null : missingFieldsReason }
}

export function legendEntries(
  series: readonly XySeries[]
): SeriesLegendEntry[] {
  return series.map(({ key, name, color, points, yConfig, hideInLegend }) => ({
    key,
    label: name,
    color,
    values: points.map((point) => point[1]),
    config: yConfig,
    hideInLegend,
  }))
}

function dimensionName(item: XySeries, dimension: 0 | 1): string {
  return dimension === 0 ? item.xName : item.yName
}

function dimensionConfig(item: XySeries, dimension: 0 | 1): FieldConfig {
  return dimension === 0 ? item.xConfig : item.yConfig
}

function paddedRange(min: number, max: number): [number, number] {
  const span = max - min || Math.abs(max) || 1
  return [min - span * rangePadding, max + span * rangePadding]
}

function axisRange(
  series: readonly XySeries[],
  dimension: 0 | 1,
  settings: ValueAxisSettings
): XyRange {
  let min = Infinity
  let max = -Infinity
  let positiveMin = Infinity
  for (const { points } of series) {
    for (const point of points) {
      const value = point[dimension]
      if (value < min) min = value
      if (value > max) max = value
      if (value > 0 && value < positiveMin) positiveMin = value
    }
  }
  const logarithmic = settings.scale === "log"
  if (min > max) return logarithmic ? emptyLogRange : emptyRange
  const auto: [number, number] = logarithmic
    ? [positiveMin, max]
    : paddedRange(min, max)
  const [low, high] = valueRange(auto, positiveMin, settings)
  if (logarithmic) return { min: low, max: high }
  let widenedLow = low
  let widenedHigh = high
  for (const item of series) {
    const { min: floor, max: ceiling } = dimensionConfig(item, dimension)
    if (floor !== undefined && floor < widenedLow) widenedLow = floor
    if (ceiling !== undefined && ceiling > widenedHigh) widenedHigh = ceiling
  }
  return { min: widenedLow, max: widenedHigh }
}

function axisOf(
  series: readonly XySeries[],
  dimension: 0 | 1
): {
  name: string
  config: FieldConfig | undefined
  settings: ValueAxisSettings
} {
  const first = series[0]
  if (!first) {
    return { name: "", config: undefined, settings: baseAxisSettings }
  }
  const config = dimensionConfig(first, dimension)
  return {
    name: dimensionName(first, dimension),
    config,
    settings: valueAxisSettings(config),
  }
}

function drawable(series: XySeries, keep: (point: XyPoint) => boolean) {
  return series.points.every(keep)
    ? series
    : { ...series, points: series.points.filter(keep) }
}

export function xyModel(
  data: XyData,
  hidden: ReadonlySet<string> = noneHidden
): XyModel {
  const shown = data.series.filter(({ key }) => !hidden.has(key))
  const horizontal = axisOf(shown, 0)
  const vertical = axisOf(shown, 1)
  const logX = horizontal.settings.scale === "log"
  const logY = vertical.settings.scale === "log"
  const series =
    logX || logY
      ? shown.map((item) =>
          drawable(item, (p) => (!logX || p[0] > 0) && (!logY || p[1] > 0))
        )
      : shown
  const colorOf = (settings: ValueAxisSettings) =>
    settings.seriesColored && series[0] ? series[0].color : null
  return {
    series,
    x: {
      ...horizontal,
      scale: scaleOf(horizontal.settings),
      range: axisRange(series, 0, horizontal.settings),
      color: colorOf(horizontal.settings),
    },
    y: {
      ...vertical,
      scale: scaleOf(vertical.settings),
      range: axisRange(series, 1, vertical.settings),
      color: colorOf(vertical.settings),
    },
    reason: data.reason,
  }
}

export function pointCount(model: XyModel): number {
  return model.series.reduce((sum, { points }) => sum + points.length, 0)
}

export function axisFraction(axis: XyAxis, value: number): number {
  return scalePosition(value, axis.range.min, axis.range.max, axis.scale)
}

type Grid = {
  size: number
  starts: Int32Array
  seriesOf: Int32Array
  indexOf: Int32Array
}

const maxGridSize = 256
const pointsPerCell = 4
const grids = new WeakMap<XyModel, Grid>()

function cellOf(fraction: number, size: number): number {
  return Math.min(size - 1, Math.max(0, Math.floor(fraction * size)))
}

function buildGrid(model: XyModel): Grid {
  const total = pointCount(model)
  const size = Math.min(
    maxGridSize,
    Math.max(1, Math.ceil(Math.sqrt(total / pointsPerCell)))
  )
  const cells = new Int32Array(total)
  const starts = new Int32Array(size * size + 1)
  let next = 0
  model.series.forEach(({ points }) => {
    for (const [x, y] of points) {
      const cell =
        cellOf(axisFraction(model.y, y), size) * size +
        cellOf(axisFraction(model.x, x), size)
      cells[next++] = cell
      starts[cell + 1]++
    }
  })
  for (let i = 0; i < size * size; i++) starts[i + 1] += starts[i]
  const fill = starts.slice(0, size * size)
  const seriesOf = new Int32Array(total)
  const indexOf = new Int32Array(total)
  next = 0
  model.series.forEach(({ points }, series) => {
    for (let index = 0; index < points.length; index++) {
      const slot = fill[cells[next++]]++
      seriesOf[slot] = series
      indexOf[slot] = index
    }
  })
  return { size, starts, seriesOf, indexOf }
}

function gridOf(model: XyModel): Grid {
  let grid = grids.get(model)
  if (!grid) {
    grid = buildGrid(model)
    grids.set(model, grid)
  }
  return grid
}

export function nearestPoint(
  model: XyModel,
  px: number,
  py: number,
  scale: XyScale
): XyHit | null {
  if (scale.width <= 0 || scale.height <= 0 || pointCount(model) === 0) {
    return null
  }
  const grid = gridOf(model)
  const column = cellOf(px / scale.width, grid.size)
  const row = cellOf(1 - py / scale.height, grid.size)
  const cellSize = Math.min(scale.width, scale.height) / grid.size
  let best: XyHit | null = null
  let bestSquared = scale.radius * scale.radius
  for (let ring = 0; ring <= grid.size; ring++) {
    const reach = (ring - 1) * cellSize
    if (ring > 1 && reach * reach > bestSquared) break
    for (let r = row - ring; r <= row + ring; r++) {
      if (r < 0 || r >= grid.size) continue
      const edge = r === row - ring || r === row + ring
      const step = edge ? 1 : 2 * ring || 1
      for (let c = column - ring; c <= column + ring; c += step) {
        if (c < 0 || c >= grid.size) continue
        const cell = r * grid.size + c
        for (
          let slot = grid.starts[cell];
          slot < grid.starts[cell + 1];
          slot++
        ) {
          const series = grid.seriesOf[slot]
          const index = grid.indexOf[slot]
          const point = model.series[series].points[index]
          const dx = axisFraction(model.x, point[0]) * scale.width - px
          const dy = (1 - axisFraction(model.y, point[1])) * scale.height - py
          const squared = dx * dx + dy * dy
          if (squared <= bestSquared) {
            bestSquared = squared
            best = { series, index, distance: Math.sqrt(squared) }
          }
        }
      }
    }
  }
  return best
}

export function pointsAtX(model: XyModel, x: number): XyHit[] {
  const hits: XyHit[] = []
  model.series.forEach(({ points }, series) => {
    let nearest = -1
    let gap = Infinity
    for (let index = 0; index < points.length; index++) {
      const distance = Math.abs(points[index][0] - x)
      if (distance < gap) {
        gap = distance
        nearest = index
      }
    }
    if (nearest >= 0) hits.push({ series, index: nearest, distance: gap })
  })
  return hits
}

export function tooltipHits(
  model: XyModel,
  hits: readonly XyHit[],
  options: Pick<XyOptions, "tooltipSort" | "tooltipHideZeros">
): XyHit[] {
  const candidates = hits.flatMap((hit, index) =>
    model.series[hit.series].hideInTooltip
      ? []
      : [{ index, value: model.series[hit.series].points[hit.index][1] }]
  )
  const { rows } = tooltipRows(
    candidates,
    {
      sort: options.tooltipSort,
      hideZeros: options.tooltipHideZeros,
      cumulative: false,
      stack: [],
    },
    Infinity
  )
  return rows.map(({ index }) => hits[index])
}

function tickStep(span: number, count: number): number {
  const raw = span / Math.max(1, count)
  const power = 10 ** Math.floor(Math.log10(raw))
  const fraction = raw / power
  const nice = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10
  return nice * power
}

function tickValues(
  range: XyRange,
  count: number,
  scale: ValueScale
): number[] {
  if (scale.kind === "log") {
    return logTicks(range.min, range.max, scale.logBase, count)
  }
  if (scale.kind === "symlog") {
    return symlogTicks(range.min, range.max, scale.linearThreshold, count)
  }
  const step = tickStep(range.max - range.min, count)
  const first = Math.ceil(range.min / step)
  const last = Math.floor(range.max / step)
  const values: number[] = []
  for (let i = first; i <= last; i++) {
    values.push(Number((i * step).toPrecision(12)))
  }
  return values
}

export function axisTicks(
  range: XyRange,
  config: FieldConfig | undefined,
  count: number,
  scale: ValueScale = linearScale
): XyTick[] {
  if (!(range.max - range.min > 0) || count < 1) return []
  const values = tickValues(range, count, scale)
  const decimals = axisDecimals(values, config?.unit, config?.decimals)
  return values.map((value) => ({
    value,
    text: formatValue(value, config?.unit, decimals),
  }))
}

function channelText(channel: XyChannel, value: number): string {
  return displayValue(value, channel.config).text
}

export function pointSummary(
  model: XyModel,
  hit: Pick<XyHit, "series" | "index">
): XyPointSummary {
  const { points, xConfig, yConfig, size, tone } = model.series[hit.series]
  const [x, y, , , sizeValue, toneValue] = points[hit.index]
  const extras: XyPointSummary["extras"] = []
  if (size && sizeValue !== null) {
    extras.push({ name: size.name, text: channelText(size, sizeValue) })
  }
  if (tone && toneValue !== null) {
    extras.push({ name: tone.name, text: channelText(tone, toneValue) })
  }
  return {
    x: formatValue(x, xConfig.unit, xConfig.decimals),
    y: formatValue(y, yConfig.unit, yConfig.decimals),
    extras,
  }
}
