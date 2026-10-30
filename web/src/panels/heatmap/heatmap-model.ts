import { formatDateTime } from "@/lib/time"
import {
  axisDecimals,
  commonDecimals,
  formatValue,
  ownDecimals,
} from "@/lib/units"
import type { DataPanel } from "@/spec"

import type { FieldConfig, FieldResolver } from "../field-config"
import { rangePosition, schemeColor, type ColorScheme } from "../color-scheme"
import { isNumber } from "../reducers"
import { thresholdColor } from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"

type HeatmapMode = "buckets" | "ordinal" | "calculated"

type HeatmapRow = {
  label: string
  range: string
  from?: number
  to?: number
}

export type HeatmapModel = {
  mode: HeatmapMode
  xs: number[]
  rows: HeatmapRow[]
  cells: (number | null)[][]
  min: number
  max: number
  unit: string | undefined
  reversed: boolean
}

export const calculatedRows = 16
const legendSteps = 5
const minOpacity = 0.1
const defaultExponent = 0.5
const labelFontSize = 11
const labelPadding = 2
const maxColumns = 1000
const maxRows = 1000
const niceSteps = [1, 2, 2.5, 5, 10]
const defaultLogBase = 2

type BucketMode = "size" | "count"
type BucketSpec = { mode: BucketMode; value: number | null }
type BucketScale = "linear" | "log" | "symlog"
type RowsLayout = "auto" | "le" | "ge" | "unknown"

function bucketSpec(raw: unknown): BucketSpec | null {
  if (typeof raw !== "object" || raw === null) return null
  const { mode, value } = raw as { mode?: unknown; value?: unknown }
  if (mode !== "size" && mode !== "count") return null
  return {
    mode,
    value:
      typeof value === "number" && Number.isFinite(value) && value > 0
        ? value
        : null,
  }
}

function niceStep(raw: number): number {
  if (!(raw > 0) || !Number.isFinite(raw)) return 1
  const magnitude = 10 ** Math.floor(Math.log10(raw))
  const unit = niceSteps.find((step) => step * magnitude >= raw * 0.999999)
  return (unit ?? 10) * magnitude
}

function sampleStep(xs: number[]): number {
  let step = Infinity
  for (let i = 1; i < xs.length; i++) step = Math.min(step, xs[i] - xs[i - 1])
  return Number.isFinite(step) && step > 0 ? step : 1000
}

type Columns = { xs: number[]; column: number[] }

function groupColumns(xs: number[], spec: BucketSpec | null): Columns {
  const own: Columns = { xs, column: xs.map((_, i) => i) }
  if (!spec || xs.length === 0) return own
  if (spec.mode === "count" && spec.value === null) return own
  const first = xs[0]
  const last = xs[xs.length - 1]
  const span = last - first
  let width: number
  let origin = first
  let count: number
  if (spec.mode === "count") {
    count = Math.min(maxColumns, Math.max(1, Math.round(spec.value ?? 1)))
    width = span / count
    if (!(width > 0)) return { xs: [first], column: xs.map(() => 0) }
  } else {
    width = Math.max(
      spec.value === null ? sampleStep(xs) : spec.value * 1000,
      span / maxColumns
    )
    origin = Math.floor(first / width) * width
    count = Math.floor((last - origin) / width) + 1
  }
  const column = xs.map((ms) =>
    Math.min(count - 1, Math.max(0, Math.floor((ms - origin) / width)))
  )
  return {
    xs: Array.from({ length: count }, (_, i) => origin + i * width),
    column,
  }
}

const scales: Record<
  BucketScale,
  (base: number) => { to: (v: number) => number; from: (t: number) => number }
> = {
  linear: () => ({ to: (v) => v, from: (t) => t }),
  log: (base) => {
    const ln = Math.log(base)
    return { to: (v) => Math.log(v) / ln, from: (t) => base ** t }
  },
  symlog: (base) => {
    const ln = Math.log(base)
    return {
      to: (v) => Math.sign(v) * (Math.log1p(Math.abs(v)) / ln),
      from: (t) => Math.sign(t) * (base ** Math.abs(t) - 1),
    }
  },
}

