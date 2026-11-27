import { signedInPrincipalId } from "@/lib/auth"

const unscopedKeys = [
  "slashobserve-recent-dashboards",
  "slashobserve-recent-observations",
  "slashobserve-prompt-history",
  "slashobserve-ask-history-logs",
  "slashobserve-ask-history-traces",
  "slashobserve-ask-history-metrics",
]

let cleared = false

export function clearUnscopedHistory() {
  try {
    for (const key of unscopedKeys) localStorage.removeItem(key)
  } catch {
    return
  }
}

export function scopedKey(base: string): string | null {
  const id = signedInPrincipalId()
  if (id === undefined) return null
  if (!cleared) {
    cleared = true
    clearUnscopedHistory()
  }
  return `${base}:${id}`
}
