import { zonedTimeValue } from "@/lib/time"

import { countText, type TraceRow } from "./trace-rows"

export type ScatterPoint = {
  traceId: string
  x: number
  y: number
  error: boolean
}

export type TimeWindow = { start: number; end: number }

const minMs = 1

function clamp(value: number): number {
  return Math.min(1, Math.max(0, value))
}

export type DurationScale = {
  minMs: number
  maxMs: number
  y: (ms: number) => number
  ms: (y: number) => number
}

export function durationScale(maxMs: number): DurationScale {
  const topExponent = Math.floor(Math.log10(Math.max(maxMs, minMs))) + 1
  const top = 10 ** topExponent
  const span = Math.log10(top / minMs)
  return {
    minMs,
    maxMs: top,
    y: (ms) => clamp(Math.log10(Math.max(ms, minMs) / minMs) / span),
    ms: (y) => minMs * 10 ** (clamp(y) * span),
  }
}

export function durationTicks(maxMs: number): number[] {
  const top = durationScale(maxMs).maxMs
  const decades: number[] = []
  for (let tick = top; tick >= 10; tick /= 10) decades.push(tick)
  const stride = Math.ceil(decades.length / 4)
  return decades.filter((_, index) => index % stride === 0).reverse()
}

export function durationTickText(ms: number): string {
  return ms < 1000 ? `${ms} ms` : `${ms / 1000} s`
}

export function scatterPoints(
  rows: readonly TraceRow[],
  range: TimeWindow,
  maxMs: number
): ScatterPoint[] {
  const scale = durationScale(maxMs)
  const span = range.end - range.start
  const points: ScatterPoint[] = []
  for (const row of rows) {
    if (row.start === null || row.durationMs === null) continue
    points.push({
      traceId: row.traceId,
      x: span > 0 ? clamp((row.start - range.start) / span) : 0,
      y: scale.y(row.durationMs),
      error: row.errors !== null && row.errors > 0,
    })
  }
  return points
}

const minute = 60_000
const hour = 60 * minute
const day = 24 * hour
const timeSteps = [
  minute,
  2 * minute,
  5 * minute,
  10 * minute,
  15 * minute,
  30 * minute,
  hour,
  2 * hour,
  3 * hour,
  6 * hour,
  12 * hour,
  day,
  2 * day,
  7 * day,
]

export function windowTicks(range: TimeWindow, maxTicks: number): number[] {
  const span = range.end - range.start
  if (!(span > 0)) return []
  const step =
    timeSteps.find((candidate) => span / candidate <= maxTicks) ??
    timeSteps[timeSteps.length - 1]!
  const offset = -new Date(range.start).getTimezoneOffset() * minute
  const ticks: number[] = []
  for (
    let tick = Math.ceil((range.start + offset) / step) * step - offset;
    tick <= range.end;
    tick += step
  ) {
    ticks.push(tick)
  }
  return ticks
}

export type Selection = {
  from: number
  to: number
  minMs: number
  maxMs: number | null
}

export type SelectionHandle = "start" | "end" | "floor"

type Point = { x: number; y: number }
type PlotSize = { width: number; height: number }

const clickThresholdPx = 4
const ceilingSnapPx = 2
const handleTimeSteps = 60
const floorStepDecades = 0.1

function within(value: number, limit: number): number {
  return Math.min(limit, Math.max(0, value))
}

export function selectionFromDrag(
  a: Point,
  b: Point,
  plot: PlotSize,
  range: TimeWindow,
  scale: DurationScale
): Selection | null {
  if (!(plot.width > 0) || !(plot.height > 0)) return null
  if (
    Math.abs(a.x - b.x) < clickThresholdPx ||
    Math.abs(a.y - b.y) < clickThresholdPx
  ) {
    return null
  }
  const left = within(Math.min(a.x, b.x), plot.width)
  const right = within(Math.max(a.x, b.x), plot.width)
  const top = within(Math.min(a.y, b.y), plot.height)
  const bottom = within(Math.max(a.y, b.y), plot.height)
  const span = range.end - range.start
  return {
    from: range.start + (left / plot.width) * span,
    to: range.start + (right / plot.width) * span,
    minMs: scale.ms(1 - bottom / plot.height),
    maxMs: top <= ceilingSnapPx ? null : scale.ms(1 - top / plot.height),
  }
}

export function inSelection(row: TraceRow, selection: Selection): boolean {
  return (
    row.start !== null &&
    row.durationMs !== null &&
    row.start >= selection.from &&
    row.start <= selection.to &&
    row.durationMs >= selection.minMs &&
    (selection.maxMs === null || row.durationMs <= selection.maxMs)
  )
}

export function selectedRows(
  rows: readonly TraceRow[],
  selection: Selection
): TraceRow[] {
  return rows.filter((row) => inSelection(row, selection))
}

export function nudgeSelection(
  selection: Selection,
  handle: SelectionHandle,
  steps: number,
  range: TimeWindow,
  scale: DurationScale
): Selection {
  const timeStep = (range.end - range.start) / handleTimeSteps
  switch (handle) {
    case "start":
      return {
        ...selection,
        from: Math.min(
          selection.to,
          Math.max(range.start, selection.from + steps * timeStep)
        ),
      }
    case "end":
      return {
        ...selection,
        to: Math.max(
          selection.from,
          Math.min(range.end, selection.to + steps * timeStep)
        ),
      }
    case "floor":
      return {
        ...selection,
        minMs: Math.min(
          selection.maxMs ?? scale.maxMs,
          Math.max(
            scale.minMs,
            selection.minMs * 10 ** (steps * floorStepDecades)
          )
        ),
      }
  }
}

function trimmed(value: number, decimals: number): string {
  return Number(value.toFixed(decimals)).toLocaleString("en")
}

export function durationBoundText(ms: number): string {
  return ms < 1000
    ? `${trimmed(ms, ms < 10 ? 1 : 0)} ms`
    : `${trimmed(ms / 1000, 2)} s`
}

export function selectionTimeText(
  ms: number,
  zone: string | undefined
): string {
  return zonedTimeValue(String(Math.round(ms)), zone)
}

export function selectionText(
  selection: Selection,
  count: number,
  zone: string | undefined
): string {
  const bounds =
    selection.maxMs === null
      ? `slower than ${durationBoundText(selection.minMs)}`
      : `between ${durationBoundText(selection.minMs)} and ${durationBoundText(selection.maxMs)}`
  return `${countText(count, "trace")} selected: ${selectionTimeText(selection.from, zone)} to ${selectionTimeText(selection.to, zone)}, ${bounds}`
}
