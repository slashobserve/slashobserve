import { describe, expect, it } from "vitest"

import { sparklineHeight, sparklinePoints } from "./sparkline-points"

describe("sparklinePoints", () => {
  it("scales a short series to the 600 by 40 viewBox with a 2px inset", () => {
    expect(
      sparklinePoints(
        [
          [0, 0],
          [10, 10],
          [20, 5],
        ],
        300
      )
    ).toEqual(["0.0,38.0", "300.0,2.0", "600.0,20.0"])
  })

  it("centres a flat series", () => {
    expect(
      sparklinePoints(
        [
          [0, 7],
          [1, 7],
        ],
        300
      )
    ).toEqual(["0.0,20.0", "600.0,20.0"])
  })

  it("draws 200000 points at about the tile width and keeps the spike", () => {
    const total = 200_000
    const points: [number, number][] = Array.from(
      { length: total },
      (_, index) => [index, index === 123_457 ? 1000 : index % 10]
    )
    const line = sparklinePoints(points, 300)
    expect(line.length).toBeLessThanOrEqual(602)
    expect(line.length).toBeGreaterThan(300)
    const ys = line.map((point) => Number(point.split(",")[1]))
    expect(Math.min(...ys)).toBe(2)
    expect(Math.max(...ys)).toBeLessThanOrEqual(sparklineHeight - 2)
    expect(line[0].startsWith("0.0,")).toBe(true)
    expect(line.at(-1)?.startsWith("600.0,")).toBe(true)
  })
})
