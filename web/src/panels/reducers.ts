import type { Reducer } from "@/spec"

import type { WireField } from "./frames"

export const reducerLabels: Record<Reducer, string> = {
  last: "Last",
  lastNotNull: "Last *",
  first: "First",
  firstNotNull: "First *",
  min: "Min",
  max: "Max",
  mean: "Mean",
  median: "Median",
  sum: "Total",
  count: "Count",
  range: "Range",
  delta: "Delta",
  diff: "Difference",
  stdDev: "StdDev",
  distinctCount: "Distinct count",
  changeCount: "Change count",
}

function isPresent(value: unknown): boolean {
  return value !== null && value !== undefined
}

export function isNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value)
}

function asNumber(value: unknown): number | null {
  return isNumber(value) ? value : null
}

function sum(numbers: number[]): number {
  return numbers.reduce((total, value) => total + value, 0)
}

function min(numbers: number[]): number {
  return numbers.reduce((lowest, value) => Math.min(lowest, value))
}

function max(numbers: number[]): number {
  return numbers.reduce((highest, value) => Math.max(highest, value))
}

function median(numbers: number[]): number {
  const sorted = [...numbers].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 1
    ? sorted[middle]
    : (sorted[middle - 1] + sorted[middle]) / 2
}

function stdDev(numbers: number[]): number {
  const mean = sum(numbers) / numbers.length
  const variance =
    sum(numbers.map((value) => (value - mean) ** 2)) / numbers.length
  return Math.sqrt(variance)
}

function delta(numbers: number[]): number {
  let total = 0
  let previousUp = true
  for (let i = 1; i < numbers.length; i++) {
    const previous = numbers[i - 1]
    const value = numbers[i]
    if (previous > value) {
      previousUp = false
      continue
    }
    total += previousUp ? value - previous : value
    previousUp = true
  }
  return total
}

function changeCount(present: unknown[]): number {
  let changes = 0
  for (let i = 1; i < present.length; i++) {
    if (present[i] !== present[i - 1]) changes++
  }
  return changes
}

export function reduce(
  values: readonly unknown[],
  reducer: Reducer
): number | null {
  if (values.length === 0) return reducer === "count" ? 0 : null
  const present = values.filter(isPresent)
  const numbers = values.filter(isNumber)
  switch (reducer) {
    case "last":
      return asNumber(values[values.length - 1])
    case "first":
      return asNumber(values[0])
    case "lastNotNull":
      return numbers.length > 0 ? numbers[numbers.length - 1] : null
    case "firstNotNull":
      return numbers.length > 0 ? numbers[0] : null
    case "count":
      return present.length
    case "distinctCount":
      return new Set(present).size
    case "changeCount":
      return changeCount(present)
  }
  if (numbers.length === 0) return null
  switch (reducer) {
    case "min":
      return min(numbers)
    case "max":
      return max(numbers)
    case "mean":
      return sum(numbers) / numbers.length
    case "median":
      return median(numbers)
    case "sum":
      return sum(numbers)
    case "range":
      return max(numbers) - min(numbers)
    case "delta":
      return delta(numbers)
    case "diff":
      return numbers[numbers.length - 1] - numbers[0]
    case "stdDev":
      return stdDev(numbers)
    default: {
      const unreachable: never = reducer
      void unreachable
      return null
    }
  }
}

export function uniqueValues(values: readonly unknown[]): string | null {
  const distinct = [...new Set(values.filter(isPresent))]
  return distinct.length > 0 ? distinct.map(String).join(", ") : null
}

export function reduceField(field: WireField, reducer: Reducer): number | null {
  return reduce(field.values, reducer)
}
