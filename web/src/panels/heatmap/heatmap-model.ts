import { formatDateTime } from "@/lib/time"
import {
  axisDecimals,
  commonDecimals,
  formatValue,
  ownDecimals,
} from "@/lib/units"
import { isExpressionQuery, type DataPanel } from "@/spec"

import type { FieldConfig, FieldResolver } from "../field-config"
import { isNumber } from "../reducers"
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
}

export const calculatedRows = 16
const legendSteps = 5
const minOpacity = 0.1

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

function boundFormatter(
  bounds: number[],
  unit: string | undefined
): (bound: number) => string {
  const decimals = axisDecimals(bounds.filter(Number.isFinite), unit)
  return (bound) => formatValue(bound, unit, decimals)
}

function exactFormatter(unit: string | undefined): (bound: number) => string {
  return (bound) => formatValue(bound, unit, ownDecimals(bound, unit))
}

function bucketModel(
  entries: Entry[],
  xs: number[],
  unit: string | undefined,
  cumulative: boolean
): Pick<HeatmapModel, "rows" | "cells"> {
  const byBound = new Map<number, (number | null)[]>()
  for (const { frame, values } of entries) {
    const bound = parseBound(frame.labels?.le) ?? Infinity
    byBound.set(
      bound,
      sumInto(byBound.get(bound) ?? new Array(xs.length).fill(null), values)
    )
  }
  const bounds = [...byBound.keys()].sort((a, b) => a - b)
  const raw = bounds.map((bound) => byBound.get(bound) ?? [])
  const format = exactFormatter(unit)
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

function calculatedModel(
  entries: Entry[],
  xs: number[],
  unit: string | undefined,
  logarithmic: boolean
): Pick<HeatmapModel, "rows" | "cells"> {
  const numbers = entries.flatMap(({ values }) => values.filter(isNumber))
  if (numbers.length === 0) return { rows: [], cells: [] }
  let low = Infinity
  let high = -Infinity
  for (const value of numbers) {
    if (value < low) low = value
    if (value > high) high = value
  }
  const log = logarithmic && low > 0
  const toScale = (value: number) => (log ? Math.log(value) : value)
  const fromScale = (value: number) => (log ? Math.exp(value) : value)
  const start = toScale(low)
  const span = toScale(high) - start
  const edges = Array.from({ length: calculatedRows + 1 }, (_, i) =>
    i === calculatedRows ? high : fromScale(start + (span * i) / calculatedRows)
  )
  const format = boundFormatter(edges, unit)
  const rows = edges.slice(0, calculatedRows).map((from, i): HeatmapRow => {
    const label = format(from)
    return {
      label,
      from,
      to: edges[i + 1],
      range: `${label} - ${format(edges[i + 1])}`,
    }
  })
  const cells = rows.map(() => new Array<number | null>(xs.length).fill(0))
  for (const { values } of entries) {
    values.forEach((value, col) => {
      if (value === null) return
      const position = span === 0 ? 0 : (toScale(value) - start) / span
      const row = Math.min(
        calculatedRows - 1,
        Math.floor(position * calculatedRows)
      )
      cells[row][col] = (cells[row][col] ?? 0) + 1
    })
  }
  return { rows, cells }
}

function ordinalModel(
  entries: Entry[],
  fields: FieldResolver
): Pick<HeatmapModel, "rows" | "cells"> {
  const labels = entries.map(({ frame, field }) =>
    fields.displayName(frame, field)
  )
  return {
    rows: labels.map((label) => ({ label, range: label })),
    cells: entries.map(({ values }) => values),
  }
}

function extent(cells: (number | null)[][]): { min: number; max: number } {
  let min = Infinity
  let max = 0
  for (const row of cells) {
    for (const value of row) {
      if (value === null || value <= 0) continue
      if (value < min) min = value
      if (value > max) max = value
    }
  }
  return { min: min === Infinity ? 0 : min, max }
}

export function heatmapModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  queries: DataPanel["queries"]
): HeatmapModel {
  const { xs, entries } = entriesOf(frames, fields)
  const unit = entries[0]?.config.unit
  const isBuckets =
    entries.length > 0 &&
    display.calculate !== true &&
    entries.every(({ frame }) => parseBound(frame.labels?.le) !== null)
  const mode: HeatmapMode =
    display.calculate === true
      ? "calculated"
      : isBuckets
        ? "buckets"
        : "ordinal"
  const shape =
    mode === "calculated"
      ? calculatedModel(entries, xs, unit, display.yScale === "log")
      : mode === "buckets"
        ? bucketModel(
            entries,
            xs,
            unit,
            queries.some(
              (query) => !isExpressionQuery(query) && query.format === "heatmap"
            )
          )
        : ordinalModel(entries, fields)
  return { mode, xs, unit, ...shape, ...extent(shape.cells) }
}

export function cellOpacity(value: number, max: number): number {
  return minOpacity + (1 - minOpacity) * (max > 0 ? value / max : 0)
}

export type LegendStep = { value: number; opacity: number }

export function legendScale(min: number, max: number): LegendStep[] {
  return Array.from({ length: legendSteps }, (_, i) => {
    const value = min + ((max - min) * i) / (legendSteps - 1)
    return { value, opacity: cellOpacity(value, max) }
  })
}

export function formatCounts(values: number[]): (value: number) => string {
  const decimals = commonDecimals(values)
  return (value) => formatValue(value, undefined, decimals)
}

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

export function edge(index: number, count: number, size: number): number {
  return Math.round((index * size) / count)
}

export function cellBox(
  model: HeatmapModel,
  cell: HeatmapCell,
  width: number,
  height: number
): { left: number; top: number; width: number; height: number } {
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
