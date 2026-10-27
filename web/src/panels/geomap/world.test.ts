import { describe, expect, it } from "vitest"

import { worldOutline } from "./world"

function segments(path: string): { command: string; x: number; y: number }[] {
  return [...path.matchAll(/([ML])(-?[\d.e-]+) (-?[\d.e-]+)/g)].map(
    ([, command, x, y]) => ({ command, x: Number(x), y: Number(y) })
  )
}

describe("worldOutline", () => {
  it("draws country borders inside the unit square", async () => {
    const points = segments(await worldOutline())
    expect(points.length).toBeGreaterThan(1000)
    for (const { x, y } of points) {
      expect(x).toBeGreaterThanOrEqual(0)
      expect(x).toBeLessThanOrEqual(1)
      expect(y).toBeGreaterThanOrEqual(0)
      expect(y).toBeLessThanOrEqual(1)
    }
  })

  it("never joins points on opposite sides of the antimeridian", async () => {
    const points = segments(await worldOutline())
    points.forEach((point, index) => {
      if (point.command === "M" || index === 0) return
      expect(Math.abs(point.x - points[index - 1].x)).toBeLessThan(0.5)
    })
  })

  it("is built once", async () => {
    expect(worldOutline()).toBe(worldOutline())
  })
})
