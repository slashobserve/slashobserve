const stepMultipliers = [1, 2, 2.5, 5]
const fewestTicks = 4
const mostTicks = 6
const preferredTicks = 5
const floatSlack = 1e-9

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

function niceStep(durationMs: number): number | null {
  let best: { step: number; distance: number } | null = null
  for (const step of candidateSteps(durationMs)) {
    const count = tickCount(durationMs, step)
    if (count < fewestTicks || count > mostTicks) continue
    const distance = Math.abs(count - preferredTicks)
    if (best === null || distance <= best.distance) best = { step, distance }
  }
  return best?.step ?? null
}

export function niceTicks(durationMs: number): number[] {
  if (!Number.isFinite(durationMs) || durationMs <= 0) return [0]
  const step = niceStep(durationMs)
  if (step === null) return [0, durationMs]
  return Array.from(
    { length: tickCount(durationMs, step) },
    (_, index) => index * step
  )
}
