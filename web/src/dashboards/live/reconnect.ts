const minBackoffMs = 1_000
const maxBackoffMs = 30_000
const checkEveryFailures = 3

export const reconnectingGraceMs = 2_000

export function backoffDelay(
  attempt: number,
  random: () => number = Math.random
): number {
  const base = Math.min(minBackoffMs * 2 ** attempt, maxBackoffMs)
  return base / 2 + random() * (base / 2)
}

export function shouldCheckDashboard(failures: number): boolean {
  return failures > 0 && failures % checkEveryFailures === 0
}
