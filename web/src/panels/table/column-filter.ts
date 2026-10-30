import type { TableRow } from "./table-model"

export const maxFilterValues = 200

export type ColumnFilters = ReadonlyMap<string, ReadonlySet<string>>

function compareText(a: string, b: string): number {
  return a.localeCompare(b, "en", { numeric: true })
}

export function distinctValues(
  rows: TableRow[],
  textOf: (row: TableRow) => string
): { values: string[]; total: number } {
  const all = [...new Set(rows.map(textOf))].sort(compareText)
  return { values: all.slice(0, maxFilterValues), total: all.length }
}

export function filterRows(
  rows: TableRow[],
  filters: ColumnFilters,
  textOf: (key: string, row: TableRow) => string
): TableRow[] {
  const active = [...filters].filter(([, kept]) => kept.size > 0)
  if (active.length === 0) return rows
  return rows.filter((row) =>
    active.every(([key, kept]) => kept.has(textOf(key, row)))
  )
}

export function toggleValue(
  filters: ColumnFilters,
  key: string,
  value: string
): ColumnFilters {
  const next = new Map(filters)
  const kept = new Set(filters.get(key))
  if (kept.has(value)) kept.delete(value)
  else kept.add(value)
  if (kept.size === 0) next.delete(key)
  else next.set(key, kept)
  return next
}

export function clearFilter(
  filters: ColumnFilters,
  key: string
): ColumnFilters {
  const next = new Map(filters)
  next.delete(key)
  return next
}