type RowGrid = { edges: number[]; indexOf: (value: number) => number }

function rowGrid(
  low: number,
  high: number,
  spec: BucketSpec | null,
  scale: BucketScale,
  base: number
): RowGrid {
  const { to, from } =
    scales[scale === "log" && low <= 0 ? "linear" : scale](base)
  const start = to(low)
  const span = to(high) - start
  if (spec?.mode !== "size") {
    const rows = Math.min(
      maxRows,
      Math.max(1, Math.round(spec?.value ?? calculatedRows))
    )
    const edges = Array.from({ length: rows + 1 }, (_, i) =>
      i === rows ? high : from(start + (span * i) / rows)
    )
    return {
      edges,
      indexOf: (value) =>
        Math.min(
          rows - 1,
          Math.floor((span === 0 ? 0 : (to(value) - start) / span) * rows)
        ),
    }
  }
  const step = Math.max(
    spec.value ?? niceStep(span / calculatedRows),
    span / maxRows
  )
  const origin = Math.floor(start / step) * step
  const rows = Math.max(1, Math.ceil((to(high) - origin) / step - 1e-9))
  return {
    edges: Array.from({ length: rows + 1 }, (_, i) => from(origin + i * step)),
    indexOf: (value) =>
      Math.min(rows - 1, Math.max(0, Math.floor((to(value) - origin) / step))),
  }
}

type Entry = {
  frame: PreparedFrame
  field: PreparedField
  config: FieldConfig
  values: (number | null)[]
}

function parseBound(text: string | undefined): number | null {
  const trimmed = text?.trim()
  if (!trimmed) return null
  if (/^\+?inf$/i.test(trimmed)) return Infinity
  const bound = Number(trimmed)
  return Number.isFinite(bound) ? bound : null
}

function entriesOf(
  frames: PreparedFrame[],
  fields: FieldResolver
): { xs: number[]; entries: Entry[] } {
  const usable = frames.flatMap((frame) => {
    const time = frame.fields.find((field) => field.type === "time")
    return time ? [{ frame, time }] : []
  })
  const stamps = new Set<number>()
  for (const { time } of usable) {
    for (const value of time.values) if (isNumber(value)) stamps.add(value)
  }
  const xs = [...stamps].sort((a, b) => a - b)
  const index = new Map(xs.map((ms, position) => [ms, position]))
  const entries = usable.flatMap(({ frame, time }) =>
    frame.fields.flatMap((field): Entry[] => {
      if (field.type !== "number") return []
      const config = fields.configFor(frame, field)
      if (config.hidden) return []
      const values: (number | null)[] = new Array(xs.length).fill(null)
      time.values.forEach((ms, row) => {
        const position = isNumber(ms) ? index.get(ms) : undefined
        const value = field.values[row]
        if (position !== undefined && isNumber(value)) values[position] = value
      })
      return [{ frame, field, config, values }]
    })
  )
  return { xs, entries }
}

function sumInto(
  total: (number | null)[],
  values: (number | null)[]
): (number | null)[] {
  return total.map((value, i) => {
    const other = values[i]
    if (other === null) return value
    return value === null ? other : value + other
  })
}

function deaccumulate(columns: (number | null)[][]): (number | null)[][] {
  return columns.map((column, row) =>
    column.map((value, col) => {
      if (value === null) return null
      const lower = row > 0 ? (columns[row - 1][col] ?? 0) : 0
      return Math.max(0, value - lower)
    })
  )
}

type Labelling = { unit: string | undefined; decimals: number | undefined }

function boundFormatter(
  bounds: number[],
  labelling: Labelling
): (bound: number) => string {
  const decimals = axisDecimals(
    bounds.filter(Number.isFinite),
    labelling.unit,
    labelling.decimals
  )
  return (bound) => formatValue(bound, labelling.unit, decimals)
}

