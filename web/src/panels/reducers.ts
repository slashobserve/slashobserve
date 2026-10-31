import type { AnyReducer, Reducer } from "@/spec"

import type { WireField } from "./frames"

export type Reduced = number | boolean | string | null
export type ReducedKind = "number" | "bool" | "text"

type Percentile = Extract<Reducer, `p${number}`>
type NamedReducer = Exclude<Reducer, Percentile>
type NonNumberReducer = Exclude<AnyReducer, Reducer>

const namedLabels: Record<NamedReducer, string> = {
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
  variance: "Variance",
  step: "Step",
  diffperc: "Difference percent",
  logmin: "Min (above zero)",
  countAll: "Count all",
}

const nonNumberReducers: Record<
  NonNumberReducer,
  { label: string; kind: Exclude<ReducedKind, "number"> }
> = {
  allIsNull: { label: "All nulls", kind: "bool" },
  allIsZero: { label: "All zeros", kind: "bool" },
  allValues: { label: "All values", kind: "text" },
  uniqueValues: { label: "All unique values", kind: "text" },
}

const countingReducers: ReadonlySet<AnyReducer> = new Set([
  "count",
  "countAll",
  "distinctCount",
  "changeCount",
])

const percentilePattern = /^p([1-9][0-9]?)$/

function isPercentile(reducer: unknown): reducer is Percentile {
  return typeof reducer === "string" && percentilePattern.test(reducer)
}

function percentRank(reducer: Percentile): number {
  return Number(reducer.slice(1))
}

function isNonNumber(reducer: unknown): reducer is NonNumberReducer {
  return (
    typeof reducer === "string" && Object.hasOwn(nonNumberReducers, reducer)
  )
}

export function isReducer(value: unknown): value is Reducer {
  return (
    isPercentile(value) ||
    (typeof value === "string" && Object.hasOwn(namedLabels, value))
  )
}

function ordinal(rank: number): string {
  const lastTwo = rank % 100
  if (lastTwo >= 11 && lastTwo <= 13) return `${rank}th`
  switch (rank % 10) {
    case 1:
      return `${rank}st`
    case 2:
      return `${rank}nd`
    case 3:
      return `${rank}rd`
    default:
      return `${rank}th`
  }
}

export function reducerLabel(reducer: AnyReducer): string {
  if (isNonNumber(reducer)) return nonNumberReducers[reducer].label
  if (isPercentile(reducer)) return `${ordinal(percentRank(reducer))} %`
  return namedLabels[reducer]
}

export function reducerKind(reducer: AnyReducer): ReducedKind {
  return isNonNumber(reducer) ? nonNumberReducers[reducer].kind : "number"
}

export function isCountReducer(reducer: AnyReducer): boolean {
  return countingReducers.has(reducer)
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

function percentile(numbers: number[], rank: number): number {
  const sorted = [...numbers].sort((a, b) => a - b)
  const position = ((sorted.length - 1) * rank) / 100
  const below = Math.floor(position)
  if (below + 1 >= sorted.length) return sorted[below]
  return (
    sorted[below] + (position - below) * (sorted[below + 1] - sorted[below])
  )
}

function variance(numbers: number[]): number {
  const mean = sum(numbers) / numbers.length
  return sum(numbers.map((value) => (value - mean) ** 2)) / numbers.length
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

function smallestStep(numbers: number[]): number | null {
  if (numbers.length < 2) return null
  let smallest = Infinity
  for (let i = 1; i < numbers.length; i++) {
    smallest = Math.min(smallest, numbers[i] - numbers[i - 1])
  }
  return smallest
}

function differencePercent(numbers: number[]): number | null {
  const first = numbers[0]
  return first === 0 ? null : (numbers[numbers.length - 1] - first) / first
}

function smallestAboveZero(numbers: number[]): number | null {
  const positive = numbers.filter((value) => value > 0)
  return positive.length > 0 ? min(positive) : null
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
  if (values.length === 0) {
    return reducer === "count" || reducer === "countAll" ? 0 : null
  }
  const present = values.filter(isPresent)
  const numbers = values.filter(isNumber)
  if (isPercentile(reducer)) {
    return numbers.length > 0 ? percentile(numbers, percentRank(reducer)) : null
  }
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
    case "countAll":
      return values.length
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
      return Math.sqrt(variance(numbers))
    case "variance":
      return variance(numbers)
    case "step":
      return smallestStep(numbers)
    case "diffperc":
      return differencePercent(numbers)
    case "logmin":
      return smallestAboveZero(numbers)
    default: {
      const unreachable: never = reducer
      void unreachable
      return null
    }
  }
}

function joined(values: readonly unknown[]): string | null {
  return values.length > 0 ? values.map(String).join(", ") : null
}

export function reduceAny(
  values: readonly unknown[],
  reducer: AnyReducer
): Reduced {
  const present = values.filter(isPresent)
  switch (reducer) {
    case "allIsNull":
      return present.length === 0
    case "allIsZero":
      return present.length > 0 && present.every((value) => value === 0)
    case "allValues":
      return joined(present)
    case "uniqueValues":
      return joined([...new Set(present)])
    default:
      return reduce(values, reducer)
  }
}

export function reduceField(field: WireField, reducer: Reducer): number | null {
  return reduce(field.values, reducer)
}
