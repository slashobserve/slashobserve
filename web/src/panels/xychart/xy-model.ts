import { axisDecimals, formatValue } from "@/lib/units"

import { styleForSlot } from "../colors"
import {
  displayValue,
  fieldRange,
  type FieldConfig,
  type FieldResolver,
} from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { isNumber } from "../reducers"
import { thresholdColor } from "../thresholds"
import {
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"

export type XyShow = "points" | "lines" | "points+lines"
export type XyLegend = "bottom" | "right" | "hidden"
export type XyTooltip = "single" | "all" | "hidden"

type XyPoint = readonly [x: number, y: number, color: string]

type XySeries = {
  key: string
  name: string
  color: string
  xConfig: FieldConfig
  yConfig: FieldConfig
  points: XyPoint[]
}

export type XyRange = { min: number; max: number }

type XyAxis = { name: string; config: FieldConfig }

export type XyModel = {
  series: XySeries[]
  x: XyRange
  y: XyRange
  xAxis: XyAxis | null
  yAxis: XyAxis | null
  reason: string | null
}

export type XyScale = { width: number; height: number; radius: number }

export type XyHit = { series: number; index: number; distance: number }

export type XyTick = { value: number; text: string }

export const missingFieldsReason =
  "An XY chart needs an x field and another number field in the same frame; join queries with a joinByField transform."

const rangePadding = 0.05
const emptyRange: XyRange = { min: 0, max: 1 }
const minPointSize = 1
const maxPointSize = 40
const defaultPointSize = 5

export function showOf(display: Record<string, unknown>): XyShow {
  if (display.show === "lines") return "lines"
  if (display.show === "points+lines") return "points+lines"
  return "points"
}

export function pointSizeOf(display: Record<string, unknown>): number {
  const size = display.pointSize
  if (typeof size !== "number" || !Number.isFinite(size)) {
    return defaultPointSize
  }
  return Math.min(maxPointSize, Math.max(minPointSize, size))
}

export function legendOf(display: Record<string, unknown>): XyLegend {
  if (display.legend === "right") return "right"
  if (display.legend === "hidden") return "hidden"
  return "bottom"
}

export function tooltipOf(display: Record<string, unknown>): XyTooltip {
  if (display.tooltip === "all") return "all"
  if (display.tooltip === "hidden") return "hidden"
  return "single"
}

function xFieldOf(
  frame: PreparedFrame,
  xName: unknown
): PreparedField | undefined {
  if (typeof xName === "string" && xName !== "") {
    return frame.fields.find(
      (field) => field.type === "number" && matchesName(frame, field, xName)
    )
  }
  return frame.fields.find((field) => field.type === "number")
}

function paints(config: FieldConfig): boolean {
  return (config.thresholds?.length ?? 0) > 0 || config.mappings.length > 0
}

function pointsOf(
  x: PreparedField,
  y: PreparedField,
  config: FieldConfig,
  seriesColor: string
): XyPoint[] {
  const colored = paints(config)
  const { min, max } = fieldRange(y)
  const length = Math.min(x.values.length, y.values.length)
  const points: XyPoint[] = []
  for (let i = 0; i < length; i++) {
    const px = x.values[i]
    const py = y.values[i]
    if (!isNumber(px) || !isNumber(py)) continue
    const color = colored
      ? (displayValue(py, config, { fieldMin: min, fieldMax: max }).color ??
        seriesColor)
      : seriesColor
    points.push([px, py, color])
  }
  return points
}

function paddedRange(
  values: Iterable<readonly [number, number]>,
  bounds: FieldConfig[]
): XyRange {
  let min = Infinity
  let max = -Infinity
  for (const [low, high] of values) {
    if (low < min) min = low
    if (high > max) max = high
  }
  if (min > max) return emptyRange
  const span = max - min || Math.abs(max) || 1
  let low = min - span * rangePadding
  let high = max + span * rangePadding
  for (const { min: floor, max: ceiling } of bounds) {
    if (floor !== undefined && floor < low) low = floor
    if (ceiling !== undefined && ceiling > high) high = ceiling
  }
  return { min: low, max: high }
}

function extent(series: XySeries[], axis: 0 | 1): [number, number][] {
  return series.flatMap(({ points }) => {
    if (points.length === 0) return []
    let min = Infinity
    let max = -Infinity
    for (const point of points) {
      const value = point[axis]
      if (value < min) min = value
      if (value > max) max = value
    }
    return [[min, max] as [number, number]]
  })
}

export function xyModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): XyModel {
  const seen = new Map<string, number>()
  const sorted = showOf(display) !== "points"
  const series: XySeries[] = []
  let xAxis: XyAxis | null = null
  let yAxis: XyAxis | null = null
  let qualified = false
  for (const frame of frames) {
    const x = xFieldOf(frame, display.xField)
    if (!x) continue
    const ys = frame.fields.filter(
      (field) => field.type === "number" && field !== x
    )
    if (ys.length === 0) continue
    qualified = true
    const xConfig = fields.configFor(frame, x)
    for (const y of ys) {
      const config = fields.configFor(frame, y)
      if (config.hidden) continue
      const color = config.color
        ? thresholdColor(config.color)
        : styleForSlot(series.length).color
      const points = pointsOf(x, y, config, color)
      if (sorted) points.sort((a, b) => a[0] - b[0])
      series.push({
        key: uniqueKey(seen, seriesKey(frame, y.name)),
        name: fields.displayName(frame, y),
        color,
        xConfig,
        yConfig: config,
        points,
      })
      xAxis ??= { name: x.name, config: xConfig }
      yAxis ??= { name: y.name, config }
    }
  }
  return {
    series,
    x: paddedRange(
      extent(series, 0),
      series.map((item) => item.xConfig)
    ),
    y: paddedRange(
      extent(series, 1),
      series.map((item) => item.yConfig)
    ),
    xAxis,
    yAxis,
    reason: qualified ? null : missingFieldsReason,
  }
}

