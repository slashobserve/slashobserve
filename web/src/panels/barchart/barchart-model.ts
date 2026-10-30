import { formatDateTime } from "@/lib/time"
import { timeLabel } from "@/lib/time-ticks"
import { decimalsByUnit } from "@/lib/units"
import { displayWithDefaults, type Reducer } from "@/spec"

import { styleForSlot } from "../colors"
import {
  displayValue,
  fieldRange,
  type DisplayValue,
  type FieldConfig,
  type FieldResolver,
} from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { tooltipRows, type TooltipSort } from "../timeseries/tooltip-rows"
import { thresholdColor } from "../thresholds"
import {
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"
import {
  linearScale,
  scaleOf,
  scalePosition,
  valueAxisSettings,
  valueRange,
  type ValueAxisSettings,
  type ValueScale,
} from "../value-axis"

export type Orientation = "horizontal" | "vertical"
type Stack = FieldConfig["stack"]
type BarchartLegend = "bottom" | "right" | "hidden"
type BarchartTooltip = "single" | "all" | "hidden"
export type ShowValues = "auto" | "always" | "never"

type Category = { key: string; label: string; title: string }

type BarSeries = {
  key: string
  name: string
  color: string
  stack: Stack
  slot: number
  hideInTooltip: boolean
  fillOpacity: number
  borderWidth: number
}

export type Bar = {
  value: number | null
  shown: DisplayValue
  color: string
  start: number
  end: number
  roundEnd: boolean
}

type BarAxis = {
  unit?: string
  decimals?: number
  settings: ValueAxisSettings
  color: string | null
}

export type BarchartModel = {
  categories: Category[]
  series: BarSeries[]
  slotCount: number
  bars: Bar[][]
  min: number
  max: number
  scale: ValueScale
  axis: BarAxis
  orientation: Orientation
  reason: string | null
  note: string | null
}

export type BarOptions = {
  legend: BarchartLegend
  legendMode?: "list" | "table"
  legendValues: Reducer[]
  legendSortBy?: "name" | Reducer
  legendSortDesc: boolean
  legendWidth?: number
  tooltip: BarchartTooltip
  tooltipSort: TooltipSort
  tooltipHideZeros: boolean
  tooltipMaxWidth?: number
  tooltipMaxHeight?: number
  xTickRotation: number
  xTickSpacing: number
  xTickMaxLength?: number
  barRadius: number
  fullHighlight: boolean
  valueSize: number
}

export const maxCategories = 500
const defaultValueSize = 12
export const noCategoriesReason =
  "A bar chart needs a text or time field for its categories."
export const truncatedNote = `Showing the first ${maxCategories} categories.`

function numberOf(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined
}

export function barOptions(display: Record<string, unknown>): BarOptions {
  const merged = displayWithDefaults("barchart", display)
  const sortBy = merged.legendSortBy
  return {
    legend: legendOf(merged),
    legendMode:
      merged.legendMode === "list" || merged.legendMode === "table"
        ? merged.legendMode
        : undefined,
    legendValues: (merged.legendValues ?? []) as Reducer[],
    legendSortBy:
      typeof sortBy === "string" ? (sortBy as "name" | Reducer) : undefined,
    legendSortDesc: merged.legendSortDesc === true,
    legendWidth: numberOf(merged.legendWidth),
    tooltip: tooltipOf(merged),
    tooltipSort:
      merged.tooltipSort === "asc" || merged.tooltipSort === "desc"
        ? merged.tooltipSort
        : "none",
    tooltipHideZeros: merged.tooltipHideZeros === true,
    tooltipMaxWidth: numberOf(merged.tooltipMaxWidth),
    tooltipMaxHeight: numberOf(merged.tooltipMaxHeight),
    xTickRotation: numberOf(merged.xTickRotation) ?? 0,
    xTickSpacing: numberOf(merged.xTickSpacing) ?? 0,
    xTickMaxLength: numberOf(merged.xTickMaxLength),
    barRadius: numberOf(merged.barRadius) ?? 0,
    fullHighlight: merged.fullHighlight === true,
    valueSize: numberOf(merged.valueSize) ?? defaultValueSize,
  }
}

export function orientationOf(
  display: Record<string, unknown>,
  size: { width: number; height: number }
): Orientation {
  if (display.orientation === "horizontal") return "horizontal"
  if (display.orientation === "vertical") return "vertical"
  return size.height > size.width ? "horizontal" : "vertical"
}

function legendOf(display: Record<string, unknown>): BarchartLegend {
  if (display.legend === "right") return "right"
  if (display.legend === "hidden") return "hidden"
  return "bottom"
}

function tooltipOf(display: Record<string, unknown>): BarchartTooltip {
  if (display.tooltip === "all") return "all"
  if (display.tooltip === "hidden") return "hidden"
  return "single"
}

export function showValuesOf(display: Record<string, unknown>): ShowValues {
  if (display.showValues === "always") return "always"
  if (display.showValues === "never") return "never"
  return "auto"
}

export function shareOf(
  display: Record<string, unknown>,
  key: "barWidth" | "groupWidth",
  fallback: number
): number {
  const value = display[key]
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback
  return Math.min(1, Math.max(0, value))
}

function xFieldFor(
  frame: PreparedFrame,
  xField: string | undefined,
  kind: "string" | "time"
): PreparedField | undefined {
  if (xField !== undefined) {
    return frame.fields.find((field) => matchesName(frame, field, xField))
  }
  return frame.fields.find((field) => field.type === kind)
}

function categoryKindOf(
  frames: PreparedFrame[],
  xField: string | undefined
): "string" | "time" {
  if (xField !== undefined) return "string"
  const hasText = frames.some((frame) =>
    frame.fields.some((field) => field.type === "string")
  )
  return hasText ? "string" : "time"
}

function categoryOf(
  field: PreparedField,
  value: unknown,
  spanMs: number
): Category {
  const key = String(value)
  if (field.type === "time" && typeof value === "number") {
    return {
      key,
      label: timeLabel(value, spanMs),
      title: formatDateTime(value),
    }
  }
  return { key, label: key, title: key }
}

function spanOf(field: PreparedField): number {
  if (field.type !== "time") return 0
  let min = Infinity
  let max = -Infinity
  for (const value of field.values) {
    if (typeof value !== "number") continue
    if (value < min) min = value
    if (value > max) max = value
  }
  return min <= max ? max - min : 0
}

function finite(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

type Column = {
  series: Omit<BarSeries, "slot">
  config: FieldConfig
  field: PreparedField
  byCategory: Map<number, number | null>
  colors: Map<number, string> | null
  hideInLegend: boolean
}

export type BarSource = {
  categories: Category[]
  columns: Column[]
  reason: string | null
  note: string | null
}

type Cell = { column: Column; category: number; value: number | null }

function colorsFrom(
  frame: PreparedFrame,
  colorByField: string | undefined,
  fields: FieldResolver,
  rows: ReadonlyMap<number, number>
): Map<number, string> | null {
  if (colorByField === undefined) return null
  const source = frame.fields.find(
    (field) =>
      field.type === "number" && matchesName(frame, field, colorByField)
  )
  if (!source) return null
  const config = fields.configFor(frame, source)
  const range = fieldRange(source)
  const colors = new Map<number, string>()
  for (const [row, category] of rows) {
    const color = displayValue(finite(source.values[row]), config, {
      fieldMin: range?.min,
      fieldMax: range?.max,
    }).color
    if (color) colors.set(category, color)
  }
  return colors
}

export function barSource(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): BarSource {
  const xField = typeof display.xField === "string" ? display.xField : undefined
  const colorByField =
    typeof display.colorByField === "string" ? display.colorByField : undefined
  const kind = categoryKindOf(frames, xField)
  const categories: Category[] = []
  const indexByKey = new Map<string, number>()
  const columns: Column[] = []
  const seen = new Map<string, number>()
  let truncated = false
  let usable = false

  for (const frame of frames) {
    const x = xFieldFor(frame, xField, kind)
    if (!x) continue
    usable = true
    const rows = new Map<number, number>()
    const spanMs = spanOf(x)
    x.values.forEach((value, row) => {
      if (value === null || value === undefined) return
      const key = String(value)
      let index = indexByKey.get(key)
      if (index === undefined) {
        if (categories.length >= maxCategories) {
          truncated = true
          return
        }
        index = categories.length
        indexByKey.set(key, index)
        categories.push(categoryOf(x, value, spanMs))
      }
      rows.set(row, index)
    })
    const colors = colorsFrom(frame, colorByField, fields, rows)
    for (const field of frame.fields) {
      if (field.type !== "number" || field === x) continue
      const config = fields.configFor(frame, field)
      if (config.hidden) continue
      const byCategory = new Map<number, number | null>()
      for (const [row, index] of rows) {
        byCategory.set(index, finite(field.values[row]))
      }
      columns.push({
        series: {
          key: uniqueKey(seen, seriesKey(frame, field.name)),
          name: fields.displayName(frame, field),
          color: config.color
            ? thresholdColor(config.color)
            : styleForSlot(columns.length).color,
          stack: config.stack,
          hideInTooltip: config.hideInTooltip,
          fillOpacity: fields.isSet(frame, field, "fill")
            ? config.fill / 100
            : 1,
          borderWidth: fields.isSet(frame, field, "lineWidth")
            ? config.lineWidth
            : 0,
        },
        config,
        field,
        byCategory,
        colors,
        hideInLegend: config.hideInLegend,
      })
    }
  }

  return {
    categories: usable ? categories : [],
    columns: usable ? columns : [],
    reason: usable ? null : noCategoriesReason,
    note: truncated ? truncatedNote : null,
  }
}

export function legendEntries(source: BarSource) {
  return source.columns.map((column) => ({
    key: column.series.key,
    label: column.series.name,
    color: column.series.color,
    values: [...column.byCategory.values()],
    config: column.config,
    hideInLegend: column.hideInLegend,
  }))
}

function slotsFor(columns: Column[]): Map<string, number> {
  const slots = new Map<string, number>()
  for (const { series } of columns) {
    const group = series.stack === "none" ? series.key : `stack:${series.stack}`
    if (!slots.has(group)) slots.set(group, slots.size)
  }
  return slots
}

function stackedBars(cells: Cell[][], shown: Map<Cell, DisplayValue>): Bar[][] {
  return cells.map((row) => {
    const totals = new Map<Stack, number>()
    for (const { column, value } of row) {
      const mode = column.series.stack
      if (mode === "percent" && value !== null) {
        totals.set(mode, (totals.get(mode) ?? 0) + Math.abs(value))
      }
    }
    const positive = new Map<Stack, number>()
    const negative = new Map<Stack, number>()
    const outermost = new Map<string, Bar>()
    const bars = row.map((cell): Bar => {
      const { column, value } = cell
      const display = shown.get(cell) as DisplayValue
      const mode = column.series.stack
      const bar: Bar = {
        value,
        shown: display,
        color:
          column.colors?.get(cell.category) ??
          display.color ??
          column.series.color,
        start: 0,
        end: 0,
        roundEnd: mode === "none",
      }
      if (value === null) return bar
      if (mode === "none") {
        bar.end = value
        return bar
      }
      const total = totals.get(mode) ?? 0
      const scaled =
        mode === "percent" ? (total === 0 ? 0 : (value / total) * 100) : value
      const edges = scaled < 0 ? negative : positive
      bar.start = edges.get(mode) ?? 0
      bar.end = bar.start + scaled
      edges.set(mode, bar.end)
      outermost.set(`${mode}:${scaled < 0 ? "down" : "up"}`, bar)
      return bar
    })
    for (const bar of outermost.values()) bar.roundEnd = true
    return bars
  })
}

function rangeOf(
  columns: Column[],
  bars: Bar[][]
): { range: [number, number]; positiveMin: number } {
  let min = 0
  let max = 0
  let positiveMin = Infinity
  for (const row of bars) {
    for (const bar of row) {
      if (bar.value === null) continue
      min = Math.min(min, bar.start, bar.end)
      max = Math.max(max, bar.start, bar.end)
      for (const edge of [bar.start, bar.end]) {
        if (edge > 0) positiveMin = Math.min(positiveMin, edge)
      }
    }
  }
  for (const { config, series } of columns) {
    if (series.stack === "percent") continue
    if (config.min !== undefined) min = Math.min(min, config.min)
    if (config.max !== undefined) max = Math.max(max, config.max)
  }
  return { range: [min, max], positiveMin }
}

const defaultAxisConfig = {
  axis: "left",
  axisBorder: false,
  axisColor: "text",
  scale: "linear",
  logBase: 10,
  axisCenteredZero: false,
} as const

function axisOf(columns: Column[]): BarAxis {
  const [first] = columns
  if (!first) {
    return { settings: valueAxisSettings(defaultAxisConfig), color: null }
  }
  const settings = valueAxisSettings(first.config)
  const percent = columns.every(({ series }) => series.stack === "percent")
  return {
    unit: percent ? "percent" : first.config.unit,
    decimals: percent ? 0 : first.config.decimals,
    settings: percent
      ? {
          ...settings,
          scale: "linear",
          softMin: undefined,
          softMax: undefined,
          centeredZero: false,
        }
      : settings,
    color: settings.seriesColored ? first.series.color : null,
  }
}

const noneHidden: ReadonlySet<string> = new Set()

export function barchartModel(
  source: BarSource,
  display: Record<string, unknown>,
  size: { width: number; height: number },
  hidden: ReadonlySet<string> = noneHidden
): BarchartModel {
  const orientation = orientationOf(display, size)
  if (source.reason !== null) {
    return {
      categories: [],
      series: [],
      slotCount: 0,
      bars: [],
      min: 0,
      max: 1,
      scale: linearScale,
      axis: axisOf([]),
      orientation,
      reason: source.reason,
      note: null,
    }
  }

  const { categories } = source
  const columns = source.columns.filter(({ series }) => !hidden.has(series.key))
  const cells: Cell[][] = categories.map((_, category) =>
    columns.map((column) => ({
      column,
      category,
      value: column.byCategory.get(category) ?? null,
    }))
  )
  const decimals = decimalsByUnit(
    cells.flat().map(({ column, value }) => ({
      unit: column.config.unit,
      decimals: column.config.decimals,
      value,
    }))
  )
  const ranges = new Map(
    columns.map((column) => [column, fieldRange(column.field)])
  )
  const shown = new Map<Cell, DisplayValue>()
  for (const cell of cells.flat()) {
    const { config } = cell.column
    const range = ranges.get(cell.column)
    shown.set(
      cell,
      displayValue(cell.value, config, {
        decimals: decimals.get(config.unit),
        fieldMin: range?.min,
        fieldMax: range?.max,
      })
    )
  }

  const slots = slotsFor(columns)
  const bars = stackedBars(cells, shown)
  const axis = axisOf(columns)
  const { range, positiveMin } = rangeOf(columns, bars)
  const [min, high] = valueRange(range, positiveMin, axis.settings)
  return {
    categories,
    series: columns.map(({ series }) => ({
      ...series,
      slot: slots.get(
        series.stack === "none" ? series.key : `stack:${series.stack}`
      ) as number,
    })),
    slotCount: slots.size,
    bars,
    min,
    max: high === min ? min + 1 : high,
    scale: scaleOf(axis.settings),
    axis,
    orientation,
    reason: null,
    note: source.note,
  }
}

export type Hit = { category: number; bar: number | null }

export function tooltipIndexes(
  model: Pick<BarchartModel, "series" | "bars">,
  hit: Hit,
  options: Pick<BarOptions, "tooltip" | "tooltipSort" | "tooltipHideZeros">
): number[] {
  const row = model.bars[hit.category]
  const candidates =
    options.tooltip === "single"
      ? hit.bar === null
        ? []
        : [hit.bar]
      : row.flatMap((bar, index) => (bar.value === null ? [] : [index]))
  const listed = candidates
    .filter((index) => !model.series[index].hideInTooltip)
    .map((index) => ({ index, value: row[index].value as number }))
  return tooltipRows(
    listed,
    {
      sort: options.tooltipSort,
      hideZeros: options.tooltipHideZeros,
      cumulative: false,
      stack: [],
    },
    Infinity
  ).rows.map(({ index }) => index)
}

const tickBases = [1, 2, 5, 10]

export function axisTicks(
  min: number,
  max: number,
  maxTicks: number
): { min: number; max: number; ticks: number[] } {
  const intervals = Math.max(1, Math.floor(maxTicks))
  const raw = (max - min) / intervals
  const magnitude = 10 ** Math.floor(Math.log10(raw))
  const base = tickBases.find((candidate) => candidate * magnitude >= raw)
  const step = (base ?? 10) * magnitude
  const low = Math.floor(min / step)
  const high = Math.ceil(max / step)
  const ticks = Array.from({ length: high - low + 1 }, (_, index) =>
    Number(((low + index) * step).toPrecision(12))
  )
  return { min: ticks[0], max: ticks[ticks.length - 1], ticks }
}

export function labelStride(
  count: number,
  length: number,
  labelLength: number
): number {
  if (count < 1 || length <= 0) return 1
  return Math.max(1, Math.ceil(labelLength / (length / count)))
}

export function labelStep(
  count: number,
  length: number,
  autoLabelLength: number,
  spacing: number
): number {
  return labelStride(count, length, spacing > 0 ? spacing : autoLabelLength)
}

export function truncateLabel(label: string, maxLength?: number): string {
  if (maxLength === undefined) return label
  const chars = Array.from(label)
  return chars.length > maxLength
    ? `${chars.slice(0, maxLength).join("")}…`
    : label
}

export function rotatedExtent(
  textLength: number,
  rotation: number,
  lineHeight: number
): number {
  const radians = (Math.abs(rotation) * Math.PI) / 180
  return Math.ceil(
    textLength * Math.sin(radians) + lineHeight * Math.cos(radians)
  )
}

export type Plot = { width: number; height: number }
export type Rect = { x: number; y: number; width: number; height: number }

export type Layout = {
  orientation: Orientation
  plot: Plot
  min: number
  max: number
  scale: ValueScale
  count: number
  slotCount: number
  groupWidth: number
  barWidth: number
}

function along(layout: Layout, value: number): number {
  const length =
    layout.orientation === "vertical" ? layout.plot.height : layout.plot.width
  const share = scalePosition(value, layout.min, layout.max, layout.scale)
  return layout.orientation === "vertical"
    ? length - share * length
    : share * length
}

export function barRect(
  layout: Layout,
  category: number,
  slot: number,
  bar: Pick<Bar, "start" | "end">
): Rect {
  const vertical = layout.orientation === "vertical"
  const across = vertical ? layout.plot.width : layout.plot.height
  const categoryLength = across / layout.count
  const groupLength = categoryLength * layout.groupWidth
  const slotLength = groupLength / layout.slotCount
  const barLength = slotLength * layout.barWidth
  const offset =
    category * categoryLength +
    (categoryLength - groupLength) / 2 +
    slot * slotLength +
    (slotLength - barLength) / 2
  const first = along(layout, bar.start)
  const second = along(layout, bar.end)
  const low = Math.min(first, second)
  const extent = Math.abs(second - first)
  return vertical
    ? { x: offset, y: low, width: barLength, height: extent }
    : { x: low, y: offset, width: extent, height: barLength }
}

export function categoryBand(layout: Layout, category: number): Rect {
  const vertical = layout.orientation === "vertical"
  const across = vertical ? layout.plot.width : layout.plot.height
  const length = across / layout.count
  return vertical
    ? { x: category * length, y: 0, width: length, height: layout.plot.height }
    : { x: 0, y: category * length, width: layout.plot.width, height: length }
}

export function roundedBarPath(
  rect: Rect,
  orientation: Orientation,
  negative: boolean,
  radiusShare: number
): string | null {
  const vertical = orientation === "vertical"
  const thickness = vertical ? rect.width : rect.height
  const length = vertical ? rect.height : rect.width
  const r = Math.min(radiusShare * thickness, thickness / 2, length)
  if (!(r > 0)) return null
  const { x, y, width: w, height: h } = rect
  const arc = (dx: number, dy: number) => `A${r} ${r} 0 0 1 ${x + dx} ${y + dy}`
  if (vertical && !negative) {
    return `M${x} ${y + h}L${x} ${y + r}${arc(r, 0)}L${x + w - r} ${y}${arc(w, r)}L${x + w} ${y + h}Z`
  }
  if (vertical) {
    return `M${x} ${y}L${x + w} ${y}L${x + w} ${y + h - r}${arc(w - r, h)}L${x + r} ${y + h}${arc(0, h - r)}Z`
  }
  if (!negative) {
    return `M${x} ${y}L${x + w - r} ${y}${arc(w, r)}L${x + w} ${y + h - r}${arc(w - r, h)}L${x} ${y + h}Z`
  }
  return `M${x + w} ${y}L${x + r} ${y}${arc(0, r)}L${x} ${y + h - r}${arc(r, h)}L${x + w} ${y + h}Z`
}

export const labelHeight = 16
const minLabeledThickness = 16
const labelPadding = 4

export type LabelSpot = "inside" | "outside" | null

export function labelSpot(
  mode: ShowValues,
  rect: Rect,
  orientation: Orientation,
  plot: Plot,
  textWidth: number,
  options: { stacked: boolean; negative: boolean; lineHeight?: number }
): LabelSpot {
  if (mode === "never") return null
  const vertical = orientation === "vertical"
  const thickness = vertical ? rect.width : rect.height
  const length = vertical ? rect.height : rect.width
  const lineHeight = options.lineHeight ?? labelHeight
  const textThickness = vertical ? textWidth : lineHeight
  const textLength = vertical ? lineHeight : textWidth
  if (
    mode === "auto" &&
    (thickness < minLabeledThickness || thickness < textThickness)
  ) {
    return null
  }
  if (length >= textLength + labelPadding) return "inside"
  const end = vertical
    ? options.negative
      ? plot.height - (rect.y + rect.height)
      : rect.y
    : options.negative
      ? rect.x
      : plot.width - (rect.x + rect.width)
  if (!options.stacked && end >= textLength + labelPadding) return "outside"
  return mode === "always" ? "inside" : null
}

export function lineHeightFor(fontSize: number): number {
  return Math.max(labelHeight, Math.ceil(fontSize * 1.34))
}

export function textWidthFor(text: string, fontSize: number): number {
  return Math.ceil((text.length * fontSize * 7) / defaultValueSize)
}

export function hitTest(
  layout: Layout,
  model: Pick<BarchartModel, "series" | "bars">,
  x: number,
  y: number
): Hit {
  const vertical = layout.orientation === "vertical"
  const across = vertical ? layout.plot.width : layout.plot.height
  const slotLength = across / layout.count
  const category = Math.min(
    layout.count - 1,
    Math.max(0, Math.floor((vertical ? x : y) / slotLength))
  )
  let hit: number | null = null
  for (const [index, bar] of model.bars[category].entries()) {
    if (bar.value === null) continue
    const rect = barRect(layout, category, model.series[index].slot, bar)
    if (
      x >= rect.x &&
      x <= rect.x + rect.width &&
      y >= rect.y &&
      y <= rect.y + rect.height
    ) {
      hit = index
    }
  }
  return { category, bar: hit }
}
