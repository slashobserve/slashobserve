import { describe, expect, it } from "vitest"

import {
  clampCenter,
  clampZoom,
  fitBounds,
  maxLatitude,
  maxZoom,
  minZoom,
  panBy,
  project,
  unproject,
  worldSize,
  worldView,
  zoomAround,
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

describe("zoom limits from the panel", () => {
  it("keeps zoom between the panel minimum and maximum", () => {
    const limits = { min: 3, max: 6 }
    expect(clampZoom(1, 600, 300, limits)).toBe(3)
    expect(clampZoom(9, 600, 300, limits)).toBe(6)
    expect(clampZoom(4.5, 600, 300, limits)).toBe(4.5)
  })

  it("never zooms out past the world view whatever the minimum says", () => {
    expect(clampZoom(0, 600, 300, { min: -4 })).toBe(minZoom(600, 300))
  })

  it("lets the maximum win when the limits cross", () => {
    expect(clampZoom(8, 600, 300, { min: 7, max: 5 })).toBe(5)
    expect(clampZoom(1, 600, 300, { min: 7, max: 5 })).toBe(5)
  })

  it("keeps a fitted view inside the limits", () => {
    const points = [{ lon: 10, lat: 50 }]
    expect(fitBounds(points, 600, 300, { limits: { max: 4 } }).zoom).toBe(4)
    expect(fitBounds(points, 600, 300, { limits: { min: 8 } }).zoom).toBe(8)
    expect(fitBounds([], 600, 300, { limits: { min: 5 } }).zoom).toBe(5)
  })

  it("pads the fit by the given pixels", () => {
    const points = [
      { lon: -20, lat: 10 },
      { lon: 20, lat: 30 },
    ]
    const tight = fitBounds(points, 600, 400, { padding: 0 })
    const padded = fitBounds(points, 600, 400, { padding: 100 })
    expect(tight.zoom).toBeGreaterThan(padded.zoom)
    expect(tight.center).toEqual(padded.center)
  })
})

describe("unproject", () => {
  it("undoes project", () => {
    for (const [lon, lat] of [
      [0, 0],
      [139.69, 35.68],
      [-74, 40.7],
      [151.2, -33.87],
    ]) {
      const { x, y } = project(lon, lat)
      const back = unproject(x, y)
      expect(back.lon).toBeCloseTo(lon, 6)
      expect(back.lat).toBeCloseTo(lat, 6)
    }
  })
})

describe("zoomAround", () => {
  const view = { center: { x: 0.5, y: 0.5 }, zoom: 3 }

  it("keeps the point under the pointer in place", () => {
    const [width, height, px, py] = [800, 400, 600, 100]
    const next = zoomAround(view, 4, px, py, width, height)
    const worldX = (zoom: number, center: number) =>
      center + (px - width / 2) / worldSize(zoom)
    const worldY = (zoom: number, center: number) =>
      center + (py - height / 2) / worldSize(zoom)
    expect(worldX(4, next.center.x)).toBeCloseTo(worldX(3, 0.5), 10)
    expect(worldY(4, next.center.y)).toBeCloseTo(worldY(3, 0.5), 10)
  })

  it("leaves the center alone when the pointer is at the center", () => {
    expect(zoomAround(view, 5, 400, 200, 800, 400).center).toEqual(view.center)
  })

  it("stops at the limits", () => {
    expect(zoomAround(view, 12, 10, 10, 800, 400, { max: 5 }).zoom).toBe(5)
  })
})

describe("clampCenter", () => {
  it("keeps the center inside the unit square", () => {
    expect(clampCenter(-0.2)).toBe(0)
    expect(clampCenter(0.4)).toBe(0.4)
    expect(clampCenter(1.3)).toBe(1)
  })
})

describe("panBy", () => {
  const view = { center: { x: 0.5, y: 0.5 }, zoom: 3 }

  it("moves the center by the panel fraction in world units", () => {
    const panned = panBy(view, 0.1, 0, 800, 400)
    expect(panned.center.x).toBeCloseTo(0.5 + (0.1 * 800) / worldSize(3), 10)
    expect(panned.center.y).toBe(0.5)
    expect(panned.zoom).toBe(3)
  })

  it("moves up and left with negative fractions", () => {
    const panned = panBy(view, -0.1, -0.1, 800, 400)
    expect(panned.center.x).toBeCloseTo(0.5 - (0.1 * 800) / worldSize(3), 10)
    expect(panned.center.y).toBeCloseTo(0.5 - (0.1 * 400) / worldSize(3), 10)
  })

  it("stops at the edges of the world", () => {
    const edge = { center: { x: 0.99, y: 0.01 }, zoom: 3 }
    const panned = panBy(edge, 0.5, -0.5, 800, 400)
    expect(panned.center).toEqual({ x: 1, y: 0 })
  })
})
