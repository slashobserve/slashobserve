import type { Reducer } from "@/spec"

import { isNumber, reduce, reducerLabels } from "../reducers"

export type LegendMode = "list" | "table"

export type LegendOptions = {
  mode: LegendMode
  values: Reducer[]
  sortBy?: "name" | Reducer
  sortDesc: boolean
  hideZero: boolean
  hideEmpty: boolean
}

export type LegendSource = {
  key: string
  label: string
  values: readonly unknown[]
  hidden?: boolean
}

export function effectiveLegendMode(
  legendMode: LegendMode | undefined,
  legendValues: readonly Reducer[]
): LegendMode {
  if (legendMode) return legendMode
  return legendValues.length > 0 ? "table" : "list"
}

function isEmpty(source: LegendSource): boolean {
  return !source.values.some(isNumber)
}

function isAllZero(source: LegendSource): boolean {
  return source.values.every(
    (value) => value === 0 || value === null || value === undefined
  )
}

const collator = new Intl.Collator(undefined, { numeric: true })

type Entry = { index: number; source: LegendSource }

function byName(sortDesc: boolean) {
  const direction = sortDesc ? -1 : 1
  return (a: Entry, b: Entry) =>
    direction * collator.compare(a.source.label, b.source.label) ||
    a.index - b.index
}

function byReducer(reducer: Reducer, sortDesc: boolean) {
  const direction = sortDesc ? -1 : 1
  return (a: Entry, b: Entry) => {
    const left = reduce(a.source.values, reducer)
    const right = reduce(b.source.values, reducer)
    if (left === null || right === null) {
      if (left === right) return a.index - b.index
      return left === null ? 1 : -1
    }
    return direction * (left - right) || a.index - b.index
  }
}

export function legendOrder(
  sources: readonly LegendSource[],
  options: LegendOptions
): number[] {
  const shown = sources
    .map((source, index): Entry => ({ source, index }))
    .filter(
      ({ source }) =>
        !source.hidden &&
        !(options.hideEmpty && isEmpty(source)) &&
        !(options.hideZero && isAllZero(source))
    )
  const { sortBy, sortDesc } = options
  if (sortBy === "name") shown.sort(byName(sortDesc))
  else if (sortBy !== undefined) shown.sort(byReducer(sortBy, sortDesc))
  return shown.map(({ index }) => index)
}

export function legendHeaders(
  columns: readonly string[],
  headers?: readonly string[]
): readonly string[] {
  return headers ?? columns.map((column) => reducerLabels[column as Reducer])
}

export function inlineValues(
  columns: readonly string[],
  cells: readonly string[],
  headers?: readonly string[]
): string {
  return legendHeaders(columns, headers)
    .map((header, column) => `${header} ${cells[column]}`.trim())
    .join("  ")
}
