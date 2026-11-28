import type { AnyReducer, Reducer } from "@/spec"

import { isNumber, reduce, reducerLabel } from "../reducers"

export type LegendMode = "list" | "table"

export type LegendOptions = {
  mode: LegendMode
  values: AnyReducer[]
  sortBy?: "name" | Reducer | undefined
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

export const maxStateLegendEntries = 12

const boundary = /[\s_\-:/.=,|(]/
const minPrefix = 4

export type NameRoom = { px: number; measure: (text: string) => number }

function sharedPrefixCut(labels: readonly string[]): number {
  let [common] = labels
  if (common === undefined) return 0
  for (const label of labels) {
    let at = 0
    while (
      at < common.length &&
      at < label.length &&
      common[at] === label[at]
    ) {
      at++
    }
    common = common.slice(0, at)
  }
  let cut = common.length
  while (cut > 0 && !boundary.test(common.charAt(cut - 1))) cut--
  if (cut < minPrefix || labels.some((label) => label.length <= cut)) return 0
  return cut
}

export function legendNames(
  labels: readonly string[],
  room?: NameRoom
): string[] {
  if (!room || labels.length < 2) return [...labels]
  if (labels.every((label) => room.measure(label) <= room.px)) {
    return [...labels]
  }
  const cut = sharedPrefixCut(labels)
  return labels.map((label) => label.slice(cut))
}

export function capLegend<T>(
  items: readonly T[],
  max: number
): { shown: T[]; more: number } {
  return {
    shown: items.slice(0, max),
    more: Math.max(0, items.length - max),
  }
}

export function effectiveLegendMode(
  legendMode: LegendMode | undefined,
  legendValues: readonly AnyReducer[]
): LegendMode {
  if (legendMode) return legendMode
  return legendValues.length > 0 ? "table" : "list"
}

export function legendWorthShowing(
  seriesCount: number,
  legendValues: readonly AnyReducer[]
): boolean {
  return seriesCount > 1 || (seriesCount === 1 && legendValues.length > 0)
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
  return headers ?? columns.map((column) => reducerLabel(column as AnyReducer))
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
