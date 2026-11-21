import {
  clip,
  fit,
  inRect,
  msToIso,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import type { HeatmapModel } from "./heatmap-model"

export type HeatmapAnchorLayout = {
  body: Size
  origin: Point
  plot: Size
  model: Pick<HeatmapModel, "xs" | "rows" | "cells">
  format: (value: number) => string
}

export function hitTest(
  { body, origin, plot, model, format }: HeatmapAnchorLayout,
  point: Point
): Anchor | null {
  const x = point.x - origin.x
  const y = point.y - origin.y
  const columns = model.xs.length
  const rows = model.rows.length
  if (x < 0 || y < 0 || x > plot.width || y > plot.height) return null
  if (columns === 0 || rows === 0) return null
  if (!(plot.width > 0 && plot.height > 0)) return null
  const col = Math.min(columns - 1, Math.floor((x / plot.width) * columns))
  const row =
    rows - 1 - Math.min(rows - 1, Math.floor((y / plot.height) * rows))
  const value = model.cells[row]?.[col] ?? null
  const from = msToIso(model.xs[col])
  if (from === null) return null
  return fit({
    kind: "heat_cell",
    pos: positionOf(point, body),
    from,
    category: clip(model.rows[row].label),
    ...(value === null ? {} : { shown: clip(format(value)) }),
  })
}

function stepOf(model: HeatmapAnchorLayout["model"]): number {
  const { xs } = model
  return xs.length > 1 ? (xs[xs.length - 1] - xs[0]) / (xs.length - 1) : 0
}

function spanOf(layout: HeatmapAnchorLayout): number {
  const { xs } = layout.model
  return xs.length > 1 ? xs[xs.length - 1] - xs[0] + stepOf(layout.model) : 0
}

function timeAt(layout: HeatmapAnchorLayout, x: number): number {
  return layout.model.xs[0] + (x / layout.plot.width) * spanOf(layout)
}

function pixelAt(layout: HeatmapAnchorLayout, time: number): number {
  return ((time - layout.model.xs[0]) / spanOf(layout)) * layout.plot.width
}

function plotArea(layout: HeatmapAnchorLayout): Rect {
  return { ...layout.origin, ...layout.plot }
}

export function rangeTest(
  layout: HeatmapAnchorLayout,
  start: Point,
  end: Point
): Anchor | null {
  const area = plotArea(layout)
  if (!inRect(area, start) || !inRect(area, end)) return null
  if (!(layout.plot.width > 0) || !(spanOf(layout) > 0)) return null
  const [low, high] = start.x <= end.x ? [start, end] : [end, start]
  const from = msToIso(timeAt(layout, low.x - layout.origin.x))
  const to = msToIso(timeAt(layout, high.x - layout.origin.x))
  if (from === null || to === null) return null
  return {
    kind: "range",
    pos: positionOf(
      { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 },
      layout.body
    ),
    from,
    to,
  }
}

function locateRange(layout: HeatmapAnchorLayout, anchor: Anchor): Rect | null {
  if (!(spanOf(layout) > 0)) return null
  const from = Date.parse(anchor.from ?? "")
  const to = Date.parse(anchor.to ?? "")
  const left = Math.max(0, pixelAt(layout, from))
  const right = Math.min(layout.plot.width, pixelAt(layout, to))
  if (!(left <= right)) return null
  return {
    x: layout.origin.x + left,
    y: layout.origin.y,
    width: right - left,
    height: layout.plot.height,
  }
}

export function locate(
  layout: HeatmapAnchorLayout,
  anchor: Anchor
): Rect | null {
  const { origin, plot, model } = layout
  if (anchor.kind === "range") return locateRange(layout, anchor)
  if (anchor.kind !== "heat_cell") return null
  const col = model.xs.findIndex((x) => msToIso(x) === anchor.from)
  const row = model.rows.findIndex(
    ({ label }) => clip(label) === anchor.category
  )
  if (col < 0 || row < 0) return null
  const width = plot.width / model.xs.length
  const height = plot.height / model.rows.length
  return {
    x: origin.x + col * width,
    y: origin.y + (model.rows.length - 1 - row) * height,
    width,
    height,
  }
}