function exactFormatter(labelling: Labelling): (bound: number) => string {
  return (bound) =>
    formatValue(
      bound,
      labelling.unit,
      labelling.decimals ?? ownDecimals(bound, labelling.unit)
    )
}

function deaccumulateFromAbove(
  columns: (number | null)[][]
): (number | null)[][] {
  return columns.map((column, row) =>
    column.map((value, col) => {
      if (value === null) return null
      const upper = row < columns.length - 1 ? (columns[row + 1][col] ?? 0) : 0
      return Math.max(0, value - upper)
    })
  )
}

type Shape = Pick<HeatmapModel, "rows" | "cells">

function bucketModel(
  entries: Entry[],
  xs: number[],
  labelling: Labelling,
  cumulative: boolean,
  layout: "le" | "ge",
  boundOf: (entry: Entry) => number
): Shape {
  const byBound = new Map<number, (number | null)[]>()
  for (const entry of entries) {
    const bound = boundOf(entry)
    byBound.set(
      bound,
      sumInto(
        byBound.get(bound) ?? new Array(xs.length).fill(null),
        entry.values
      )
    )
  }
  const bounds = [...byBound.keys()].sort((a, b) => a - b)
  const raw = bounds.map((bound) => byBound.get(bound) ?? [])
  const format = exactFormatter(labelling)
  if (layout === "ge") {
    const rows = bounds.map((from, i): HeatmapRow => {
      const label = format(from)
      const to = i < bounds.length - 1 ? bounds[i + 1] : undefined
      return {
        label,
        from,
        to,
        range:
          to === undefined ? `${label} and above` : `${label} - ${format(to)}`,
      }
    })
    return { rows, cells: cumulative ? deaccumulateFromAbove(raw) : raw }
  }
  const rows = bounds.map((to, i): HeatmapRow => {
    const from = i > 0 ? bounds[i - 1] : to > 0 ? 0 : undefined
    const label = format(to)
    return {
      label,
      from,
      to,
      range:
        from === undefined ? `up to ${label}` : `${format(from)} - ${label}`,
    }
  })
  return { rows, cells: cumulative ? deaccumulate(raw) : raw }
}

type Calculation = {
  x: BucketSpec | null
  y: BucketSpec | null
  scale: BucketScale
  base: number
}

function calculatedModel(
  entries: Entry[],
  xs: number[],
  labelling: Labelling,
  calculation: Calculation
): Shape & { xs: number[] } {
  const numbers = entries.flatMap(({ values }) => values.filter(isNumber))
  if (numbers.length === 0) return { rows: [], cells: [], xs }
  let low = Infinity
  let high = -Infinity
  for (const value of numbers) {
    if (value < low) low = value
    if (value > high) high = value
  }
  const grid = rowGrid(
    low,
    high,
    calculation.y,
    calculation.scale,
    calculation.base
  )
  const columns = groupColumns(xs, calculation.x)
  const format = boundFormatter(grid.edges, labelling)
  const rows = grid.edges.slice(0, -1).map((from, i): HeatmapRow => {
    const label = format(from)
    return {
      label,
      from,
      to: grid.edges[i + 1],
      range: `${label} - ${format(grid.edges[i + 1])}`,
    }
  })
  const cells = rows.map(() =>
    new Array<number | null>(columns.xs.length).fill(0)
  )
  for (const { values } of entries) {
    values.forEach((value, col) => {
      if (value === null) return
      const row = grid.indexOf(value)
      const target = columns.column[col]
      cells[row][target] = (cells[row][target] ?? 0) + 1
    })
  }
  return { rows, cells, xs: columns.xs }
}

function ordinalModel(entries: Entry[], fields: FieldResolver): Shape {
  const labels = entries.map(({ frame, field }) =>
    fields.displayName(frame, field)
  )
  return {
    rows: labels.map((label) => ({ label, range: label })),
    cells: entries.map(({ values }) => values),
  }
}

