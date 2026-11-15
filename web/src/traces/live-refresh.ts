import type { TimeRangeValue } from "@/dashboards/time-range-picker"

const liveIntervalMs = 10_000

export const liveBlockedText =
  "Live works on a relative time range, like the last 15 minutes."

function isNowExpression(value: string): boolean {
  return value === "now" || value.startsWith("now-") || value.startsWith("now/")
}

export function isRelativeRange(range: TimeRangeValue): boolean {
  return isNowExpression(range.from) && isNowExpression(range.to)
}

export function liveRefreshMs(
  on: boolean,
  range: TimeRangeValue
): number | false {
  return on && isRelativeRange(range) ? liveIntervalMs : false
}
