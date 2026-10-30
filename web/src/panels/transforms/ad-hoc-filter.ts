import type { Selections } from "@/api/keys"
import {
  parseFilters,
  type AdhocFilter,
} from "@/dashboards/variable-controls/adhoc"
import type { Variable } from "@/spec"
import { compileAnchoredPattern } from "@/spec/patterns"

import { rowCount, type PreparedField, type PreparedFrame } from "./names"

export type ReusedFilter = { filters: AdhocFilter[]; after: number }

export function adhocFiltersOf(
  variables: readonly Variable[],
  selections: Selections
): AdhocFilter[] {
  return variables.flatMap((variable) =>
    variable.type === "adhoc"
      ? parseFilters(selections[variable.name] ?? [])
      : []
  )
}

function cellText(value: unknown): string {
  return value === null || value === undefined ? "" : String(value)
}

function matchesPattern(pattern: string, text: string): boolean {
  return compileAnchoredPattern(pattern)?.test(text) ?? false
}

function passes(filter: AdhocFilter, value: unknown): boolean {
  const text = cellText(value)
  switch (filter.op) {
    case "=":
      return text === filter.value
    case "!=":
      return text !== filter.value
    case "=~":
      return matchesPattern(filter.value, text)
    case "!~":
      return !matchesPattern(filter.value, text)
  }
}

type Applicable = { filter: AdhocFilter; field: PreparedField }

function applicableTo(
  frame: PreparedFrame,
  filters: AdhocFilter[]
): Applicable[] {
  const out: Applicable[] = []
  for (const filter of filters) {
    const field = frame.fields.find(
      (candidate) => candidate.name === filter.key
    )
    if (field) out.push({ filter, field })
  }
  return out
}

function filterFrame(
  frame: PreparedFrame,
  filters: AdhocFilter[]
): PreparedFrame {
  const applicable = applicableTo(frame, filters)
  if (applicable.length === 0) return frame
  const keep: number[] = []
  const rows = rowCount(frame)
  for (let row = 0; row < rows; row++) {
    if (applicable.every((a) => passes(a.filter, a.field.values[row]))) {
      keep.push(row)
    }
  }
  return {
    ...frame,
    fields: frame.fields.map((field) => ({
      ...field,
      values: keep.map((row) => field.values[row]),
    })),
  }
}

export function filterByAdhoc(
  frames: PreparedFrame[],
  filters: AdhocFilter[]
): PreparedFrame[] {
  if (filters.length === 0) return frames
  return frames.map((frame) => filterFrame(frame, filters))
}
