import {
  clip,
  fit,
  inRect,
  msToIso,
  positionOf,
  namesLabels,
  seriesLabels,
  type Anchor,
  type Point,
  type Rect,
  type Size,
} from "../anchors"

export type AnchorSeries = {
  name: string
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
  hidden?: readonly Record<string, string>[]
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
  const { plot, xMin, xMax } = layout
  if (!(plot.width > 0) || !Number.isFinite(xMin) || !Number.isFinite(xMax)) {
    return null
  }
  if (!inRect(plot, point)) return null
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
  const seconds = layout.xs[index]
  if (seconds === undefined) return null
  const x = pixelAt(layout, seconds)
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
    const leftSeconds = layout.xs[index]
    const rightSeconds = layout.xs[index + 1]
    if (leftSeconds === undefined || rightSeconds === undefined) continue
    const left = pixelAt(layout, leftSeconds)
    const right = pixelAt(layout, rightSeconds)
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
  const seconds = index < 0 ? secondsAt(layout, at.x) : layout.xs[index]
  if (seconds === undefined) return null
  const time = msToIso(seconds * 1000)
  if (time === null) return null
  if (index < 0) return { kind: "time", pos, from: time }
  const near = seriesAt(layout, at, index)
  if (near !== null && near.away <= pointRadius) {
    const entry = layout.series[near.series]
    if (entry === undefined) return null
    const value = entry.values[index]
    return fit({
      kind: "point",
      pos,
      from: time,
      series: seriesLabels(entry.labels, entry.name),
      ...(value === null || value === undefined
        ? {}
        : { shown: clip(entry.format(value)) }),
    })
  }
  const line = lineAt(layout, at)
  if (line !== null) {
    const entry = layout.series[line]
    if (entry === undefined) return null
    return fit({
      kind: "series",
      pos,
      series: seriesLabels(entry.labels, entry.name),
    })
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
  const from = msToIso(secondsAt(layout, low.x) * 1000)
  const to = msToIso(secondsAt(layout, high.x) * 1000)
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

function seriesIndex(layout: TimeseriesAnchorLayout, anchor: Anchor): number {
  return layout.series.findIndex((entry) =>
    namesLabels(anchor.series, seriesLabels(entry.labels, entry.name))
  )
}

function sampleIndex(layout: TimeseriesAnchorLayout, seconds: number): number {
  const { xs } = layout
  let best = -1
  let distance = Infinity
  xs.forEach((x, index) => {
    const away = Math.abs(x - seconds)
    if (away < distance) {
      best = index
      distance = away
    }
  })
  if (best < 0) return -1
  const first = xs[0]
  const last = xs.at(-1)
  const step =
    xs.length > 1 && first !== undefined && last !== undefined
      ? (last - first) / (xs.length - 1)
      : 0
  return distance <= step / 2 ? best : -1
}

function inWindow(layout: TimeseriesAnchorLayout, seconds: number): boolean {
  return seconds >= layout.xMin && seconds <= layout.xMax
}

function secondsOf(value: string | undefined): number {
  return value === undefined ? Number.NaN : Date.parse(value) / 1000
}

function pointRect(
  layout: TimeseriesAnchorLayout,
  index: number,
  y: number
): Rect | null {
  const seconds = layout.xs[index]
  if (seconds === undefined) return null
  return {
    x: layout.plot.x + pixelAt(layout, seconds) - pointRadius,
    y: layout.plot.y + y - pointRadius,
    width: pointRadius * 2,
    height: pointRadius * 2,
  }
}

function band(layout: TimeseriesAnchorLayout, from: number, to: number): Rect {
  const left = pixelAt(layout, Math.max(from, layout.xMin))
  const right = pixelAt(layout, Math.min(to, layout.xMax))
  return {
    x: layout.plot.x + left - 1,
    y: layout.plot.y,
    width: Math.max(2, right - left + 2),
    height: layout.plot.height,
  }
}

function locatePoint(
  layout: TimeseriesAnchorLayout,
  anchor: Anchor
): Rect | null {
  const series = seriesIndex(layout, anchor)
  const seconds = secondsOf(anchor.from)
  if (series < 0 || !inWindow(layout, seconds)) return null
  const index = sampleIndex(layout, seconds)
  const y = index < 0 ? null : layout.series[series]?.ys[index]
  return y === null || y === undefined ? null : pointRect(layout, index, y)
}

function locateSeries(
  layout: TimeseriesAnchorLayout,
  anchor: Anchor
): Rect | null {
  const series = seriesIndex(layout, anchor)
  if (series < 0) return null
  const x = anchor.pos.x * layout.body.width - layout.plot.x
  const entry = layout.series[series]
  if (entry === undefined) return null
  const { ys } = entry
  const index = nearestIndex(layout, x)
  const y = index < 0 ? null : ys[index]
  if (y !== null && y !== undefined) return pointRect(layout, index, y)
  const drawn = ys.findIndex((value) => value !== null && value !== undefined)
  return drawn < 0 ? null : pointRect(layout, drawn, ys[drawn] as number)
}

export function locate(
  layout: TimeseriesAnchorLayout,
  anchor: Anchor
): Rect | null {
  switch (anchor.kind) {
    case "point":
      return locatePoint(layout, anchor)
    case "series":
      return locateSeries(layout, anchor)
    case "time": {
      const seconds = secondsOf(anchor.from)
      return inWindow(layout, seconds) ? band(layout, seconds, seconds) : null
    }
    case "range": {
      const from = secondsOf(anchor.from)
      const to = secondsOf(anchor.to)
      return to < layout.xMin || from > layout.xMax
        ? null
        : band(layout, from, to)
    }
    default:
      return null
  }
}

function namesHidden(layout: TimeseriesAnchorLayout, anchor: Anchor): boolean {
  return (layout.hidden ?? []).some((labels) =>
    namesLabels(anchor.series, labels)
  )
}

function outsideShownTime(
  layout: TimeseriesAnchorLayout,
  anchor: Anchor
): boolean {
  switch (anchor.kind) {
    case "point":
    case "time":
      return !inWindow(layout, secondsOf(anchor.from))
    case "range":
      return (
        secondsOf(anchor.to) < layout.xMin ||
        secondsOf(anchor.from) > layout.xMax
      )
    default:
      return false
  }
}

export function hides(layout: TimeseriesAnchorLayout, anchor: Anchor): boolean {
  return namesHidden(layout, anchor) || outsideShownTime(layout, anchor)
}
