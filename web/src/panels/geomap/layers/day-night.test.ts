import { describe, expect, it } from "vitest"

import { dayNightModel, subsolarPoint, terminator } from "./day-night"

const equinox = new Date("2024-03-20T12:00:00Z")
const juneSolstice = new Date("2025-06-21T12:00:00Z")
const decemberSolstice = new Date("2025-12-21T12:00:00Z")

type Ring = { lon: number; lat: number }[]

function contains(ring: Ring, lon: number, lat: number): boolean {
  let inside = false
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const a = ring[i]
    const b = ring[j]
    const crosses =
      a.lat > lat !== b.lat > lat &&
      lon < ((b.lon - a.lon) * (lat - a.lat)) / (b.lat - a.lat) + a.lon
    if (crosses) inside = !inside
  }
  return inside
}

describe("subsolarPoint", () => {
  it("is near longitude and latitude 0 at the March equinox at 12:00 UTC", () => {
    const sun = subsolarPoint(equinox)
    expect(Math.abs(sun.lon)).toBeLessThan(2)
    expect(Math.abs(sun.lat)).toBeLessThan(2)
  })

  it("sits over the Tropic of Cancer at the June solstice", () => {
    expect(subsolarPoint(juneSolstice).lat).toBeGreaterThan(22.5)
    expect(subsolarPoint(juneSolstice).lat).toBeLessThan(24)
  })

  it("sits over the Tropic of Capricorn at the December solstice", () => {
    expect(subsolarPoint(decemberSolstice).lat).toBeLessThan(-22.5)
    expect(subsolarPoint(decemberSolstice).lat).toBeGreaterThan(-24)
  })

  it("moves 90 degrees west six hours later", () => {
    const later = subsolarPoint(new Date("2024-03-20T18:00:00Z"))
    expect(later.lon).toBeGreaterThan(-93)
    expect(later.lon).toBeLessThan(-87)
  })
})

describe("terminator", () => {
  it("samples 360 longitudes between the two poles' edges", () => {
    const ring = terminator(equinox)
    expect(ring).toHaveLength(361 + 2)
    expect(ring[0].lon).toBe(-180)
    expect(ring[ring.length - 1].lon).toBe(180)
  })

  it("covers the point opposite the sun and not the point under it", () => {
    const sun = subsolarPoint(equinox)
    const ring = terminator(equinox)
    const opposite = sun.lon > 0 ? sun.lon - 180 : sun.lon + 180
    expect(contains(ring, sun.lon, 0)).toBe(false)
    expect(contains(ring, opposite, 0)).toBe(true)
  })

  it("covers the south pole in northern summer and the north pole in winter", () => {
    expect(contains(terminator(juneSolstice), 0, -80)).toBe(true)
    expect(contains(terminator(juneSolstice), 0, 80)).toBe(false)
    expect(contains(terminator(decemberSolstice), 0, 80)).toBe(true)
    expect(contains(terminator(decemberSolstice), 0, -80)).toBe(false)
  })
})

describe("dayNightModel", () => {
  it("draws the sun only when asked", () => {
    expect(dayNightModel(equinox, {}, undefined).sun).toBeNull()
    const sun = dayNightModel(equinox, { showSun: true }, undefined).sun
    expect(sun?.x).toBeGreaterThan(0.45)
    expect(sun?.x).toBeLessThan(0.55)
    expect(sun?.y).toBeGreaterThan(0.45)
    expect(sun?.y).toBeLessThan(0.55)
  })

  it("closes the night path and keeps a chosen color", () => {
    const model = dayNightModel(equinox, {}, "var(--threshold-info)")
    expect(model.path.startsWith("M")).toBe(true)
    expect(model.path.endsWith("Z")).toBe(true)
    expect(model.color).toBe("var(--threshold-info)")
    expect(dayNightModel(equinox, {}, undefined).color).toBeNull()
  })
})
