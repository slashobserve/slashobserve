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
