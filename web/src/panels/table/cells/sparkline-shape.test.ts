import { describe, expect, it } from "vitest"

import {
  sparkHeight,
  sparkRange,
  sparkShape,
  type SparkOptions,
} from "./sparkline-shape"

const base: SparkOptions = {
  style: "line",
  interpolation: "linear",
  lineWidth: 2,
  fill: 0,
  points: "auto",
  pointSize: 4,
  spanNulls: false,
}

describe("sparkRange", () => {
  it("spans the data and widens an empty range by one", () => {
    expect(sparkRange([2, 8, null], {})).toEqual({ low: 2, high: 8 })
    expect(sparkRange([3, 3], {})).toEqual({ low: 3, high: 4 })
  })

  it("lets soft limits widen and hard limits win", () => {
    expect(sparkRange([2, 8], { softMin: 0, softMax: 5 })).toEqual({
      low: 0,
      high: 8,
    })
    expect(sparkRange([2, 8], { softMin: 0, min: 1, max: 20 })).toEqual({
      low: 1,
      high: 20,
    })
  })
})

describe("sparkShape", () => {
  it("draws nothing for no values", () => {
    expect(sparkShape([], base)).toEqual({
      paths: [],
      areas: [],
      dots: [],
      bars: [],
    })
  })

  it("draws one path through a gapless series", () => {
    const shape = sparkShape([1, 2, 3], base)
    expect(shape.paths).toEqual(["M2.0,22.0 L48.0,12.0 L94.0,2.0"])
  })

  it("breaks the line at a null and marks the isolated sample", () => {
    const shape = sparkShape([1, 2, null, 3], base)
    expect(shape.paths).toHaveLength(1)
    expect(shape.dots).toHaveLength(1)
  })

  it("joins a gap when nulls are spanned", () => {
    const shape = sparkShape([1, null, 3], { ...base, spanNulls: true })
    expect(shape.paths).toHaveLength(1)
    expect(shape.dots).toHaveLength(0)
  })

  it("steps before and after each sample", () => {
    expect(
      sparkShape([1, 3], { ...base, interpolation: "stepAfter" }).paths
    ).toEqual(["M2.0,22.0 L94.0,22.0 L94.0,2.0"])
    expect(
      sparkShape([1, 3], { ...base, interpolation: "stepBefore" }).paths
    ).toEqual(["M2.0,22.0 L2.0,2.0 L94.0,2.0"])
  })

  it("smooths with horizontal tangents that never leave the range", () => {
    const [d] = sparkShape([1, 3], { ...base, interpolation: "smooth" }).paths
    expect(d).toBe("M2.0,22.0 C48.0,22.0 48.0,2.0 94.0,2.0")
  })

  it("fills under the line only when a fill is set", () => {
    expect(sparkShape([1, 2], base).areas).toEqual([])
    const [area] = sparkShape([1, 2], { ...base, fill: 30 }).areas
    expect(
      area.endsWith(`L94.0,${sparkHeight - 2}.0 L2.0,${sparkHeight - 2}.0 Z`)
    ).toBe(true)
  })

  it("marks every sample for always and none for never", () => {
    expect(
      sparkShape([1, 2, 3], { ...base, points: "always" }).dots
    ).toHaveLength(3)
    expect(
      sparkShape([1, null, 3], { ...base, points: "never" }).dots
    ).toHaveLength(0)
  })

  it("draws bars and no line for the bars style", () => {
    const shape = sparkShape([1, null, 3], { ...base, style: "bars" })
    expect(shape.paths).toEqual([])
    expect(shape.bars).toHaveLength(2)
    expect(shape.bars[1].y).toBeLessThan(shape.bars[0].y)
  })

  it("draws only dots for the points style", () => {
    const shape = sparkShape([1, 2, 3], { ...base, style: "points" })
    expect(shape.paths).toEqual([])
    expect(shape.dots).toHaveLength(3)
  })
})
