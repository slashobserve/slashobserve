import {
  clip,
  inRect,
  msToIso,
  positionOf,
  seriesLabels,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"
import { segmentAt, type TimelineRow, type TimeRange } from "./timeline-model"

export type TimelineAnchorLayout = {
  body: Size
  plot: Rect
  range: TimeRange
  rowHeight: number
  rows: readonly Pick<TimelineRow, "name" | "segments">[]
}

export function hitTest(
  layout: TimelineAnchorLayout,
  point: Point
): Anchor | null {
  if (!inRect(layout.plot, point)) return null
  const { range, plot } = layout
  if (!(range.from < range.to) || !(layout.rowHeight > 0)) return null
  const row = layout.rows[Math.floor((point.y - plot.y) / layout.rowHeight)]
  if (row === undefined) return null
  const at =
    range.from + ((point.x - plot.x) / plot.width) * (range.to - range.from)
  const series = seriesLabels(undefined, row.name)
  const pos = positionOf(point, layout.body)
  const segment = segmentAt(row.segments, at)
  if (segment === null) return { kind: "series", pos, series }
  return {
    kind: "point",
    pos,
    series,
    from: msToIso(at),
    ...(segment.text ? { shown: clip(segment.text) } : {}),
  }
}
