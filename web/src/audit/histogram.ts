const units = [
  { ms: 86_400_000, name: "day" },
  { ms: 3_600_000, name: "hour" },
  { ms: 60_000, name: "minute" },
  { ms: 1_000, name: "second" },
]

export function stepText(stepMs: number): string {
  for (const unit of units) {
    if (stepMs >= unit.ms && stepMs % unit.ms === 0) {
      const count = stepMs / unit.ms
      return count === 1 ? unit.name : `${count} ${unit.name}s`
    }
  }
  return `${stepMs} ms`
}

export function barHeight(count: number, highest: number): string | number {
  return count === 0 ? 1 : `${(count / Math.max(1, highest)) * 100}%`
}

export type BucketRange = { from: string; to: string }

export function bucketRange(
  times: readonly number[],
  stepMs: number,
  window: { since: string; until: string },
  a: number,
  b: number
): BucketRange | null {
  const start = times[Math.min(a, b)]
  const last = times[Math.max(a, b)]
  if (start === undefined || last === undefined) return null
  const from = Math.max(start, Date.parse(window.since))
  const to = Math.min(last + stepMs, Date.parse(window.until))
  if (!(to > from)) return null
  return { from: new Date(from).toISOString(), to: new Date(to).toISOString() }
}

export function eventCountText(count: number): string {
  return `${count.toLocaleString("en-US")} ${count === 1 ? "event" : "events"}`
}

export function tickAlign(
  index: number,
  count: number
): "start" | "middle" | "end" {
  const ratio = index / Math.max(1, count)
  if (ratio < 0.06) return "start"
  return ratio > 0.9 ? "end" : "middle"
}
