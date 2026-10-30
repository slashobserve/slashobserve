import { beforeAll, describe, expect, it } from "vitest"

import { buildGazetteer, type Gazetteers } from "../geomap/gazetteer/lookup"
import { atlasFeatures } from "../geomap/world"
import type { PreparedFrame } from "./names"
import { spatial } from "./spatial"

type Options = Parameters<typeof spatial>[1]

let gazetteers: Gazetteers

beforeAll(async () => {
  gazetteers = {
    countries: buildGazetteer("countries", await atlasFeatures("countries")),
  }
})

function points(lat: unknown[], lon: unknown[]): PreparedFrame {
  return {
    ref: "A",
    name: "route",
    fields: [
      { name: "lat", type: "number", values: lat },
      { name: "lon", type: "number", values: lon },
      { name: "load", type: "number", values: lat.map((_, i) => i * 10) },
    ],
  }
}

function values(frame: PreparedFrame, name: string): unknown[] {
  return frame.fields.find((field) => field.name === name)?.values ?? []
}

function run(
  frame: PreparedFrame,
  options: Omit<Options, "type">,
  loaded: Gazetteers = gazetteers
): PreparedFrame[] {
  return spatial([frame], { type: "spatial", ...options }, loaded)
}

describe("spatial prepare", () => {
  it("adds lat and lon from fields found by name, in any case", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "Latitude", type: "number", values: [10, 20] },
        { name: "Lng", type: "number", values: [30, 40] },
      ],
    }
    const [out] = run(frame, {})
    expect(out.fields.map((field) => field.name)).toEqual([
      "Latitude",
      "Lng",
      "lat",
      "lon",
    ])
    expect(values(out, "lat")).toEqual([10, 20])
    expect(values(out, "lon")).toEqual([30, 40])
  })

  it("ignores field names in automatic mode", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "y", type: "number", values: [1] },
        { name: "x", type: "number", values: [2] },
      ],
    }
    const [out] = run(frame, {
      source: { mode: "auto", latitude: "y", longitude: "x" },
    })
    expect(out.fields).toEqual(frame.fields)
  })

  it("reads the fields a coords source names and parses numeric text", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "y", type: "string", values: ["51.5", " 48.8 ", "x"] },
        { name: "x", type: "number", values: [-0.1, 2.3, 4] },
      ],
    }
    const [out] = run(frame, {
      source: { mode: "coords", latitude: "y", longitude: "x" },
    })
    expect(values(out, "lat")).toEqual([51.5, 48.8, null])
    expect(values(out, "lon")).toEqual([-0.1, 2.3, null])
  })

  it("leaves a row without a valid place null in both fields", () => {
    const [out] = run(points([95, 10, null, 20], [10, 200, 30, 40]), {})
    expect(values(out, "lat")).toEqual([null, null, null, 20])
    expect(values(out, "lon")).toEqual([null, null, null, 40])
  })

  it("replaces the lat and lon fields it finds instead of repeating them", () => {
    const [out] = run(points([1, 2], [3, 4]), {})
    expect(out.fields.map((field) => field.name)).toEqual([
      "lat",
      "lon",
      "load",
    ])
  })

  it("decodes a geohash field", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [{ name: "geohash", type: "string", values: ["ezs42", "!", 7] }],
    }
    const [out] = run(frame, { source: { mode: "geohash" } })
    expect(values(out, "lat")[0]).toBeCloseTo(42.6, 1)
    expect(values(out, "lon")[0]).toBeCloseTo(-5.6, 1)
    expect(values(out, "lat").slice(1)).toEqual([null, null])
    const [auto] = run(frame, {})
    expect(values(auto, "lat")).toEqual(values(out, "lat"))
  })

  it("looks a text field up in the gazetteer", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "value", type: "number", values: [1, 2, 3] },
        {
          name: "country",
          type: "string",
          values: ["France", "FR", "Nowhere"],
        },
      ],
    }
    const [out] = run(frame, {
      source: { mode: "lookup", lookup: "country", gazetteer: "countries" },
    })
    const lat = values(out, "lat")
    expect(lat[0]).toBeGreaterThan(41.3)
    expect(lat[0]).toBeLessThan(51.1)
    expect(lat[1]).toEqual(lat[0])
    expect(lat[2]).toBeNull()
  })

  it("changes nothing when a gazetteer has not loaded or a field is missing", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [{ name: "country", type: "string", values: ["France"] }],
    }
    const lookup = { mode: "lookup" as const, lookup: "country" }
    const pending = run(frame, { source: lookup }, {})
    expect(pending[0].fields).toEqual(frame.fields)
    const other = run(frame, { source: { ...lookup, lookup: "city" } })
    expect(other[0].fields).toEqual(frame.fields)
    const noPlaces = run(frame, {})
    expect(noPlaces[0].fields).toEqual(frame.fields)
    expect(noPlaces[0]).not.toBe(frame)
  })
})

