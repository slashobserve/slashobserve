import { describe, expect, it } from "vitest"

import { anyReducerIds, reducerIds } from "@/panels/reducers"

import { reducerChoices, reducerMatches, reducerPools } from "./reducer-choices"

const legend = [
  { value: "lastNotNull", label: "Last" },
  { value: "mean", label: "Mean" },
  { value: "max", label: "Max" },
  { value: "min", label: "Min" },
  { value: "sum", label: "Total" },
]

describe("reducerChoices", () => {
  it("offers every id outside the curated list when nothing else is selected", () => {
    const { shown, offered } = reducerChoices(legend, ["mean"], anyReducerIds)
    expect(shown).toEqual(legend)
    expect(offered).toHaveLength(anyReducerIds.length - legend.length)
    expect(offered).not.toContain("mean")
    expect(offered).toContain("allValues")
    expect(offered).toContain("p95")
  })

  it("shows a selected value that is not curated with its reducer label", () => {
    const { shown, offered } = reducerChoices(
      legend,
      ["mean", "p95", "variance"],
      anyReducerIds
    )
    expect(shown.slice(legend.length)).toEqual([
      { value: "p95", label: "95th %" },
      { value: "variance", label: "Variance" },
    ])
    expect(offered).not.toContain("p95")
    expect(offered).not.toContain("variance")
  })

  it("keeps a selected value that is no reducer under its own text and ignores an empty one", () => {
    const { shown } = reducerChoices(legend, ["", "bogus"], reducerIds)
    expect(shown.slice(legend.length)).toEqual([
      { value: "bogus", label: "bogus" },
    ])
  })

  it("does not repeat a value selected twice", () => {
    const { shown } = reducerChoices(legend, ["p50", "p50"], reducerIds)
    expect(shown.filter((option) => option.value === "p50")).toHaveLength(1)
  })

  it("leaves the four non-number reducers out of the numeric pool", () => {
    const { offered } = reducerChoices(legend, [], reducerPools.reducer)
    expect(offered).toHaveLength(reducerIds.length - legend.length)
    expect(offered).not.toContain("allValues")
  })

  it("offers everything for an empty curated list", () => {
    expect(reducerChoices([], [], reducerIds).offered).toEqual(reducerIds)
  })
})

describe("reducerMatches", () => {
  it.each([
    ["p95", "95", true],
    ["p95", "P9", true],
    ["p95", "95th", true],
    ["p95", "  95th % ", true],
    ["p95", "p96", false],
    ["variance", "var", true],
    ["allValues", "all val", true],
    ["countAll", "count all", true],
    ["max", "", true],
    ["max", "min", false],
  ] as const)("matches %s against %j: %s", (id, query, want) => {
    expect(reducerMatches(id, query)).toBe(want)
  })
})
