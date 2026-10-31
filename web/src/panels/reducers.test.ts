import { describe, expect, it } from "vitest"

import type { AnyReducer, Reducer } from "@/spec"

import {
  isCountReducer,
  isReducer,
  reduce,
  reduceAny,
  reduceField,
  reducerKind,
  reducerLabel,
} from "./reducers"

const values = [1, null, 3, 2]

type Percentile = Extract<Reducer, `p${number}`>

const percentiles = Object.fromEntries(
  Array.from({ length: 99 }, (_, index) => [
    `p${index + 1}`,
    1 + 0.02 * (index + 1),
  ])
) as Record<Percentile, number>

const nonNumber: AnyReducer[] = [
  "allIsNull",
  "allIsZero",
  "allValues",
  "uniqueValues",
]

describe("reduce", () => {
  const expected: Record<Reducer, number | null> = {
    ...percentiles,
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
    variance: 2 / 3,
    step: -1,
    diffperc: 1,
    logmin: 1,
    countAll: 4,
  }

  it.each(Object.entries(expected))("reduces %s", (reducer, want) => {
    expect(reduce(values, reducer as Reducer)).toBeCloseTo(want ?? 0, 10)
  })

  it("returns 0 for count and countAll and null for every other reducer on empty input", () => {
    for (const reducer of Object.keys(expected) as Reducer[]) {
      const counts = reducer === "count" || reducer === "countAll"
      expect(reduce([], reducer)).toBe(counts ? 0 : null)
    }
  })

  it("returns null for every reducer that needs numbers when only nulls and text remain", () => {
    for (const reducer of Object.keys(expected) as Reducer[]) {
      if (isCountReducer(reducer)) continue
      expect(reduce([null, "a"], reducer)).toBeNull()
    }
  })

  it("counts every value for countAll, nulls included", () => {
    expect(reduce([null, 1, undefined, "a"], "countAll")).toBe(4)
    expect(reduce([null, 1, undefined, "a"], "count")).toBe(2)
  })

  it("takes the population variance as the square of stdDev", () => {
    expect(reduce([2, 4, 4, 4, 5, 5, 7, 9], "variance")).toBe(4)
    expect(reduce([5], "variance")).toBe(0)
  })

  it("takes the smallest difference between consecutive numbers as step", () => {
    expect(reduce([5, 3, 1], "step")).toBe(-2)
    expect(reduce([1, null, 4, 5], "step")).toBe(1)
    expect(reduce([7], "step")).toBeNull()
  })

  it("takes the change as a fraction of the first number for diffperc", () => {
    expect(reduce([2, 3, 5], "diffperc")).toBe(1.5)
    expect(reduce([0, 3, 5], "diffperc")).toBeNull()
  })

  it("takes the smallest number above zero for logmin", () => {
    expect(reduce([0, 4, -2, 3], "logmin")).toBe(3)
    expect(reduce([0, -1, -5], "logmin")).toBeNull()
  })

  it("matches the median with p50 on even and odd lengths", () => {
    for (const sample of [[4, 1, 3, 2], [9, 1, 5], [7], [2, 8]]) {
      expect(reduce(sample, "p50")).toBe(reduce(sample, "median"))
    }
  })

  it("interpolates percentiles on a 100 value ramp", () => {
    const ramp = Array.from({ length: 100 }, (_, index) => index + 1)
    expect(reduce(ramp, "p1")).toBeCloseTo(1.99, 10)
    expect(reduce(ramp, "p99")).toBeCloseTo(99.01, 10)
    expect(reduce([5], "p95")).toBe(5)
  })

  it("returns null for a percentile outside p1 to p99", () => {
    expect(reduce([1, 2], "p0" as Reducer)).toBeNull()
    expect(reduce([1, 2], "p100" as Reducer)).toBeNull()
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

  it("labels the original reducers as before", () => {
    const original = [
      ["last", "Last"],
      ["lastNotNull", "Last *"],
      ["first", "First"],
      ["firstNotNull", "First *"],
      ["min", "Min"],
      ["max", "Max"],
      ["mean", "Mean"],
      ["median", "Median"],
      ["sum", "Total"],
      ["count", "Count"],
      ["range", "Range"],
      ["delta", "Delta"],
      ["diff", "Difference"],
      ["stdDev", "StdDev"],
      ["distinctCount", "Distinct count"],
      ["changeCount", "Change count"],
    ] as const
    for (const [reducer, label] of original) {
      expect(reducerLabel(reducer)).toBe(label)
    }
  })

  it("labels the new reducers", () => {
    expect(reducerLabel("variance")).toBe("Variance")
    expect(reducerLabel("step")).toBe("Step")
    expect(reducerLabel("diffperc")).toBe("Difference percent")
    expect(reducerLabel("logmin")).toBe("Min (above zero)")
    expect(reducerLabel("countAll")).toBe("Count all")
    expect(reducerLabel("allIsNull")).toBe("All nulls")
    expect(reducerLabel("allIsZero")).toBe("All zeros")
    expect(reducerLabel("allValues")).toBe("All values")
    expect(reducerLabel("uniqueValues")).toBe("All unique values")
  })

  it("labels percentiles with the English ordinal", () => {
    const labels: [Reducer, string][] = [
      ["p1", "1st %"],
      ["p2", "2nd %"],
      ["p3", "3rd %"],
      ["p4", "4th %"],
      ["p11", "11th %"],
      ["p12", "12th %"],
      ["p13", "13th %"],
      ["p21", "21st %"],
      ["p22", "22nd %"],
      ["p23", "23rd %"],
      ["p95", "95th %"],
      ["p99", "99th %"],
    ]
    for (const [reducer, label] of labels) {
      expect(reducerLabel(reducer)).toBe(label)
    }
  })

  it("knows which ids are reducers", () => {
    expect(isReducer("p99")).toBe(true)
    expect(isReducer("countAll")).toBe(true)
    expect(isReducer("p100")).toBe(false)
    expect(isReducer("p0")).toBe(false)
    expect(isReducer("uniqueValues")).toBe(false)
    expect(isReducer(7)).toBe(false)
  })
})

describe("reduceAny", () => {
  it("answers true or false for allIsNull", () => {
    expect(reduceAny([], "allIsNull")).toBe(true)
    expect(reduceAny([null, undefined], "allIsNull")).toBe(true)
    expect(reduceAny([null, 0], "allIsNull")).toBe(false)
  })

  it("answers true for allIsZero only when a value is present and every one is 0", () => {
    expect(reduceAny([], "allIsZero")).toBe(false)
    expect(reduceAny([null, null], "allIsZero")).toBe(false)
    expect(reduceAny([0, null, 0], "allIsZero")).toBe(true)
    expect(reduceAny([0, 1], "allIsZero")).toBe(false)
    expect(reduceAny(["0"], "allIsZero")).toBe(false)
  })

  it("lists every present value in order for allValues", () => {
    expect(reduceAny([2, null, 1, 2, "x"], "allValues")).toBe("2, 1, 2, x")
    expect(reduceAny([], "allValues")).toBeNull()
    expect(reduceAny([null], "allValues")).toBeNull()
  })

  it("lists the distinct values in order of first appearance as text for uniqueValues", () => {
    expect(
      reduceAny(["b", "a", "b", null, undefined, "c"], "uniqueValues")
    ).toBe("b, a, c")
    expect(reduceAny([2, 1, 2], "uniqueValues")).toBe("2, 1")
    expect(reduceAny([], "uniqueValues")).toBeNull()
    expect(reduceAny([null, undefined], "uniqueValues")).toBeNull()
  })

  it("reduces numbers like reduce for every other reducer", () => {
    expect(reduceAny(values, "sum")).toBe(6)
    expect(reduceAny(values, "p50")).toBe(2)
    expect(reduceAny([], "count")).toBe(0)
  })
})

describe("reducerKind and isCountReducer", () => {
  it("decides the result kind of the four non-number reducers", () => {
    expect(reducerKind("allIsNull")).toBe("bool")
    expect(reducerKind("allIsZero")).toBe("bool")
    expect(reducerKind("allValues")).toBe("text")
    expect(reducerKind("uniqueValues")).toBe("text")
  })

  it("gives number for every Reducer", () => {
    for (const reducer of [
      "last",
      "countAll",
      "variance",
      "p1",
      "p99",
    ] as const) {
      expect(reducerKind(reducer)).toBe("number")
    }
  })

  it("flags the count reducers only", () => {
    const counts = ["count", "countAll", "distinctCount", "changeCount"]
    for (const reducer of [
      ...counts,
      "sum",
      "p50",
      ...nonNumber,
    ] as AnyReducer[]) {
      expect(isCountReducer(reducer)).toBe(counts.includes(reducer))
    }
  })
})
