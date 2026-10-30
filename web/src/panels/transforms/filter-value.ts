import { compileNamePattern, type Transform } from "@/spec"

import {
  copyFrame,
  matchesName,
  rowCount,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type FilterValueOptions = Extract<Transform, { type: "filterByValue" }>
type ValueFilter = FilterValueOptions["filters"][number]
type RowTest = (row: number) => boolean

function numeric(value: unknown): number {
  if (typeof value === "number") return value
  if (typeof value !== "string" || value.trim() === "") return NaN
  return Number(value)
}

function isEmpty(value: unknown): boolean {
  return value === null || value === undefined
}

function sameValue(cell: unknown, wanted: unknown): boolean {
  if (typeof cell === "number" && typeof wanted === "number") {
    return cell === wanted
  }
  return String(cell) === String(wanted)
}

function compare(
  cell: unknown,
  wanted: unknown,
  test: (cell: number, wanted: number) => boolean
): boolean {
  const left = numeric(cell)
  const right = numeric(wanted)
  return !Number.isNaN(left) && !Number.isNaN(right) && test(left, right)
}

function inRange(cell: unknown, from?: number, to?: number): boolean {
  const value = numeric(cell)
  if (Number.isNaN(value)) return false
  return (
    (from === undefined || value >= from) && (to === undefined || value <= to)
  )
}

function cellTest(filter: ValueFilter): (cell: unknown) => boolean {
  const { op, value } = filter
  switch (op) {
    case "gt":
      return (cell) => compare(cell, value, (a, b) => a > b)
    case "gte":
      return (cell) => compare(cell, value, (a, b) => a >= b)
    case "lt":
      return (cell) => compare(cell, value, (a, b) => a < b)
    case "lte":
      return (cell) => compare(cell, value, (a, b) => a <= b)
    case "eq":
      return (cell) => !isEmpty(cell) && sameValue(cell, value)
    case "neq":
      return (cell) => isEmpty(cell) || !sameValue(cell, value)
    case "null":
      return isEmpty
    case "notNull":
      return (cell) => !isEmpty(cell)
    case "regex": {
      const pattern = compileNamePattern(String(value ?? ""))
      return (cell) => !isEmpty(cell) && (pattern?.test(String(cell)) ?? false)
    }
    case "range":
      return (cell) => inRange(cell, filter.from, filter.to)
  }
}

function rowTests(frame: PreparedFrame, filters: ValueFilter[]): RowTest[] {
  return filters.flatMap((filter) => {
    const field: PreparedField | undefined = frame.fields.find((candidate) =>
      matchesName(frame, candidate, filter.field)
    )
    if (field === undefined) return []
    const test = cellTest(filter)
    return [(row: number) => test(field.values[row] ?? null)]
  })
}

function filterFrame(
  frame: PreparedFrame,
  { keep = "include", match = "any", filters }: FilterValueOptions
): PreparedFrame {
  const tests = rowTests(frame, filters)
  if (tests.length === 0) return copyFrame(frame)
  const rows: number[] = []
  for (let row = 0; row < rowCount(frame); row++) {
    const hit =
      match === "all"
        ? tests.every((test) => test(row))
        : tests.some((test) => test(row))
    if (hit === (keep === "include")) rows.push(row)
  }
  return withFields(
    frame,
    frame.fields.map((field) => ({
      ...field,
      values: rows.map((row) => field.values[row] ?? null),
    }))
  )
}

export function filterByValue(
  frames: PreparedFrame[],
  options: FilterValueOptions
): PreparedFrame[] {
  return frames.map((frame) => filterFrame(frame, options))
}