function inWindow(
  row: HeatmapRow,
  low: number | undefined,
  high: number | undefined
): boolean {
  if (low !== undefined && row.to !== undefined && row.to <= low) return false
  return !(high !== undefined && row.from !== undefined && row.from >= high)
}

function arranged(
  shape: Shape,
  window: { low: number | undefined; high: number | undefined },
  reversed: boolean
): Shape {
  const kept = shape.rows.flatMap((row, index) =>
    inWindow(row, window.low, window.high) ? [index] : []
  )
  const order = reversed ? kept.reverse() : kept
  return {
    rows: order.map((index) => shape.rows[index]),
    cells: order.map((index) => shape.cells[index]),
  }
}

function extent(
  cells: (number | null)[][],
  shown: (value: number) => boolean
): { min: number; max: number } {
  let min = Infinity
  let max = 0
  for (const row of cells) {
    for (const value of row) {
      if (value === null || !shown(value)) continue
      if (value < min) min = value
      if (value > max) max = value
    }
  }
  return { min: min === Infinity ? 0 : min, max }
}

export type ShowValues = "auto" | "always" | "never"

export type HeatmapLook = {
  mode: "scheme" | "opacity"
  scheme: ColorScheme
  steps: number | null
  exponential: boolean
  exponent: number
  fill: string
  reverse: boolean
  low: number
  high: number
  labelLow: number
  labelHigh: number
  gap: number
  radius: number
  hideBelow: number
  hideAbove: number | null
  showValues: ShowValues
  legend: boolean
  unit: string | undefined
  decimals: number | undefined
}

function numberOf(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined
}

export function visibleCell(
  look: Pick<HeatmapLook, "hideBelow" | "hideAbove">,
  value: number | null
): value is number {
  if (value === null || value <= look.hideBelow) return false
  return look.hideAbove === null || value < look.hideAbove
}

function shownCell(display: Record<string, unknown>) {
  const hideBelow = numberOf(display.hideBelow) ?? 0
  const hideAbove = numberOf(display.hideAbove) ?? null
  return (value: number) => visibleCell({ hideBelow, hideAbove }, value)
}

export function heatmapLook(
  display: Record<string, unknown>,
  model: Pick<HeatmapModel, "min" | "max">
): HeatmapLook {
  const opacity = display.colorMode !== "scheme"
  const min = numberOf(display.colorMin)
  const max = numberOf(display.colorMax)
  const high = max ?? model.max
  return {
    mode: opacity ? "opacity" : "scheme",
    scheme: (display.colorScheme as ColorScheme | undefined) ?? "blues",
    steps: numberOf(display.colorSteps) ?? null,
    exponential: display.colorScale === "exponential",
    exponent: numberOf(display.colorExponent) ?? defaultExponent,
    fill:
      typeof display.colorFill === "string"
        ? thresholdColor(display.colorFill)
        : "var(--series-1)",
    reverse: display.colorReverse === true,
    low: min ?? (opacity ? 0 : model.min),
    high,
    labelLow: min ?? model.min,
    labelHigh: high,
    gap: numberOf(display.cellGap) ?? 0,
    radius: numberOf(display.cellRadius) ?? 0,
    hideBelow: numberOf(display.hideBelow) ?? 0,
    hideAbove: numberOf(display.hideAbove) ?? null,
    showValues:
      display.showValues === "auto" || display.showValues === "always"
        ? display.showValues
        : "never",
    legend: display.legend !== "hidden",
    unit: typeof display.cellUnit === "string" ? display.cellUnit : undefined,
    decimals: numberOf(display.cellDecimals),
  }
}

