import type { Selections } from "@/api/keys"
import {
  compareValues,
  fromSpecFilter,
  parseFilters,
  type AdhocFilter,
} from "@/dashboards/variable-controls/adhoc"
import type { Variable } from "@/spec"
import { compileAnchoredPattern } from "@/spec/patterns"

import { rowCount, type PreparedField, type PreparedFrame } from "./names"

export type AdhocScope = AdhocFilter[]

export type ReusedFilter = { scopes: AdhocScope[]; after: number }

export function adhocFiltersOf(
  variables: readonly Variable[],
  selections: Selections
): AdhocScope[] {
  return variables.flatMap((variable) =>
    variable.type === "adhoc"
      ? [
          (variable.baseFilters ?? []).map(fromSpecFilter),
          parseFilters(selections[variable.name] ?? []),
        ].filter((scope) => scope.length > 0)
      : []
  )
}

function cellIsEmpty(value: unknown): boolean {
  return value === null || value === undefined
}

function cellText(value: unknown): string {
  return cellIsEmpty(value) ? "" : String(value)
}

function matchesPattern(pattern: string, text: string): boolean {
  return compileAnchoredPattern(pattern)?.test(text) ?? false
}

function passes(filter: AdhocFilter, value: unknown): boolean {
  if ((filter.op === "<" || filter.op === ">") && cellIsEmpty(value)) {
    return false
  }
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
    case "<":
      return compareValues(text, filter.value) < 0
    case ">":
      return compareValues(text, filter.value) > 0
  }
}

type Applicable = { filter: AdhocFilter; field: PreparedField }

function splitGroups(scope: AdhocScope): AdhocFilter[][] {
  const groups: AdhocFilter[][] = []
  for (const filter of scope) {
    if (filter.condition === "or" || groups.length === 0) groups.push([])
    groups[groups.length - 1].push(filter)
  }
  return groups
}

function applicableGroups(
  frame: PreparedFrame,
  scope: AdhocScope
): Applicable[][] | null {
  const groups: Applicable[][] = []
  for (const group of splitGroups(scope)) {
    const applicable: Applicable[] = []
    for (const filter of group) {
      const field = frame.fields.find(
        (candidate) => candidate.name === filter.key
      )
      if (field) applicable.push({ filter, field })
    }
    if (applicable.length === 0) return null
    groups.push(applicable)
  }
  return groups
}

function filterFrame(
  frame: PreparedFrame,
  scopes: AdhocScope[]
): PreparedFrame {
  const applicable = scopes.flatMap((scope) => {
    const groups = applicableGroups(frame, scope)
    return groups ? [groups] : []
  })
  if (applicable.length === 0) return frame
  const keep: number[] = []
  const rows = rowCount(frame)
  for (let row = 0; row < rows; row++) {
    const passesScope = (groups: Applicable[][]) =>
      groups.some((group) =>
        group.every((a) => passes(a.filter, a.field.values[row]))
      )
    if (applicable.every(passesScope)) keep.push(row)
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
  scopes: AdhocScope[]
): PreparedFrame[] {
  if (scopes.length === 0) return frames
  return frames.map((frame) => filterFrame(frame, scopes))
}
