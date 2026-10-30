import { beforeAll, describe, expect, it } from "vitest"

import { buildGazetteer, type Gazetteers } from "../geomap/gazetteer/lookup"
import { atlasFeatures } from "../geomap/world"
import { fieldLookup } from "./field-lookup"
import { gazetteersNeeded } from "./location"
import type { PreparedFrame } from "./names"

let gazetteers: Gazetteers

beforeAll(async () => {
  gazetteers = {
    countries: buildGazetteer("countries", await atlasFeatures("countries")),
    "usa-states": buildGazetteer(
      "usa-states",
      await atlasFeatures("usa-states")
    ),
  }
})

function table(place: string[]): PreparedFrame {
  return {
    ref: "A",
    fields: [
      { name: "requests", type: "number", values: place.map((_, i) => i) },
      { name: "place", type: "string", values: place },
    ],
  }
}

function values(frame: PreparedFrame, name: string): unknown[] {
  return frame.fields.find((field) => field.name === name)?.values ?? []
}

describe("fieldLookup", () => {
  it("adds lat and lon for country names and codes, null for unknown ones", () => {
    const [out] = fieldLookup(
      [table(["France", "FRA", "Atlantis"])],
      { type: "fieldLookup", lookupField: "place" },
      gazetteers
    )
    expect(out.fields.map((field) => field.name)).toEqual([
      "requests",
      "place",
      "lat",
      "lon",
    ])
    const lat = values(out, "lat") as number[]
    const lon = values(out, "lon") as number[]
    expect(lat[0]).toBeGreaterThan(41.3)
    expect(lat[0]).toBeLessThan(51.1)
    expect(lon[0]).toBeGreaterThan(-5.2)
    expect(lon[0]).toBeLessThan(9.6)
    expect([lat[1], lon[1]]).toEqual([lat[0], lon[0]])
    expect([values(out, "lat")[2], values(out, "lon")[2]]).toEqual([null, null])
  })

  it("looks the first text field up in countries when nothing is named", () => {
    const [out] = fieldLookup(
      [table(["Japan"])],
      { type: "fieldLookup" },
      gazetteers
    )
    const lon = values(out, "lon")[0] as number
    expect(lon).toBeGreaterThan(122)
    expect(lon).toBeLessThan(146)
  })

  it("looks US state codes up in the states gazetteer", () => {
    const [out] = fieldLookup(
      [table(["CA", "Texas"])],
      { type: "fieldLookup", gazetteer: "usa-states" },
      gazetteers
    )
    const lat = values(out, "lat") as number[]
    expect(lat[0]).toBeGreaterThan(32.5)
    expect(lat[0]).toBeLessThan(42)
    expect(lat[1]).toBeGreaterThan(25.8)
    expect(lat[1]).toBeLessThan(36.5)
  })

  it("changes nothing while the gazetteer loads or when the named field is absent", () => {
    const frame = table(["France"])
    const [pending] = fieldLookup([frame], { type: "fieldLookup" }, {})
    expect(pending.fields).toEqual(frame.fields)
    expect(pending).not.toBe(frame)
    const [absent] = fieldLookup(
      [frame],
      { type: "fieldLookup", lookupField: "city" },
      gazetteers
    )
    expect(absent.fields).toEqual(frame.fields)
  })

  it("never changes its input", () => {
    const frame = table(["France"])
    const before = structuredClone(frame)
    fieldLookup([frame], { type: "fieldLookup" }, gazetteers)
    expect(frame).toEqual(before)
  })
})

describe("gazetteersNeeded", () => {
  it("lists each gazetteer that lookups in the transforms use, once", () => {
    expect(
      gazetteersNeeded([
        { type: "fieldLookup" },
        { type: "fieldLookup", gazetteer: "countries" },
        {
          type: "spatial",
          source: { mode: "lookup", gazetteer: "usa-states" },
        },
        { type: "spatial", source: { mode: "coords" } },
        { type: "spatial", target: { mode: "lookup" } },
      ])
    ).toEqual(["countries", "usa-states"])
    expect(gazetteersNeeded(undefined)).toEqual([])
    expect(gazetteersNeeded([{ type: "merge" }])).toEqual([])
  })
})