describe("spatial calculate", () => {
  it("measures heading and distance from the previous located row", () => {
    const frame = points([0, 0, 1, null, 0], [0, 1, 1, 5, 1])
    const [heading] = run(frame, { action: "calculate", calc: "heading" })
    expect(values(heading, "Heading")).toEqual([null, 90, 0, null, 180])
    const [distance] = run(frame, { action: "calculate", calc: "distance" })
    const meters = values(distance, "Distance") as (number | null)[]
    expect(meters[0]).toBeNull()
    expect(meters[1]).toBeCloseTo(111195, 0)
    expect(meters[2]).toBeCloseTo(111195, 0)
    expect(meters[3]).toBeNull()
    expect(meters[4]).toBeCloseTo(111195, 0)
  })

  it("is heading when no calculation is named, with units on the new field", () => {
    const [out] = run(points([0, 0], [0, 1]), { action: "calculate" })
    const added = out.fields.at(-1)
    expect(added?.name).toBe("Heading")
    expect(added?.config?.unit).toBe("degree")
    const [metres] = run(points([0, 0], [0, 1]), {
      action: "calculate",
      calc: "distance",
      field: "Leg",
    })
    expect(metres.fields.at(-1)?.name).toBe("Leg")
    expect(metres.fields.at(-1)?.config?.unit).toBe("lengthm")
  })

  it("matches the known distance and bearing from London to Paris", () => {
    const frame = points([51.5074, 48.8566], [-0.1278, 2.3522])
    const [distance] = run(frame, { action: "calculate", calc: "distance" })
    const meters = values(distance, "Distance")[1] as number
    expect(meters).toBeGreaterThan(343000)
    expect(meters).toBeLessThan(344500)
    const [heading] = run(frame, { action: "calculate", calc: "heading" })
    expect(values(heading, "Heading")[1] as number).toBeCloseTo(148.1, 0)
  })

  it("gives the area of the polygon the rows outline on every row", () => {
    const square = points([0, 0, 1, 1], [0, 1, 1, 0])
    const [out] = run(square, { action: "calculate", calc: "area" })
    const areas = values(out, "Area") as number[]
    expect(new Set(areas).size).toBe(1)
    expect(areas[0]).toBeGreaterThan(1.23e10)
    expect(areas[0]).toBeLessThan(1.24e10)
    expect(out.fields.at(-1)?.config?.unit).toBe("areaM2")
    const [line] = run(points([0, 1], [0, 1]), {
      action: "calculate",
      calc: "area",
    })
    expect(values(line, "Area")).toEqual([null, null])
  })
})

describe("spatial modify", () => {
  it("keeps the located rows in order as the points of one line", () => {
    const frame = points([1, null, 3], [4, 5, 6])
    const out = run(frame, { action: "modify", modify: "asLine" })
    expect(out).toHaveLength(1)
    expect(values(out[0], "lat")).toEqual([1, 3])
    expect(values(out[0], "lon")).toEqual([4, 6])
    expect(values(out[0], "load")).toEqual([0, 20])
  })

  it("builds one two-point line per row from the source to the target", () => {
    const frame: PreparedFrame = {
      ref: "A",
      name: "flows",
      fields: [
        { name: "lat", type: "number", values: [1, 2, 3] },
        { name: "lon", type: "number", values: [10, 20, 30] },
        { name: "lat2", type: "number", values: [4, null, 6] },
        { name: "lon2", type: "number", values: [40, 50, 60] },
        { name: "rate", type: "number", values: [7, 8, 9] },
      ],
    }
    const out = run(frame, {
      action: "modify",
      modify: "lineBuilder",
      target: { mode: "coords", latitude: "lat2", longitude: "lon2" },
    })
    expect(out).toHaveLength(2)
    expect(out.map((line) => line.name)).toEqual(["flows", "flows"])
    expect(values(out[0], "lat")).toEqual([1, 4])
    expect(values(out[0], "lon")).toEqual([10, 40])
    expect(values(out[0], "rate")).toEqual([7, 7])
    expect(values(out[1], "lat")).toEqual([3, 6])
    expect(values(out[1], "lon")).toEqual([30, 60])
  })

  it("builds lines when no modification is named, and keeps the frame whole when the target is missing", () => {
    const frame = points([1], [2])
    const [whole] = run(frame, {
      action: "modify",
      target: { mode: "coords", latitude: "nope", longitude: "lon" },
    })
    expect(whole.fields).toEqual(frame.fields)
    const lines = run(frame, {
      action: "modify",
      target: { mode: "coords", latitude: "lat", longitude: "lon" },
    })
    expect(values(lines[0], "lat")).toEqual([1, 1])
  })
})

describe("spatial purity", () => {
  it("never changes its input frames", () => {
    const frame = points([0, 1, null], [0, 1, 2])
    const before = structuredClone(frame)
    for (const options of [
      {},
      { action: "calculate" as const, calc: "area" as const },
      { action: "modify" as const, modify: "asLine" as const },
    ]) {
      run(frame, options)
    }
    expect(frame).toEqual(before)
  })
})
