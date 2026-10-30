import { describe, expect, it } from "vitest"

import {
  distanceMeters,
  formatImperial,
  formatMetric,
  metersPerPixel,
  niceLength,
  pathMeters,
  scaleBars,
} from "./scale"

describe("metersPerPixel", () => {
  it("is about 156543 meters at the equator at zoom 0", () => {
    expect(metersPerPixel(0, 0)).toBeCloseTo(156543.03, 1)
  })

  it("halves with each zoom level", () => {
    expect(metersPerPixel(0, 3)).toBeCloseTo(metersPerPixel(0, 2) / 2, 6)
  })

  it("shrinks with the cosine of the latitude", () => {
    expect(metersPerPixel(60, 4)).toBeCloseTo(metersPerPixel(0, 4) / 2, 6)
  })
})

describe("niceLength", () => {
  it.each([
    [1, 1],
    [1.9, 1],
    [2, 2],
    [4.9, 2],
    [5, 5],
    [9.99, 5],
    [10, 10],
    [370, 200],
    [0.3, 0.2],
    [12_345, 10_000],
    [78_000, 50_000],
  ])("rounds %d down to %d", (limit, expected) => {
    expect(niceLength(limit)).toBeCloseTo(expected, 10)
  })

  it("is zero for nothing", () => {
    expect(niceLength(0)).toBe(0)
    expect(niceLength(-3)).toBe(0)
  })
})

describe("scaleBars", () => {
  it("labels a continental view in kilometres and miles", () => {
    const bars = scaleBars(0, 4, 100)
    expect(bars.metric.label).toBe("500 km")
    expect(bars.imperial.label).toBe("500 mi")
    expect(bars.metric.pixels).toBeLessThanOrEqual(100)
    expect(bars.imperial.pixels).toBeLessThanOrEqual(100)
  })

  it("labels a street view in metres and feet", () => {
    const bars = scaleBars(0, 18, 100)
    expect(bars.metric.label).toBe("50 m")
    expect(bars.imperial.label).toBe("100 ft")
  })

  it("draws each bar at the pixel length of its distance", () => {
    const bars = scaleBars(40, 10, 120)
    const perPixel = metersPerPixel(40, 10)
    const meters = Number.parseFloat(bars.metric.label) * 1000
    expect(bars.metric.pixels).toBeCloseTo(meters / perPixel, 6)
  })
})

describe("great-circle distance", () => {
  it("measures a degree of longitude on the equator", () => {
    expect(distanceMeters({ lon: 0, lat: 0 }, { lon: 1, lat: 0 })).toBeCloseTo(
      111_195,
      -1
    )
  })

  it("measures London to Paris at about 344 km", () => {
    const london = { lon: -0.1276, lat: 51.5072 }
    const paris = { lon: 2.3522, lat: 48.8566 }
    expect(distanceMeters(london, paris) / 1000).toBeCloseTo(343.6, 0)
  })

  it("adds up the legs of a path", () => {
    const a = { lon: 0, lat: 0 }
    const b = { lon: 1, lat: 0 }
    const c = { lon: 2, lat: 0 }
    expect(pathMeters([a, b, c])).toBeCloseTo(distanceMeters(a, c), 6)
    expect(pathMeters([a])).toBe(0)
    expect(pathMeters([])).toBe(0)
  })
})

describe("distance text", () => {
  it.each([
    [0, "0 m"],
    [850, "850 m"],
    [1500, "1.5 km"],
    [12_345, "12.35 km"],
    [343_600, "344 km"],
  ])("writes %d meters as %s", (meters, text) => {
    expect(formatMetric(meters)).toBe(text)
  })

  it.each([
    [100, "328 ft"],
    [1609.344, "1 mi"],
    [16_093.44, "10 mi"],
    [343_600, "214 mi"],
  ])("writes %d meters as %s", (meters, text) => {
    expect(formatImperial(meters)).toBe(text)
  })
})
