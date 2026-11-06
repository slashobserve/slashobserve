import { resolveTime, type TimeOptions } from "./time"

export type AbsoluteRange = { from: string; to: string }

function resolved(
  from: string,
  to: string,
  now: number,
  options: TimeOptions | undefined
): { start: number; end: number } | null {
  const start = resolveTime(from, now, false, options)
  const end = resolveTime(to, now, true, options)
  if (Number.isNaN(start) || Number.isNaN(end) || start >= end) return null
  return { start, end }
}

function wholeSecond(ms: number): number {
  return Math.round(ms / 1000) * 1000
}

function absolute(start: number, end: number, now: number): AbsoluteRange {
  const length = end - start
  const last = Math.min(end, now)
  const first = end > now ? now - length : start
  return {
    from: new Date(wholeSecond(first)).toISOString(),
    to: new Date(wholeSecond(last)).toISOString(),
  }
}

export function zoomOut(
  from: string,
  to: string,
  now: number,
  options?: TimeOptions
): AbsoluteRange | null {
  const range = resolved(from, to, now, options)
  if (range === null) return null
  const length = range.end - range.start
  const center = range.start + length / 2
  return absolute(center - length, center + length, now)
}

export function shift(
  from: string,
  to: string,
  now: number,
  direction: -1 | 1,
  options?: TimeOptions
): AbsoluteRange | null {
  const range = resolved(from, to, now, options)
  if (range === null) return null
  const step = ((range.end - range.start) / 2) * direction
  return absolute(range.start + step, range.end + step, now)
}
