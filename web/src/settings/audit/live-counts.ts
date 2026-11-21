import type { AuditEvent } from "./stream"

export type FacetValue = { value: string; count: number }

export function liveFacetValues(
  values: readonly FacetValue[],
  live: readonly AuditEvent[],
  field: string
): FacetValue[] {
  if (live.length === 0) return [...values]
  const extra = new Map<string, number>()
  for (const event of live) {
    const value = (event as Record<string, unknown>)[field]
    if (typeof value !== "string" || value === "") continue
    extra.set(value, (extra.get(value) ?? 0) + 1)
  }
  return values.map((entry) => ({
    value: entry.value,
    count: entry.count + (extra.get(entry.value) ?? 0),
  }))
}
