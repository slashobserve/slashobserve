import { describe, expect, it } from "vitest"

import {
  capLegend,
  effectiveLegendMode,
  inlineValues,
  legendNames,
  legendOrder,
  legendWorthShowing,
  maxStateLegendEntries,
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

describe("legendWorthShowing", () => {
  it("shows a legend for several series", () => {
    expect(legendWorthShowing(2, [])).toBe(true)
  })

  it("hides the legend of a single series that has no values to show", () => {
    expect(legendWorthShowing(1, [])).toBe(false)
  })

  it("shows the legend of a single series when it lists values", () => {
    expect(legendWorthShowing(1, ["lastNotNull"])).toBe(true)
  })

  it("shows nothing for no series", () => {
    expect(legendWorthShowing(0, ["lastNotNull"])).toBe(false)
  })
})

describe("capLegend", () => {
  it("keeps the first entries and counts the rest", () => {
    const items = Array.from({ length: 25 }, (_, index) => index)
    const { shown, more } = capLegend(items, maxStateLegendEntries)
    expect(shown).toEqual(items.slice(0, maxStateLegendEntries))
    expect(more).toBe(25 - maxStateLegendEntries)
  })

  it("leaves a short list alone", () => {
    expect(capLegend(["a", "b"], 12)).toEqual({ shown: ["a", "b"], more: 0 })
  })
})

describe("legendNames", () => {
  const tenPerCharacter = (text: string) => text.length * 10
  const statuses = ["STATUS_CODE_UNSET", "STATUS_CODE_OK", "STATUS_CODE_ERROR"]

  it("keeps whole labels without a room to fit them in", () => {
    expect(legendNames(statuses)).toEqual(statuses)
  })

  it("keeps whole labels when the widest one fits", () => {
    expect(
      legendNames(statuses, { px: 170, measure: tenPerCharacter })
    ).toEqual(statuses)
  })

  it("drops the shared prefix from every label at once when one does not fit", () => {
    expect(
      legendNames(statuses, { px: 169, measure: tenPerCharacter })
    ).toEqual(["UNSET", "OK", "ERROR"])
  })

  it("cuts only at a word boundary", () => {
    const room = { px: 1, measure: tenPerCharacter }
    expect(legendNames(["checkout-a", "checkout-b"], room)).toEqual(["a", "b"])
    expect(legendNames(["frontend", "frontproxy"], room)).toEqual([
      "frontend",
      "frontproxy",
    ])
  })

  it("keeps whole labels when no useful prefix is shared", () => {
    const room = { px: 1, measure: tenPerCharacter }
    expect(legendNames(["a_1", "a_2"], room)).toEqual(["a_1", "a_2"])
    expect(legendNames(["only"], room)).toEqual(["only"])
  })

  it("keeps every label when one is the prefix of another", () => {
    expect(
      legendNames(["svc_", "svc_x"], { px: 1, measure: tenPerCharacter })
    ).toEqual(["svc_", "svc_x"])
  })
})