export function heatmapModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  queries: DataPanel["queries"]
): HeatmapModel {
  const { xs: sampleXs, entries } = entriesOf(frames, fields)
  const unit = entries[0]?.config.unit
  const labelling: Labelling = {
    unit: typeof display.yUnit === "string" ? display.yUnit : unit,
    decimals: numberOf(display.yDecimals),
  }
  const layout = rowsLayoutOf(display.rowsLayout)
  const boundOf = (entry: Entry) =>
    parseBound(entry.frame.labels?.le) ??
    (layout === "le" || layout === "ge"
      ? parseBound(fields.displayName(entry.frame, entry.field))
      : null)
  const isBuckets =
    entries.length > 0 &&
    display.calculate !== true &&
    layout !== "unknown" &&
    entries.every((entry) => boundOf(entry) !== null)
  const mode: HeatmapMode =
    display.calculate === true
      ? "calculated"
      : isBuckets
        ? "buckets"
        : "ordinal"
  const scale =
    bucketScaleOf(display.yBucketScale) ?? bucketScaleOf(display.yScale)
  let xs = sampleXs
  let shape: Shape
  if (mode === "calculated") {
    const calculated = calculatedModel(entries, sampleXs, labelling, {
      x: bucketSpec(display.xBuckets),
      y: bucketSpec(display.yBuckets),
      scale: scale ?? "linear",
      base: display.yBucketLogBase === 10 ? 10 : defaultLogBase,
    })
    xs = calculated.xs
    shape = calculated
  } else if (mode === "buckets") {
    shape = bucketModel(
      entries,
      sampleXs,
      labelling,
      queries.some((query) => query.format === "heatmap"),
      layout === "ge" ? "ge" : "le",
      (entry) => boundOf(entry) ?? Infinity
    )
  } else {
    shape = ordinalModel(entries, fields)
  }
  const reversed = display.yReverse === true
  const arrangedShape = arranged(
    shape,
    { low: numberOf(display.yMin), high: numberOf(display.yMax) },
    reversed
  )
  return {
    mode,
    xs,
    unit,
    reversed,
    ...arrangedShape,
    ...extent(arrangedShape.cells, shownCell(display)),
  }
}

function rowsLayoutOf(value: unknown): RowsLayout {
  return value === "le" || value === "ge" || value === "unknown"
    ? value
    : "auto"
}

function bucketScaleOf(value: unknown): BucketScale | undefined {
  return value === "linear" || value === "log" || value === "symlog"
    ? value
    : undefined
}

export function colorPosition(look: HeatmapLook, value: number): number {
  let position = rangePosition(value, look.low, look.high)
  if (look.mode === "opacity" && look.exponential) {
    position = position ** look.exponent
  }
  if (look.steps !== null && look.steps > 1) {
    const bucket = Math.min(look.steps - 1, Math.floor(position * look.steps))
    position = bucket / (look.steps - 1)
  }
  return look.reverse ? 1 - position : position
}

export type CellPaint = { color: string; alpha: number }

export function cellPaint(look: HeatmapLook, value: number): CellPaint {
  const position = colorPosition(look, value)
  if (look.mode === "scheme") {
    return { color: schemeColor(look.scheme, position), alpha: 1 }
  }
  return {
    color: look.fill,
    alpha: minOpacity + (1 - minOpacity) * position,
  }
}

export type LegendStep = { value: number; paint: CellPaint }

export function legendScale(look: HeatmapLook): LegendStep[] {
  const count =
    look.steps !== null && look.steps < legendSteps ? look.steps : legendSteps
  const { labelLow, labelHigh } = look
  return Array.from({ length: count }, (_, i) => {
    const value =
      count === 1
        ? labelHigh
        : labelLow + ((labelHigh - labelLow) * i) / (count - 1)
    return { value, paint: cellPaint(look, value) }
  })
}

export function legendDescription(
  look: HeatmapLook,
  format: (value: number) => string
): string {
  return `Color scale from ${format(look.labelLow)} to ${format(look.labelHigh)}`
}

export function formatCounts(
  values: number[],
  unit?: string,
  decimals?: number
): (value: number) => string {
  const shared = commonDecimals(values, unit, decimals)
  return (value) => formatValue(value, unit, shared)
}

export type Box = { left: number; top: number; width: number; height: number }

