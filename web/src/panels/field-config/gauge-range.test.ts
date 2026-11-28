import { describe, expect, it } from "vitest"

import { gaugeFraction, gaugeRange } from "./gauge-range"

describe("gauge range", () => {
  it("uses the configured range, then the unit's, then the data", () => {
    expect(gaugeRange({ min: 10, max: 20 }, { min: 0, max: 5 })).toEqual({
      min: 10,
      max: 20,
    })
    expect(gaugeRange({ unit: "percentunit" }, { max: 7 })).toEqual({
      min: 0,
      max: 1,
    })
    expect(gaugeRange({}, { min: 3, max: 9 })).toEqual({
      min: 0,
      max: 9,
    })
    expect(gaugeRange({}, { min: -4, max: 9 })).toEqual({
      min: -4,
      max: 9,
    })
  })

  it("widens an empty range by one", () => {
    expect(gaugeRange({}, { min: 0, max: 0 })).toEqual({
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
