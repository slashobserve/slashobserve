import { rangePosition } from "../color-scheme"
import { thresholdColor } from "../thresholds"
import type { ExemplarPoint } from "../timeseries/exemplars"
import {
  visibleCell,
  type HeatmapLook,
  type HeatmapModel,
} from "./heatmap-model"

type TooltipMode = "single" | "all" | "hidden"

export type TooltipLook = {
  mode: TooltipMode
  histogram: boolean
  colorScale: boolean
  maxWidth: number | undefined
  maxHeight: number | undefined
}

export const maxTooltipRows = 10
const exemplarRadius = 6

function sizeOf(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value > 0
    ? value
    : undefined
}

export function tooltipLook(display: Record<string, unknown>): TooltipLook {
  const mode = display.tooltip
  return {
    mode: mode === "all" || mode === "hidden" ? mode : "single",
    histogram: display.tooltipHistogram === true,
    colorScale: display.tooltipColorScale === true,
    maxWidth: sizeOf(display.tooltipMaxWidth),
    maxHeight: sizeOf(display.tooltipMaxHeight),
  }
}

export type ColumnEntry = { row: number; range: string; value: number }

export function columnEntries(
  model: HeatmapModel,
  look: HeatmapLook,
  col: number,
  hoveredRow: number
): { entries: ColumnEntry[]; more: number } {
  const all = model.cells.flatMap((cells, row): ColumnEntry[] => {
    const value = cells[col]
    return visibleCell(look, value)
      ? [{ row, range: model.rows[row].range, value }]
      : []
  })
  all.sort((a, b) => b.row - a.row)
  if (all.length <= maxTooltipRows) return { entries: all, more: 0 }
  const at = Math.max(
    0,
    all.findIndex((entry) => entry.row <= hoveredRow)
  )
  const start = Math.min(
    all.length - maxTooltipRows,
    Math.max(0, at - Math.floor(maxTooltipRows / 2))
  )
  return {
    entries: all.slice(start, start + maxTooltipRows),
    more: all.length - maxTooltipRows,
  }
}

export type HistogramBar = { row: number; share: number }

export function histogramBars(
  model: HeatmapModel,
  look: HeatmapLook,
  col: number
): HistogramBar[] {
  const values = model.cells.map((cells) => {
    const value = cells[col]
    return visibleCell(look, value) ? value : 0
  })
  const largest = Math.max(0, ...values)
  if (largest <= 0) return []
  return values.map((value, row) => ({ row, share: value / largest })).reverse()
}

export function scaleMarker(look: HeatmapLook, value: number): number {
  return rangePosition(value, look.labelLow, look.labelHigh)
}

export function rowPosition(model: HeatmapModel, value: number): number | null {
  for (let i = 0; i < model.rows.length; i++) {
    const { from, to } = model.rows[i]
    if (from !== undefined && value < from) continue
    if (to !== undefined && value > to) continue
    const bounded =
      from !== undefined &&
      to !== undefined &&
      Number.isFinite(from) &&
      Number.isFinite(to) &&
      to > from
    const share = bounded ? (value - from) / (to - from) : 0.5
    return i + (model.reversed ? 1 - share : share)
  }
  return null
}

export type ExemplarMark = {
  x: number
  y: number
  value: number
  labels: [string, string][]
}

export function exemplarMarks(
  model: HeatmapModel,
  points: readonly ExemplarPoint[],
  width: number,
  height: number
): ExemplarMark[] {
  const columns = model.xs.length
  const rows = model.rows.length
  if (model.mode === "ordinal" || columns === 0 || rows === 0) return []
  const first = model.xs[0]
  const step = columns > 1 ? (model.xs[columns - 1] - first) / (columns - 1) : 0
  return points.flatMap((point): ExemplarMark[] => {
    const at = step > 0 ? (point.x * 1000 - first) / step : 0
    const position = rowPosition(model, point.y)
    if (position === null || at < 0 || at > columns) return []
    return [
      {
        x: (at / columns) * width,
        y: height * (1 - position / rows),
        value: point.y,
        labels: point.labels,
      },
    ]
  })
}

export function markAt(
  marks: readonly ExemplarMark[],
  x: number,
  y: number
): ExemplarMark | null {
  let nearest: ExemplarMark | null = null
  let best = exemplarRadius * exemplarRadius
  for (const mark of marks) {
    const distance = (mark.x - x) ** 2 + (mark.y - y) ** 2
    if (distance <= best) {
      best = distance
      nearest = mark
    }
  }
  return nearest
}

export type AxisLook = {
  side: "left" | "right" | "hidden"
  width: number | undefined
  label: string | undefined
}

export function axisLook(display: Record<string, unknown>): AxisLook {
  const side = display.yAxis
  return {
    side: side === "right" || side === "hidden" ? side : "left",
    width:
      typeof display.yAxisWidth === "number" ? display.yAxisWidth : undefined,
    label:
      typeof display.yAxisLabel === "string" && display.yAxisLabel !== ""
        ? display.yAxisLabel
        : undefined,
  }
}

export type ExemplarLook = { shown: boolean; color: string }

export function exemplarLook(display: Record<string, unknown>): ExemplarLook {
  return {
    shown: display.exemplars !== false,
    color:
      typeof display.exemplarColor === "string"
        ? thresholdColor(display.exemplarColor)
        : "var(--foreground)",
  }
}
