import { describe, expect, it } from "vitest"

import { project } from "../projection"
import { heatmapModel, type HeatRow } from "./heatmap"

function rows(...weights: number[]): HeatRow[] {
  return weights.map((weight, index) => ({
    frameIndex: 0,
    row: index,
    lat: 10 * index,
    lon: 5 * index,
    weight,
  }))
}

describe("heatmapModel", () => {
  it("normalises weights to the largest", () => {
    const { points } = heatmapModel(rows(1, 2, 4), {}, true)
    expect(points.map((point) => point.weight)).toEqual([0.25, 0.5, 1])
  })

  it("gives every point the full weight when none is set", () => {
    const { points } = heatmapModel(rows(1, 1, 1), {}, false)
    expect(points.map((point) => point.weight)).toEqual([1, 1, 1])
  })

  it("treats negative and non-finite weights as zero", () => {
    const { points } = heatmapModel(rows(-3, Infinity, NaN, 2), {}, true)
    expect(points.map((point) => point.weight)).toEqual([0, 0, 0, 1])
  })

  it("draws nothing bright when every weight is zero", () => {
    const { points } = heatmapModel(rows(0, 0), {}, true)
    expect(points.map((point) => point.weight)).toEqual([0, 0])
  })

  it("places each point in the unit square", () => {
    const { points } = heatmapModel(rows(1, 1), {}, false)
    const expected = project(5, 10)
    expect(points[1].x).toBeCloseTo(expected.x, 10)
    expect(points[1].y).toBeCloseTo(expected.y, 10)
  })

  it("gives every unweighted point the layer's fixed weight, not normalised", () => {
    const { points } = heatmapModel(rows(1, 7, 3), { weight: 0.5 }, false)
    expect(points.map((point) => point.weight)).toEqual([0.5, 0.5, 0.5])
  })

  it("scales field weights into the weight range and draws anything above 1 as 1", () => {
    const { points } = heatmapModel(
      rows(1, 2, 10),
      { weightMin: 0.2, weightMax: 5 },
      true
    )
    expect(points[0].weight).toBeCloseTo(0.68, 10)
    expect(points.slice(1).map((point) => point.weight)).toEqual([1, 1])
  })

  it("ignores the fixed weight when the layer is weighted", () => {
    const { points } = heatmapModel(rows(1, 2), { weight: 0.1 }, true)
    expect(points.map((point) => point.weight)).toEqual([0.5, 1])
  })

  it("uses a radius and blur of 15 unless the layer sets them", () => {
    expect(heatmapModel([], {}, false)).toMatchObject({ radius: 15, blur: 15 })
    expect(heatmapModel([], { radius: 30, blur: 0 }, false)).toMatchObject({
      radius: 30,
      blur: 0,
    })
  })
})
