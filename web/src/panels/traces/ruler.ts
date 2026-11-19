const stepMultipliers = [1, 2, 2.5, 5]
const tickFloors = [4, 3, 2, 1]
const mostTicks = 6
const preferredTicks = 5
const floatSlack = 1e-9
const tickLabelPx = 64
const labelHalfPx = 24

function stepIndexes(
  originMs: number,
  durationMs: number,
  step: number
): { first: number; count: number } {
  const first = Math.ceil((originMs / step) * (1 - floatSlack))
  const last = Math.floor(((originMs + durationMs) / step) * (1 + floatSlack))
  return { first, count: Math.max(0, last - first + 1) }
}

function candidateSteps(durationMs: number): number[] {
  const exponent = Math.floor(Math.log10(durationMs))
  const steps: number[] = []
  for (let power = exponent - 2; power <= exponent + 1; power++) {
    for (const multiplier of stepMultipliers) {
      steps.push(multiplier * 10 ** power)
    }
  }
  return steps
}

function niceStep(
  durationMs: number,
  limit: number,
  originMs: number
): number | null {
  const most = Math.min(mostTicks, limit)
  const preferred = Math.min(preferredTicks, most)
  const steps = candidateSteps(durationMs).map((step) => ({
    step,
    count: stepIndexes(originMs, durationMs, step).count,
  }))
  for (const floor of tickFloors) {
    const fewest = Math.min(floor, most)
    let best: { step: number; distance: number } | null = null
    for (const { step, count } of steps) {
      if (count < fewest || count > most) continue
      const distance = Math.abs(count - preferred)
      if (best === null || distance <= best.distance) best = { step, distance }
    }
    if (best) return best.step
  }
  return null
}

export function tickLimit(rulerPx: number): number {
  return Math.max(2, Math.floor(rulerPx / tickLabelPx))
}

export function niceTicks(
  durationMs: number,
  limit: number = mostTicks,
  originMs = 0
): number[] {
  if (!Number.isFinite(durationMs) || durationMs <= 0) return [0]
  const step = niceStep(durationMs, limit, originMs)
  if (step === null) return [0, durationMs]
  const { first, count } = stepIndexes(originMs, durationMs, step)
  return Array.from({ length: count }, (_, index) =>
    Math.max(0, (first + index) * step - originMs)
  )
}

export type TickAnchor = "start" | "middle" | "end"

export function tickAnchor(
  tickMs: number,
  durationMs: number,
  rulerPx: number
): TickAnchor {
  if (tickMs <= 0) return "start"
  if (tickMs >= durationMs) return "end"
  if (rulerPx <= 0) return "middle"
  const fromStartPx = (tickMs / durationMs) * rulerPx
  if (fromStartPx < labelHalfPx) return "start"
  if (rulerPx - fromStartPx < labelHalfPx) return "end"
  return "middle"
}
