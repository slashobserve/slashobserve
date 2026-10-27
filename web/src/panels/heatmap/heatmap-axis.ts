import { formatDateTime } from "@/lib/time"

const second = 1000
const minute = 60 * second
const hour = 60 * minute
const day = 24 * hour

const niceIntervals = [
  second,
  5 * second,
  10 * second,
  15 * second,
  30 * second,
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
  14 * day,
  30 * day,
]

export type TimeTick = { index: number; label: string }

export function timeLabel(ms: number, spanMs: number): string {
  const text = formatDateTime(ms)
  if (spanMs > 7 * day) return text.slice(5, 10)
  if (spanMs > day) return text.slice(5, 16)
  return spanMs < 5 * minute ? text.slice(11, 19) : text.slice(11, 16)
}

export function timeTicks(xs: number[], maxTicks: number): TimeTick[] {
  if (xs.length === 0 || maxTicks < 1) return []
  const spanMs = xs[xs.length - 1] - xs[0]
  const interval =
    niceIntervals.find((candidate) => spanMs / candidate <= maxTicks) ??
    niceIntervals[niceIntervals.length - 1]
  const offset = -new Date(xs[0]).getTimezoneOffset() * minute
  const slot = (ms: number) => Math.floor((ms + offset) / interval)
  const ticks: TimeTick[] = []
  xs.forEach((ms, index) => {
    const crossed =
      index === 0
        ? (ms + offset) % interval === 0
        : slot(ms) !== slot(xs[index - 1])
    if (crossed) ticks.push({ index, label: timeLabel(ms, spanMs) })
  })
  return ticks
}
