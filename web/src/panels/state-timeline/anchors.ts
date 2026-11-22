import {
  clip,
  fit,
  inRect,
  msToIso,
  namesLabels,
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
  if (!(plot.width > 0)) return null
  const row = layout.rows[Math.floor((point.y - plot.y) / layout.rowHeight)]
  if (row === undefined) return null
  const at =
    range.from + ((point.x - plot.x) / plot.width) * (range.to - range.from)
  const series = seriesLabels(undefined, row.name)
  const pos = positionOf(point, layout.body)
  const segment = segmentAt(row.segments, at)
  if (segment === null) return fit({ kind: "series", pos, series })
  const from = msToIso(at)
  if (from === null) return null
  return fit({
    kind: "point",
    pos,
    series,
    from,
    ...(segment.text ? { shown: clip(segment.text) } : {}),
  })
}

function timeAt(layout: TimelineAnchorLayout, x: number): number {
  const { range, plot } = layout
  return range.from + ((x - plot.x) / plot.width) * (range.to - range.from)
}

export function rangeTest(
  layout: TimelineAnchorLayout,
  start: Point,
  end: Point
): Anchor | null {
  const { plot, range } = layout
  if (!inRect(plot, start) || !inRect(plot, end)) return null
  if (!(range.from < range.to) || !(plot.width > 0)) return null
  const [low, high] = start.x <= end.x ? [start, end] : [end, start]
  const from = msToIso(timeAt(layout, low.x))
  const to = msToIso(timeAt(layout, high.x))
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

function locateRange(
  layout: TimelineAnchorLayout,
  anchor: Anchor
): Rect | null {
  const { plot, range } = layout
  if (!(range.from < range.to)) return null
  const left = Math.max(0, timeShare(layout, Date.parse(anchor.from ?? "")))
  const right = Math.min(
    plot.width,
    timeShare(layout, Date.parse(anchor.to ?? ""))
  )
  if (!(left <= right)) return null
  return {
    x: plot.x + left,
    y: plot.y,
    width: right - left,
    height: plot.height,
  }
}

function rowIndex(layout: TimelineAnchorLayout, anchor: Anchor): number {
  return layout.rows.findIndex(({ name }) =>
    namesLabels(anchor.series, seriesLabels(undefined, name))
  )
}

function timeShare(layout: TimelineAnchorLayout, time: number): number {
  const { range, plot } = layout
  return ((time - range.from) / (range.to - range.from)) * plot.width
}

export function locate(
  layout: TimelineAnchorLayout,
  anchor: Anchor
): Rect | null {
  if (anchor.kind === "range") return locateRange(layout, anchor)
  if (anchor.kind !== "point" && anchor.kind !== "series") return null
  const { plot, range, rowHeight } = layout
  const index = rowIndex(layout, anchor)
  if (index < 0 || !(range.from < range.to)) return null
  const top = plot.y + index * rowHeight
  if (anchor.kind === "series") {
    return { x: plot.x, y: top, width: plot.width, height: rowHeight }
  }
  const at = Date.parse(anchor.from ?? "")
  const segment = Number.isNaN(at)
    ? null
    : segmentAt(layout.rows[index].segments, at)
  if (segment === null) return null
  const left = Math.max(0, timeShare(layout, segment.from))
  const right = Math.min(plot.width, timeShare(layout, segment.to))
  return { x: plot.x + left, y: top, width: right - left, height: rowHeight }
}

export function hides(layout: TimelineAnchorLayout, anchor: Anchor): boolean {
  const { from, to } = layout.range
  if (anchor.kind === "point") {
    const at = Date.parse(anchor.from ?? "")
    return at < from || at > to
  }
  if (anchor.kind === "range") {
    return (
      Date.parse(anchor.to ?? "") < from || Date.parse(anchor.from ?? "") > to
    )
  }
  return false
}
