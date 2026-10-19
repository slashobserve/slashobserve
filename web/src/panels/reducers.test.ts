import { describe, expect, it } from "vitest"

import type { Reducer } from "@/spec"

import { reduce, reduceField, reducerLabels } from "./reducers"

const values = [1, null, 3, 2]

describe("reduce", () => {
  const expected: Record<Reducer, number | null> = {
    last: 2,
    lastNotNull: 2,
    first: 1,
    firstNotNull: 1,
    min: 1,
    max: 3,
    mean: 2,
    median: 2,
    sum: 6,
    count: 3,
    range: 2,
    delta: 2,
    diff: 1,
    stdDev: Math.sqrt(2 / 3),
    distinctCount: 3,
    changeCount: 2,
  }

  it.each(Object.entries(expected))("reduces %s", (reducer, want) => {
    expect(reduce(values, reducer as Reducer)).toBeCloseTo(want ?? 0, 10)
  })

  it("returns 0 for count and null for every other reducer on empty input", () => {
    for (const reducer of Object.keys(reducerLabels) as Reducer[]) {
      expect(reduce([], reducer)).toBe(reducer === "count" ? 0 : null)
    }
  })

  it("keeps a null edge for last and first but skips it for the NotNull forms", () => {
    expect(reduce([1, 2, null], "last")).toBeNull()
    expect(reduce([1, 2, null], "lastNotNull")).toBe(2)
    expect(reduce([null, 1, 2], "first")).toBeNull()
    expect(reduce([null, 1, 2], "firstNotNull")).toBe(1)
  })

  it("returns null for numeric reducers when only nulls remain", () => {
    expect(reduce([null, null], "mean")).toBeNull()
    expect(reduce([null, null], "max")).toBeNull()
    expect(reduce([null, null], "count")).toBe(0)
  })

  it("takes the mean of the two middle values as the median", () => {
    expect(reduce([4, 1, 3, 2], "median")).toBe(2.5)
  })

  it("sums the increases for delta and skips counter resets", () => {
    expect(reduce([10, 5, 3], "delta")).toBe(0)
    expect(reduce([5, 10, 2, 4], "delta")).toBe(9)
    expect(reduce([1, 2, 3], "delta")).toBe(2)
    expect(reduce([7], "delta")).toBe(0)
    expect(reduce([null, null], "delta")).toBeNull()
  })

  it("returns null for an unknown reducer id", () => {
    expect(reduce([1, 2], "nope" as Reducer)).toBeNull()
    expect(reduce([], "nope" as Reducer)).toBeNull()
  })

  it("computes the population standard deviation", () => {
    expect(reduce([2, 4, 4, 4, 5, 5, 7, 9], "stdDev")).toBe(2)
  })

  it("counts changes and distinct values over non-null values", () => {
    expect(reduce([1, 1, null, 2, 1], "changeCount")).toBe(2)
    expect(reduce([1, 1, null, 2, 1], "distinctCount")).toBe(2)
  })

  it("handles large arrays", () => {
    const big = Array.from({ length: 300_000 }, (_, index) => index)
    expect(reduce(big, "max")).toBe(299_999)
    expect(reduce(big, "min")).toBe(0)
  })

  it("reduces a field's values", () => {
    expect(
      reduceField({ name: "v", type: "number", values: [1, 2, 3] }, "sum")
    ).toBe(6)
  })

  it("labels every reducer", () => {
    expect(Object.values(reducerLabels)).toEqual([
      "Last",
      "Last *",
      "First",
      "First *",
      "Min",
      "Max",
      "Mean",
      "Median",
      "Total",
      "Count",
      "Range",
      "Delta",
      "Difference",
      "StdDev",
      "Distinct count",
      "Change count",
    ])
  })
})
