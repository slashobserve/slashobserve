import { formatDateTime } from "@/lib/time"
import { timeLabel } from "@/lib/time-ticks"
import { decimalsByUnit } from "@/lib/units"

import { styleForSlot } from "../colors"
import {
  displayValue,
  fieldRange,
  type DisplayValue,
  type FieldConfig,
  type FieldResolver,
} from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { thresholdColor } from "../thresholds"
import {
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"

export type Orientation = "horizontal" | "vertical"
export type Stack = FieldConfig["stack"]
export type BarchartLegend = "bottom" | "right" | "hidden"
export type BarchartTooltip = "single" | "all" | "hidden"
export type ShowValues = "auto" | "always" | "never"

export type Category = { key: string; label: string; title: string }

export type BarSeries = {
  key: string
  name: string
  color: string
  stack: Stack
  slot: number
}

export type Bar = {
  value: number | null
  shown: DisplayValue
  color: string
  start: number
  end: number
}

export type BarchartModel = {
  categories: Category[]
  series: BarSeries[]
  slotCount: number
  bars: Bar[][]
  min: number
  max: number
  axis: { unit?: string; decimals?: number }
  orientation: Orientation
  reason: string | null
  note: string | null
}

export const maxCategories = 500
export const noCategoriesReason =
  "A bar chart needs a text or time field for its categories."
export const truncatedNote = `Showing the first ${maxCategories} categories.`

export function orientationOf(
  display: Record<string, unknown>,
  size: { width: number; height: number }
): Orientation {
  if (display.orientation === "horizontal") return "horizontal"
  if (display.orientation === "vertical") return "vertical"
  return size.height > size.width ? "horizontal" : "vertical"
}

export function legendOf(display: Record<string, unknown>): BarchartLegend {
  if (display.legend === "right") return "right"
  if (display.legend === "hidden") return "hidden"
  return "bottom"
}

export function tooltipOf(display: Record<string, unknown>): BarchartTooltip {
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
  const times = field.values.filter(
    (value): value is number => typeof value === "number"
  )
  return times.length === 0 ? 0 : Math.max(...times) - Math.min(...times)
}

function finite(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

type Column = {
  series: Omit<BarSeries, "slot">
  config: FieldConfig
  field: PreparedField
  byCategory: Map<number, number | null>
}

type Cell = { column: Column; category: number; value: number | null }

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
    return row.map((cell) => {
      const { column, value } = cell
      const display = shown.get(cell) as DisplayValue
      const color = display.color ?? column.series.color
      const mode = column.series.stack
      if (value === null)
        return { value, shown: display, color, start: 0, end: 0 }
      if (mode === "none") {
        return { value, shown: display, color, start: 0, end: value }
      }
      const total = totals.get(mode) ?? 0
      const scaled =
        mode === "percent" ? (total === 0 ? 0 : (value / total) * 100) : value
      const edges = scaled < 0 ? negative : positive
      const start = edges.get(mode) ?? 0
      edges.set(mode, start + scaled)
      return { value, shown: display, color, start, end: start + scaled }
    })
  })
}

function rangeOf(columns: Column[], bars: Bar[][]): [number, number] {
  let min = 0
  let max = 0
  for (const row of bars) {
    for (const bar of row) {
      min = Math.min(min, bar.start, bar.end)
      max = Math.max(max, bar.start, bar.end)
    }
  }
  for (const { config, series } of columns) {
    if (series.stack === "percent") continue
    if (config.min !== undefined) min = Math.min(min, config.min)
    if (config.max !== undefined) max = Math.max(max, config.max)
  }
  return max === min ? [min, min + 1] : [min, max]
}

function axisOf(columns: Column[]): BarchartModel["axis"] {
  if (columns.length === 0) return {}
  if (columns.every(({ series }) => series.stack === "percent")) {
    return { unit: "percent", decimals: 0 }
  }
  const { config } = columns[0]
  return { unit: config.unit, decimals: config.decimals }
}

export function barchartModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  size: { width: number; height: number }
): BarchartModel {
  const orientation = orientationOf(display, size)
  const xField = typeof display.xField === "string" ? display.xField : undefined
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
        },
        config,
        field,
        byCategory,
      })
    }
  }

  if (!usable) {
    return {
      categories: [],
      series: [],
      slotCount: 0,
      bars: [],
      min: 0,
      max: 1,
      axis: {},
      orientation,
      reason: noCategoriesReason,
      note: null,
    }
  }

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
  const [min, max] = rangeOf(columns, bars)
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
    max,
    axis: axisOf(columns),
    orientation,
    reason: null,
    note: truncated ? truncatedNote : null,
  }
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

export type Plot = { width: number; height: number }
export type Rect = { x: number; y: number; width: number; height: number }

export type Layout = {
  orientation: Orientation
  plot: Plot
  min: number
  max: number
  count: number
  slotCount: number
  groupWidth: number
  barWidth: number
}

function along(layout: Layout, value: number): number {
  const length =
    layout.orientation === "vertical" ? layout.plot.height : layout.plot.width
  const share = (value - layout.min) / (layout.max - layout.min)
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

export const labelHeight = 16
export const minLabeledThickness = 16
const labelPadding = 4

export type LabelSpot = "inside" | "outside" | null

export function labelSpot(
  mode: ShowValues,
  rect: Rect,
  orientation: Orientation,
  plot: Plot,
  textWidth: number,
  options: { stacked: boolean; negative: boolean }
): LabelSpot {
  if (mode === "never") return null
  const vertical = orientation === "vertical"
  const thickness = vertical ? rect.width : rect.height
  const length = vertical ? rect.height : rect.width
  const textThickness = vertical ? textWidth : labelHeight
  const textLength = vertical ? labelHeight : textWidth
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

export type Hit = { category: number; bar: number | null }

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
