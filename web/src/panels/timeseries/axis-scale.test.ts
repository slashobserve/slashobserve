import { describe, expect, it } from "vitest"

import {
  centeredRange,
  logRange,
  scaleDistribution,
  softRange,
} from "./axis-scale"

describe("softRange", () => {
  it("widens the range to include the soft limits", () => {
    expect(softRange([2, 8], 0, 10)).toEqual([0, 10])
  })

  it("never cuts data that lies beyond a soft limit", () => {
    expect(softRange([-5, 20], 0, 10)).toEqual([-5, 20])
  })

  it("leaves an end without a soft limit alone", () => {
    expect(softRange([2, 8], undefined, 10)).toEqual([2, 10])
    expect(softRange([2, 8], 0, undefined)).toEqual([0, 8])
    expect(softRange([2, 8])).toEqual([2, 8])
  })
})

describe("centeredRange", () => {
  it("makes the range symmetric around zero from the larger end", () => {
    expect(centeredRange([-2, 10])).toEqual([-10, 10])
    expect(centeredRange([-7, 3])).toEqual([-7, 7])
  })

  it("mirrors a range that sits on one side of zero", () => {
    expect(centeredRange([4, 9])).toEqual([-9, 9])
  })
})

describe("logRange", () => {
  it("encloses positive data between powers of the base", () => {
    expect(logRange(3, 450, 10)).toEqual([1, 1000])
    expect(logRange(3, 450, 2)).toEqual([2, 512])
  })

  it("keeps data that sits exactly on a power inside a full decade", () => {
    expect(logRange(10, 100, 10)).toEqual([10, 100])
    expect(logRange(100, 100, 10)).toEqual([100, 1000])
  })

  it("uses fractional powers for small values", () => {
    expect(logRange(0.002, 0.5, 10)).toEqual([0.001, 1])
  })

  it("falls back to one decade when there is no positive data", () => {
    expect(logRange(-5, 0, 10)).toEqual([1, 10])
    expect(logRange(0, 0, 2)).toEqual([1, 2])
    expect(logRange(Infinity, -Infinity, 10)).toEqual([1, 10])
  })

  it("ignores zero and negative values at the low end", () => {
    expect(logRange(-4, 500, 10)).toEqual([100, 1000])
    expect(logRange(0, 500, 10)).toEqual([100, 1000])
  })
})

describe("scaleDistribution", () => {
  it("leaves a linear scale to uPlot's default", () => {
    expect(scaleDistribution("linear", 10, undefined)).toEqual({})
    expect(scaleDistribution(undefined, undefined, undefined)).toEqual({})
  })

  it("selects the logarithmic distribution with its base", () => {
    expect(scaleDistribution("log", 2, undefined)).toEqual({
      distr: 3,
      log: 2,
    })
    expect(scaleDistribution("log", undefined, undefined)).toEqual({
      distr: 3,
      log: 10,
    })
  })

  it("selects the arcsinh distribution with its linear threshold", () => {
    expect(scaleDistribution("symlog", 10, 5)).toEqual({ distr: 4, asinh: 5 })
    expect(scaleDistribution("symlog", 10, undefined)).toEqual({
      distr: 4,
      asinh: 1,
    })
  })
})
