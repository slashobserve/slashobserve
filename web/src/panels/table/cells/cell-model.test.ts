import { describe, expect, it } from "vitest"

import {
  cellSpecOf,
  fillTextColor,
  gaugeFraction,
  gaugeRange,
  jsonView,
  lastNumber,
  numberSeries,
  pillHue,
  pillLabels,
} from "./cell-model"

describe("cellSpecOf", () => {
  it("is the plain cell when no cell is set", () => {
    expect(cellSpecOf(undefined)).toEqual({
      type: "auto",
      gaugeStyle: "basic",
      valueMode: "text",
      wholeRow: false,
      wrap: false,
      imageAlt: undefined,
      imageTitle: undefined,
    })
  })

  it("keeps what the Spec sets", () => {
    expect(
      cellSpecOf({ type: "gauge", gaugeStyle: "lcd", valueMode: "hidden" })
    ).toMatchObject({ type: "gauge", gaugeStyle: "lcd", valueMode: "hidden" })
  })
})

describe("fillTextColor", () => {
  it("picks white text on a dark fill and near-black on a light one", () => {
    const toRgb = (color: string) =>
      color === "dark" ? ([10, 10, 80] as const) : ([250, 250, 200] as const)
    expect(fillTextColor("dark", toRgb)).toBe("#ffffff")
    expect(fillTextColor("light", toRgb)).toBe("#0a0a0a")
  })

  it("leaves the text alone with no fill or an unparsable one", () => {
    expect(fillTextColor(null, () => [0, 0, 0])).toBeUndefined()
    expect(fillTextColor("nope", () => null)).toBeUndefined()
  })
})

describe("pillLabels", () => {
  it.each([
    ["a, b ,c", ["a", "b", "c"]],
    ["a,,b, ,a", ["a", "b"]],
    ["", []],
    [null, []],
    [42, ["42"]],
  ])("splits %j on commas", (value, expected) => {
    expect(pillLabels(value)).toEqual(expected)
  })

  it("keeps at most 20 labels", () => {
    const many = Array.from({ length: 30 }, (_, i) => `l${i}`).join(",")
    expect(pillLabels(many)).toHaveLength(20)
  })
})

describe("pillHue", () => {
  it("is stable and one of the eight series hues", () => {
    for (const label of ["api", "db", "cache", "ünï", "😀"]) {
      const hue = pillHue(label)
      expect(hue).toBeGreaterThanOrEqual(1)
      expect(hue).toBeLessThanOrEqual(8)
      expect(pillHue(label)).toBe(hue)
    }
  })
})

describe("jsonView", () => {
  it("pretty-prints an object and a JSON string", () => {
    expect(jsonView({ a: 1 })?.pretty).toBe('{\n  "a": 1\n}')
    expect(jsonView(' {"a":[1,2]} ')?.summary).toBe('{"a":[1,2]}')
  })

  it("cuts a long summary", () => {
    const view = jsonView({ text: "x".repeat(200) })
    expect(view?.summary).toHaveLength(80)
    expect(view?.summary.endsWith("…")).toBe(true)
  })

  it.each(["plain", "{broken", 5, null, undefined, true])(
    "is not JSON for %j",
    (value) => {
      expect(jsonView(value)).toBeNull()
    }
  )
})

describe("numberSeries", () => {
  it("keeps numbers and turns the rest into gaps", () => {
    expect(numberSeries([1, null, "x", NaN, 4])).toEqual([
      1,
      null,
      null,
      null,
      4,
    ])
  })

  it("is not a series without a number or an array", () => {
    expect(numberSeries([null, "a"])).toBeNull()
    expect(numberSeries("1,2")).toBeNull()
    expect(numberSeries([])).toBeNull()
  })

  it("finds the last number", () => {
    expect(lastNumber([1, 2, null])).toBe(2)
    expect(lastNumber([null])).toBeNull()
  })
})

describe("gauge range", () => {
  it("uses the configured range, then the unit's, then the column's data", () => {
    expect(
      gaugeRange({ min: 10, max: 20 }, { fieldMin: 0, fieldMax: 5 })
    ).toEqual({ min: 10, max: 20 })
    expect(gaugeRange({ unit: "percentunit" }, { fieldMax: 7 })).toEqual({
      min: 0,
      max: 1,
    })
    expect(gaugeRange({}, { fieldMin: 3, fieldMax: 9 })).toEqual({
      min: 0,
      max: 9,
    })
    expect(gaugeRange({}, { fieldMin: -4, fieldMax: 9 })).toEqual({
      min: -4,
      max: 9,
    })
  })

  it("widens an empty range by one", () => {
    expect(gaugeRange({}, { fieldMin: 0, fieldMax: 0 })).toEqual({
      min: 0,
      max: 1,
    })
  })

  it("clamps the fraction into the range", () => {
    const range = { min: 0, max: 10 }
    expect(gaugeFraction(5, range)).toBe(0.5)
    expect(gaugeFraction(50, range)).toBe(1)
    expect(gaugeFraction(-5, range)).toBe(0)
    expect(gaugeFraction(null, range)).toBe(0)
  })
})
