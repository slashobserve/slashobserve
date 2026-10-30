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
    const { points } = heatmapModel(rows(1, 2, 4), {})
    expect(points.map((point) => point.weight)).toEqual([0.25, 0.5, 1])
  })

  it("gives every point the full weight when none is set", () => {
    const { points } = heatmapModel(rows(1, 1, 1), {})
    expect(points.map((point) => point.weight)).toEqual([1, 1, 1])
  })

  it("treats negative and non-finite weights as zero", () => {
    const { points } = heatmapModel(rows(-3, Infinity, NaN, 2), {})
    expect(points.map((point) => point.weight)).toEqual([0, 0, 0, 1])
  })

  it("draws nothing bright when every weight is zero", () => {
    const { points } = heatmapModel(rows(0, 0), {})
    expect(points.map((point) => point.weight)).toEqual([0, 0])
  })

  it("places each point in the unit square", () => {
    const { points } = heatmapModel(rows(1, 1), {})
    const expected = project(5, 10)
    expect(points[1].x).toBeCloseTo(expected.x, 10)
    expect(points[1].y).toBeCloseTo(expected.y, 10)
  })

  it("uses a radius and blur of 15 unless the layer sets them", () => {
    expect(heatmapModel([], {})).toMatchObject({ radius: 15, blur: 15 })
    expect(heatmapModel([], { radius: 30, blur: 0 })).toMatchObject({
      radius: 30,
      blur: 0,
    })
  })
})
