import type { TraceRow } from "./trace-rows"

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

export function durationScale(maxMs: number): {
  minMs: number
  maxMs: number
  y: (ms: number) => number
} {
  const topExponent = Math.floor(Math.log10(Math.max(maxMs, minMs))) + 1
  const top = 10 ** topExponent
  const span = Math.log10(top / minMs)
  return {
    minMs,
    maxMs: top,
    y: (ms) => clamp(Math.log10(Math.max(ms, minMs) / minMs) / span),
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
