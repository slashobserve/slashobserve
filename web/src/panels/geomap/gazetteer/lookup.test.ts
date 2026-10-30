import { beforeAll, describe, expect, it } from "vitest"

import type { Feature } from "../layers/geojson"
import { atlasFeatures } from "../world"
import { isoCountries } from "./iso-countries"
import {
  airportsUnavailable,
  atlasOf,
  buildGazetteer,
  centroidOf,
  gazetteerName,
  resolveLookup,
  type Gazetteer,
} from "./lookup"
import { usStateCodes } from "./us-states"

let countries: Gazetteer
let states: Gazetteer
let countryFeatures: Feature[]
let stateFeatures: Feature[]

beforeAll(async () => {
  countryFeatures = await atlasFeatures("countries")
  stateFeatures = await atlasFeatures("usa-states")
  countries = buildGazetteer("countries", countryFeatures)
  states = buildGazetteer("usa-states", stateFeatures)
})

const franceBounds = { south: 41.3, north: 51.1, west: -5.2, east: 9.6 }
const californiaBounds = { south: 32.5, north: 42, west: -124.5, east: -114.1 }

function inside(
  place: { lat: number; lon: number } | null,
  bounds: typeof franceBounds
) {
  expect(place).not.toBeNull()
  expect(place?.lat).toBeGreaterThan(bounds.south)
  expect(place?.lat).toBeLessThan(bounds.north)
  expect(place?.lon).toBeGreaterThan(bounds.west)
  expect(place?.lon).toBeLessThan(bounds.east)
}

describe("country lookup", () => {
  it("resolves a name, alpha-2, alpha-3 and any case to one centroid inside France", () => {
    const named = countries.find("France")
    inside(named, franceBounds)
    for (const spelling of ["FR", "FRA", "fra", "  france  "]) {
      expect(countries.find(spelling)).toEqual(named)
    }
  })

  it("resolves the names people write for atlas names that are abbreviated", () => {
    expect(countries.find("United States")).toEqual(
      countries.find("United States of America")
    )
    expect(countries.find("US")).toEqual(countries.find("USA"))
    expect(countries.find("Czech Republic")).toEqual(countries.find("CZ"))
    expect(countries.find("Bosnia and Herzegovina")).toEqual(
      countries.find("BA")
    )
  })

  it("places Russia inside Russia rather than at the antimeridian", () => {
    const russia = countries.find("RU")
    expect(russia?.lon).toBeGreaterThan(40)
    expect(russia?.lon).toBeLessThan(120)
    expect(russia?.lat).toBeGreaterThan(50)
  })

  it("does not locate an unknown value, empty text or null", () => {
    expect(countries.find("Atlantis")).toBeNull()
    expect(countries.find("")).toBeNull()
    expect(countries.find(null)).toBeNull()
    expect(countries.find(undefined)).toBeNull()
  })

  it("has an ISO row for every atlas country that carries an id", () => {
    const known = new Set(isoCountries.map(([, , numeric]) => numeric))
    const missing = countryFeatures
      .filter((feature) => feature.id !== undefined)
      .filter((feature) => !known.has(String(feature.id)))
      .map((feature) => feature.properties.name)
    expect(missing).toEqual([])
  })

  it("has no duplicate ISO code", () => {
    for (const column of [0, 1, 2]) {
      const codes = isoCountries.map((row) => row[column])
      expect(new Set(codes).size).toBe(codes.length)
    }
  })
})

describe("US state lookup", () => {
  it("resolves a name and a two-letter code inside California", () => {
    const named = states.find("California")
    inside(named, californiaBounds)
    expect(states.find("CA")).toEqual(named)
    expect(states.find("ca")).toEqual(named)
  })

  it("does not read a country name as a state", () => {
    expect(states.find("France")).toBeNull()
  })

  it("has a code for every state and territory in the atlas", () => {
    const known = new Set(Object.values(usStateCodes))
    const missing = stateFeatures
      .filter((feature) => !known.has(String(feature.id)))
      .map((feature) => feature.properties.name)
    expect(missing).toEqual([])
  })
})

describe("centroidOf", () => {
  it("uses the largest polygon of a multipolygon", () => {
    const square = (west: number, south: number, size: number) => [
      [
        [west, south],
        [west + size, south],
        [west + size, south + size],
        [west, south + size],
        [west, south],
      ],
    ]
    const place = centroidOf({
      type: "MultiPolygon",
      coordinates: [square(0, 0, 1), square(20, 20, 10)],
    })
    expect(place?.lon).toBeCloseTo(25, 5)
    expect(place?.lat).toBeGreaterThan(24)
    expect(place?.lat).toBeLessThan(26)
  })

  it("has no centroid for a point or a degenerate ring", () => {
    expect(centroidOf({ type: "Point", coordinates: [1, 2] })).toBeNull()
    expect(
      centroidOf({
        type: "Polygon",
        coordinates: [
          [
            [0, 0],
            [1, 1],
          ],
        ],
      })
    ).toBeNull()
  })
})

describe("resolveLookup", () => {
  it("waits for a bundled gazetteer that has not loaded", () => {
    expect(resolveLookup("countries", {})).toEqual({ state: "pending" })
  })

  it("returns a loaded gazetteer", () => {
    expect(resolveLookup("usa-states", { "usa-states": states })).toEqual({
      state: "ready",
      gazetteer: states,
    })
  })

  it("never has airports and says so", () => {
    expect(resolveLookup("airports", { countries })).toEqual({
      state: "unavailable",
      note: airportsUnavailable,
    })
    expect(airportsUnavailable).toBe("Airport lookup is not available.")
  })
})

describe("gazetteerName", () => {
  it("defaults to countries", () => {
    expect(gazetteerName(undefined)).toBe("countries")
    expect(gazetteerName("usa-states")).toBe("usa-states")
    expect(gazetteerName("airports")).toBe("airports")
    expect(gazetteerName("mars")).toBe("countries")
  })

  it("names the atlas a gazetteer needs", () => {
    expect(atlasOf("usa-states")).toBe("usa-states")
    expect(atlasOf("airports")).toBeNull()
  })
})
