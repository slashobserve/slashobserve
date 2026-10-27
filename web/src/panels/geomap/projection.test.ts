import { describe, expect, it } from "vitest"

import {
  clampZoom,
  fitBounds,
  maxLatitude,
  maxZoom,
  minZoom,
  project,
  worldSize,
  worldView,
} from "./projection"

describe("project", () => {
  it("puts the origin at the center of the unit square", () => {
    const { x, y } = project(0, 0)
    expect(x).toBeCloseTo(0.5, 10)
    expect(y).toBeCloseTo(0.5, 10)
  })

  it("maps longitude linearly and north upward", () => {
    expect(project(-180, 0).x).toBe(0)
    expect(project(180, 0).x).toBe(1)
    expect(project(0, 45).y).toBeCloseTo(0.359725, 5)
    expect(project(0, -45).y).toBeCloseTo(0.640275, 5)
  })

  it("clamps latitude to the Mercator limit", () => {
    expect(project(0, 90)).toEqual(project(0, maxLatitude))
    expect(project(0, -90)).toEqual(project(0, -maxLatitude))
    expect(project(0, maxLatitude).y).toBeCloseTo(0, 4)
  })
})

describe("fitBounds", () => {
  it("shows the world when there are no points", () => {
    expect(fitBounds([], 600, 300)).toEqual(worldView(600, 300))
  })

  it("centers a single point at the closest fit zoom", () => {
    const view = fitBounds([{ lon: 10, lat: 50 }], 600, 300)
    expect(view.center).toEqual(project(10, 50))
    expect(view.zoom).toBe(6)
  })

  it("centers two points between them and zooms to contain both", () => {
    const points = [
      { lon: -10, lat: 40 },
      { lon: 30, lat: 60 },
    ]
    const { center, zoom } = fitBounds(points, 600, 400)
    const a = project(-10, 40)
    const b = project(30, 60)
    expect(center.x).toBeCloseTo((a.x + b.x) / 2, 10)
    expect(center.y).toBeCloseTo((a.y + b.y) / 2, 10)
    const size = worldSize(zoom)
    expect(Math.abs(b.x - a.x) * size).toBeLessThanOrEqual(600)
    expect(Math.abs(b.y - a.y) * size).toBeLessThanOrEqual(400)
    expect(zoom).toBeGreaterThan(minZoom(600, 400))
  })

  it("zooms out no further than the world view", () => {
    const view = fitBounds(
      [
        { lon: -179, lat: -80 },
        { lon: 179, lat: 80 },
      ],
      600,
      300
    )
    expect(view.zoom).toBe(minZoom(600, 300))
  })

  it("fits points on one parallel by their longitude span", () => {
    const view = fitBounds(
      [
        { lon: 0, lat: 10 },
        { lon: 20, lat: 10 },
      ],
      800,
      600
    )
    expect(Number.isFinite(view.zoom)).toBe(true)
    expect(view.center.y).toBeCloseTo(project(0, 10).y, 10)
  })
})

describe("zoom limits", () => {
  it("keeps zoom between the world view and the maximum", () => {
    expect(clampZoom(-5, 600, 300)).toBe(minZoom(600, 300))
    expect(clampZoom(40, 600, 300)).toBe(maxZoom)
    expect(clampZoom(4, 600, 300)).toBe(4)
  })

  it("sizes the world as a square that doubles per zoom level", () => {
    expect(worldSize(1)).toBe(2 * worldSize(0))
    expect(worldSize(0)).toBe(256)
  })
})
