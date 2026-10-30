import { describe, expect, it } from "vitest"

import { fitBounds, project, worldSize } from "./projection"
import { isRegion, regionBounds, regionCorners } from "./regions"

const regions = Object.keys(regionBounds)

describe("regions", () => {
  it("knows the ten region ids the Spec allows", () => {
    expect([...regions].sort()).toEqual(
      [
        "africa",
        "australia",
        "east-asia",
        "europe",
        "north-america",
        "oceania",
        "south-america",
        "south-asia",
        "south-east-asia",
        "west-asia",
      ].sort()
    )
  })

  it("tells regions from the other view values", () => {
    expect(isRegion("europe")).toBe(true)
    for (const other of ["fit", "world", "coords", "toString", 3, undefined]) {
      expect(isRegion(other)).toBe(false)
    }
  })

  it.each(regions)("%s fits inside the body when fitted", (region) => {
    const [width, height] = [900, 600]
    const view = fitBounds(regionCorners(region), width, height)
    const size = worldSize(view.zoom)
    const { west, south, east, north } = regionBounds[region]
    const corners = [project(west, north), project(east, south)]
    for (const corner of corners) {
      const x = width / 2 + (corner.x - view.center.x) * size
      const y = height / 2 + (corner.y - view.center.y) * size
      expect(x).toBeGreaterThanOrEqual(0)
      expect(x).toBeLessThanOrEqual(width)
      expect(y).toBeGreaterThanOrEqual(0)
      expect(y).toBeLessThanOrEqual(height)
    }
  })

  it.each(regions)("%s is centered on its bounds", (region) => {
    const view = fitBounds(regionCorners(region), 900, 600)
    const { west, south, east, north } = regionBounds[region]
    const a = project(west, north)
    const b = project(east, south)
    expect(view.center.x).toBeCloseTo((a.x + b.x) / 2, 10)
    expect(view.center.y).toBeCloseTo((a.y + b.y) / 2, 10)
  })
})
