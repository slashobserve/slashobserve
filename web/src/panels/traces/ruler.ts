const stepMultipliers = [1, 2, 2.5, 5]
const fewestTicks = 4
const mostTicks = 6
const preferredTicks = 5
const floatSlack = 1e-9
const tickLabelPx = 64

function tickCount(durationMs: number, step: number): number {
  return Math.floor((durationMs / step) * (1 + floatSlack)) + 1
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

function niceStep(durationMs: number, limit: number): number | null {
  const most = Math.min(mostTicks, limit)
  const fewest = Math.min(fewestTicks, most)
  const preferred = Math.min(preferredTicks, most)
  let best: { step: number; distance: number } | null = null
  for (const step of candidateSteps(durationMs)) {
    const count = tickCount(durationMs, step)
    if (count < fewest || count > most) continue
    const distance = Math.abs(count - preferred)
    if (best === null || distance <= best.distance) best = { step, distance }
  }
  return best?.step ?? null
}

export function tickLimit(rulerPx: number): number {
  return Math.max(2, Math.floor(rulerPx / tickLabelPx))
}

export function niceTicks(
  durationMs: number,
  limit: number = mostTicks
): number[] {
  if (!Number.isFinite(durationMs) || durationMs <= 0) return [0]
  const step = niceStep(durationMs, limit)
  if (step === null) return [0, durationMs]
  return Array.from(
    { length: tickCount(durationMs, step) },
    (_, index) => index * step
  )
}
