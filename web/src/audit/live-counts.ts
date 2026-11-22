import type { AuditEvent } from "./stream"

export type FacetValue = { value: string; count: number }

export function liveFacetValues(
  values: readonly FacetValue[],
  live: readonly AuditEvent[],
  field: string,
  limit: number
): FacetValue[] {
  if (live.length === 0) return [...values]
  const counts = new Map(values.map((entry) => [entry.value, entry.count]))
  for (const event of live) {
    const value = (event as Record<string, unknown>)[field]
    if (typeof value !== "string" || value === "") continue
    counts.set(value, (counts.get(value) ?? 0) + 1)
  }
  return [...counts]
    .map(([value, count]) => ({ value, count }))
    .sort((a, b) => b.count - a.count || a.value.localeCompare(b.value))
    .slice(0, limit)
}