export function insetCell(box: Box, gap: number): Box {
  const width = Math.max(1, box.width - gap)
  const height = Math.max(1, box.height - gap)
  return {
    left: box.left + (box.width - width) / 2,
    top: box.top + (box.height - height) / 2,
    width,
    height,
  }
}

export function labelFits(
  show: ShowValues,
  box: Box,
  textWidth: number
): boolean {
  if (show === "never") return false
  if (show === "always") return true
  return (
    textWidth + 2 * labelPadding <= box.width &&
    labelFontSize + 2 * labelPadding <= box.height
  )
}

export const cellLabelFontSize = labelFontSize

export type HeatmapCell = { row: number; col: number }

const heatmapKeys = [
  "ArrowUp",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
]

export type HeatmapKey =
  "ArrowUp" | "ArrowDown" | "ArrowLeft" | "ArrowRight" | "Home" | "End"

export function isHeatmapKey(key: string): key is HeatmapKey {
  return heatmapKeys.includes(key)
}

export function stepCell(
  model: HeatmapModel,
  cell: HeatmapCell | null,
  key: HeatmapKey
): HeatmapCell | null {
  const lastRow = model.rows.length - 1
  const lastCol = model.xs.length - 1
  if (lastRow < 0 || lastCol < 0) return null
  if (cell === null) {
    return { row: lastRow, col: key === "End" ? lastCol : 0 }
  }
  const clamp = (value: number, last: number) =>
    Math.min(last, Math.max(0, value))
  switch (key) {
    case "ArrowUp":
      return { row: clamp(cell.row + 1, lastRow), col: cell.col }
    case "ArrowDown":
      return { row: clamp(cell.row - 1, lastRow), col: cell.col }
    case "ArrowLeft":
      return { row: cell.row, col: clamp(cell.col - 1, lastCol) }
    case "ArrowRight":
      return { row: cell.row, col: clamp(cell.col + 1, lastCol) }
    case "Home":
      return { row: cell.row, col: 0 }
    case "End":
      return { row: cell.row, col: lastCol }
  }
}

export function columnAt(xs: readonly number[], x: number): number | null {
  if (xs.length === 0) return null
  const first = xs[0]
  const step = (xs[xs.length - 1] - first) / (xs.length - 1)
  if (!(step > 0)) return x === first ? 0 : null
  const position = (x - first) / step
  if (position < -0.5 || position > xs.length - 0.5) return null
  return Math.min(xs.length - 1, Math.max(0, Math.round(position)))
}

function edge(index: number, count: number, size: number): number {
  return Math.round((index * size) / count)
}

export function cellBox(
  model: HeatmapModel,
  cell: HeatmapCell,
  width: number,
  height: number
): Box {
  const columns = model.xs.length
  const rows = model.rows.length
  const top = edge(rows - cell.row - 1, rows, height)
  const left = edge(cell.col, columns, width)
  return {
    left,
    top,
    width: edge(cell.col + 1, columns, width) - left,
    height: edge(rows - cell.row, rows, height) - top,
  }
}

export function cellCenter(
  model: HeatmapModel,
  cell: HeatmapCell,
  width: number,
  height: number
): { x: number; y: number } {
  const box = cellBox(model, cell, width, height)
  return { x: box.left + box.width / 2, y: box.top + box.height / 2 }
}

function timeRange(xs: number[], col: number): string {
  const start = xs[col]
  const next =
    col + 1 < xs.length
      ? xs[col + 1]
      : xs.length > 1
        ? start + (start - xs[col - 1])
        : null
  return next === null
    ? formatDateTime(start)
    : `${formatDateTime(start)} to ${formatDateTime(next)}`
}

export function cellText(
  model: HeatmapModel,
  cell: HeatmapCell,
  format: (value: number) => string
): string {
  const value = model.cells[cell.row][cell.col]
  const text = value === null ? "-" : format(value)
  const row = model.rows[cell.row].range
  const range = timeRange(model.xs, cell.col)
  return model.mode === "ordinal"
    ? `${range}, series ${row}, value ${text}`
    : `${range}, bucket ${row}, count ${text}`
}