export function pointCount(model: XyModel): number {
  return model.series.reduce((sum, { points }) => sum + points.length, 0)
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

function cellOf(value: number, range: XyRange, size: number): number {
  const span = range.max - range.min
  const cell = Math.floor(((value - range.min) / span) * size)
  return Math.min(size - 1, Math.max(0, cell))
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
      const cell = cellOf(y, model.y, size) * size + cellOf(x, model.x, size)
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
  const xSpan = model.x.max - model.x.min
  const ySpan = model.y.max - model.y.min
  const dataX = model.x.min + (px / scale.width) * xSpan
  const dataY = model.y.max - (py / scale.height) * ySpan
  const column = cellOf(dataX, model.x, grid.size)
  const row = cellOf(dataY, model.y, grid.size)
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
          const dx = ((point[0] - model.x.min) / xSpan) * scale.width - px
          const dy = ((model.y.max - point[1]) / ySpan) * scale.height - py
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

function tickStep(span: number, count: number): number {
  const raw = span / Math.max(1, count)
  const power = 10 ** Math.floor(Math.log10(raw))
  const fraction = raw / power
  const nice = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10
  return nice * power
}

export function axisTicks(
  range: XyRange,
  config: FieldConfig | undefined,
  count: number
): XyTick[] {
  const span = range.max - range.min
  if (!(span > 0) || count < 1) return []
  const step = tickStep(span, count)
  const first = Math.ceil(range.min / step)
  const last = Math.floor(range.max / step)
  const values: number[] = []
  for (let i = first; i <= last; i++) {
    values.push(Number((i * step).toPrecision(12)))
  }
  const decimals = axisDecimals(values, config?.unit, config?.decimals)
  return values.map((value) => ({
    value,
    text: formatValue(value, config?.unit, decimals),
  }))
}

export function pointSummary(
  model: XyModel,
  hit: Pick<XyHit, "series" | "index">
): { x: string; y: string } {
  const { points, xConfig, yConfig } = model.series[hit.series]
  const [x, y] = points[hit.index]
  return {
    x: formatValue(x, xConfig.unit, xConfig.decimals),
    y: formatValue(y, yConfig.unit, yConfig.decimals),
  }
}
