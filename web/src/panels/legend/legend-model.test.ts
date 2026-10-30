import { describe, expect, it } from "vitest"

import {
  effectiveLegendMode,
  inlineValues,
  legendOrder,
  type LegendOptions,
  type LegendSource,
} from "./legend-model"

const plain: LegendOptions = {
  mode: "list",
  values: [],
  sortDesc: false,
  hideZero: false,
  hideEmpty: false,
}

function source(label: string, values: unknown[]): LegendSource {
  return { key: label, label, values }
}

const sources = [
  source("b", [4, 6]),
  source("a", [1, 2]),
  source("gap", [null, null]),
  source("c", [9, 9]),
]

describe("effectiveLegendMode", () => {
  it.each([
    [undefined, [], "list"],
    [undefined, ["mean"], "table"],
    ["list", ["mean"], "list"],
    ["table", [], "table"],
  ] as const)(
    "legendMode %s with values %j is %s",
    (mode, values, expected) => {
      expect(effectiveLegendMode(mode, [...values])).toBe(expected)
    }
  )
})

describe("legendOrder", () => {
  it("keeps source order without options", () => {
    expect(legendOrder(sources, plain)).toEqual([0, 1, 2, 3])
  })

  it("sorts by a reducer ascending with an empty series last", () => {
    expect(legendOrder(sources, { ...plain, sortBy: "mean" })).toEqual([
      1, 0, 3, 2,
    ])
  })

  it("sorts by a reducer descending with an empty series still last", () => {
    expect(
      legendOrder(sources, { ...plain, sortBy: "mean", sortDesc: true })
    ).toEqual([3, 0, 1, 2])
  })

  it("sorts by name ascending and descending", () => {
    expect(legendOrder(sources, { ...plain, sortBy: "name" })).toEqual([
      1, 0, 3, 2,
    ])
    expect(
      legendOrder(sources, { ...plain, sortBy: "name", sortDesc: true })
    ).toEqual([2, 3, 0, 1])
  })

  it("keeps source order for equal keys in both directions", () => {
    const ties = [source("x", [5]), source("y", [5]), source("z", [5])]
    expect(legendOrder(ties, { ...plain, sortBy: "max" })).toEqual([0, 1, 2])
    expect(
      legendOrder(ties, { ...plain, sortBy: "max", sortDesc: true })
    ).toEqual([0, 1, 2])
  })

  it("hides all-zero and all-empty series with hideZero but keeps a series with one non-zero value", () => {
    const zeros = [
      source("zero", [0, 0]),
      source("empty", [null, undefined]),
      source("some", [0, 3]),
    ]
    expect(legendOrder(zeros, { ...plain, hideZero: true })).toEqual([2])
  })

  it("hides only all-empty series with hideEmpty", () => {
    const mixed = [
      source("zero", [0, 0]),
      source("empty", [null, null]),
      source("some", [0, 3]),
    ]
    expect(legendOrder(mixed, { ...plain, hideEmpty: true })).toEqual([0, 2])
  })

  it("leaves out series marked hidden whatever their values", () => {
    const marked = [
      source("shown", [1, 2]),
      { ...source("marked", [3, 4]), hidden: true },
      { ...source("kept", [5, 6]), hidden: false },
    ]
    expect(legendOrder(marked, plain)).toEqual([0, 2])
  })

  it("sorts only the series that stay shown", () => {
    expect(
      legendOrder(sources, { ...plain, sortBy: "max", hideEmpty: true })
    ).toEqual([1, 0, 3])
  })
})

describe("inlineValues", () => {
  it("joins each reducer label and its cell", () => {
    expect(inlineValues(["mean", "max"], ["1.2", "3"])).toBe("Mean 1.2  Max 3")
  })
})
