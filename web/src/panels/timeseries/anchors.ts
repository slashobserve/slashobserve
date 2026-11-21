import {
  clip,
  inRect,
  msToIso,
  positionOf,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"

export type AnchorSeries = {
  labels: Record<string, string>
  ys: readonly (number | null)[]
  values: readonly (number | null)[]
  format: (value: number) => string
}

export type TimeseriesAnchorLayout = {
  body: Size
  plot: Rect
  xMin: number
  xMax: number
  xs: readonly number[]
  series: readonly AnchorSeries[]
}

export const pointRadius = 12

function secondsAt(layout: TimeseriesAnchorLayout, x: number): number {
  const share = x / layout.plot.width
  return layout.xMin + share * (layout.xMax - layout.xMin)
}

function pixelAt(layout: TimeseriesAnchorLayout, seconds: number): number {
  const span = layout.xMax - layout.xMin
  return span > 0 ? ((seconds - layout.xMin) / span) * layout.plot.width : 0
}

function nearestIndex(layout: TimeseriesAnchorLayout, x: number): number {
  let best = -1
  let distance = Infinity
  layout.xs.forEach((seconds, index) => {
    const away = Math.abs(pixelAt(layout, seconds) - x)
    if (away < distance) {
      best = index
      distance = away
    }
  })
  return best
}

function inPlot(layout: TimeseriesAnchorLayout, point: Point): Point | null {
  if (!inRect(layout.plot, point)) return null
  return { x: point.x - layout.plot.x, y: point.y - layout.plot.y }
}

type Nearest = { series: number; away: number }

function closer(best: Nearest | null, series: number, away: number): Nearest {
  return best === null || away < best.away ? { series, away } : best
}

function seriesAt(
  layout: TimeseriesAnchorLayout,
  at: Point,
  index: number
): Nearest | null {
  const x = pixelAt(layout, layout.xs[index])
  let best: Nearest | null = null
  for (const [series, entry] of layout.series.entries()) {
    const y = entry.ys[index]
    if (y === null || y === undefined) continue
    best = closer(best, series, Math.hypot(x - at.x, y - at.y))
  }
  return best
}

function lineAt(layout: TimeseriesAnchorLayout, at: Point): number | null {
  let best: Nearest | null = null
  for (let index = 0; index + 1 < layout.xs.length; index++) {
    const left = pixelAt(layout, layout.xs[index])
    const right = pixelAt(layout, layout.xs[index + 1])
    if (at.x < left || at.x > right) continue
    const share = right > left ? (at.x - left) / (right - left) : 0
    for (const [series, entry] of layout.series.entries()) {
      const from = entry.ys[index]
      const to = entry.ys[index + 1]
      if (from == null || to == null) continue
      best = closer(best, series, Math.abs(from + (to - from) * share - at.y))
    }
  }
  return best !== null && best.away <= pointRadius ? best.series : null
}

export function hitTest(
  layout: TimeseriesAnchorLayout,
  point: Point
): Anchor | null {
  const at = inPlot(layout, point)
  if (at === null) return null
  const pos = positionOf(point, layout.body)
  const index = nearestIndex(layout, at.x)
  if (index < 0)
    return { kind: "time", pos, from: msToIso(secondsAt(layout, at.x) * 1000) }
  const time = msToIso(layout.xs[index] * 1000)
  const near = seriesAt(layout, at, index)
  if (near !== null && near.away <= pointRadius) {
    const entry = layout.series[near.series]
    const value = entry.values[index]
    return {
      kind: "point",
      pos,
      from: time,
      series: entry.labels,
      ...(value === null || value === undefined
        ? {}
        : { shown: clip(entry.format(value)) }),
    }
  }
  const line = lineAt(layout, at)
  if (line !== null) {
    return { kind: "series", pos, series: layout.series[line].labels }
  }
  return { kind: "time", pos, from: time }
}

export function rangeTest(
  layout: TimeseriesAnchorLayout,
  start: Point,
  end: Point
): Anchor | null {
  const first = inPlot(layout, start)
  const last = inPlot(layout, end)
  if (first === null || last === null) return null
  const [low, high] = first.x <= last.x ? [first, last] : [last, first]
  return {
    kind: "range",
    pos: positionOf(
      { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 },
      layout.body
    ),
    from: msToIso(secondsAt(layout, low.x) * 1000),
    to: msToIso(secondsAt(layout, high.x) * 1000),
  }
}
