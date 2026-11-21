import {
  clip,
  msToIso,
  positionOf,
  type Anchor,
  type Point,
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
  const col = Math.min(columns - 1, Math.floor((x / plot.width) * columns))
  const row =
    rows - 1 - Math.min(rows - 1, Math.floor((y / plot.height) * rows))
  const value = model.cells[row]?.[col] ?? null
  return {
    kind: "heat_cell",
    pos: positionOf(point, body),
    from: msToIso(model.xs[col]),
    category: clip(model.rows[row].label),
    ...(value === null ? {} : { shown: clip(format(value)) }),
  }
}
