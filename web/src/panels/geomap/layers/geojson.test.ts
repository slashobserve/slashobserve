import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { project } from "../projection"
import {
  geojsonModel,
  geojsonUrlUsable,
  maxCoordinates,
  maxFeatures,
  parseGeojson,
  remoteGeojson,
  retryGeojson,
  ruleMatches,
  unwrapRing,
  type Feature,
} from "./geojson"

const paint = (value: unknown) =>
  typeof value === "string" ? `var(--${value})` : undefined

function point(properties: Record<string, unknown>, lon = 10, lat = 20) {
  return {
    type: "Feature",
    properties,
    geometry: { type: "Point", coordinates: [lon, lat] },
  }
}

function square(lon: number, lat: number, size = 1): Feature {
  return {
    properties: {},
    geometry: {
      type: "Polygon",
      coordinates: [
        [
          [lon, lat],
          [lon + size, lat],
          [lon + size, lat + size],
          [lon, lat + size],
          [lon, lat],
        ],
      ],
    },
  }
}

describe("parseGeojson", () => {
  it("reads a feature collection", () => {
    const features = parseGeojson({
      type: "FeatureCollection",
      features: [point({ name: "a" }), point({ name: "b" })],
    })
    expect(features.map((feature) => feature.properties.name)).toEqual([
      "a",
      "b",
    ])
  })

  it("reads a single feature, keeping its id", () => {
    const features = parseGeojson({ ...point({}), id: 7 })
    expect(features).toHaveLength(1)
    expect(features[0].id).toBe(7)
  })

  it("reads a bare geometry", () => {
    expect(
      parseGeojson({
        type: "LineString",
        coordinates: [
          [0, 0],
          [1, 1],
        ],
      })
    ).toHaveLength(1)
  })

  it("gives a geometry collection's members the feature's properties", () => {
    const features = parseGeojson({
      type: "Feature",
      properties: { name: "both" },
      geometry: {
        type: "GeometryCollection",
        geometries: [
          { type: "Point", coordinates: [0, 0] },
          { type: "Point", coordinates: [1, 1] },
        ],
      },
    })
    expect(features).toHaveLength(2)
    expect(
      features.every((feature) => feature.properties.name === "both")
    ).toBe(true)
  })

  it("returns nothing for text, numbers, null and unknown types", () => {
    for (const raw of [
      "x",
      3,
      null,
      undefined,
      [],
      {},
      { type: "Topology" },
      { type: "FeatureCollection" },
      { type: "Point" },
      { type: "Circle", coordinates: [0, 0] },
    ]) {
      expect(parseGeojson(raw)).toEqual([])
    }
  })

  it("stops at deeply nested collections", () => {
    let nested: unknown = { type: "Point", coordinates: [0, 0] }
    for (let i = 0; i < 10; i++) {
      nested = { type: "FeatureCollection", features: [nested] }
    }
    expect(parseGeojson(nested)).toEqual([])
  })
})

describe("ruleMatches", () => {
  const rule = (operation: string, value: string | number) => ({
    property: "p",
    operation,
    value,
  })

  it("compares equality as text for string values and as numbers for numbers", () => {
    expect(ruleMatches(rule("eq", "a"), { p: "a" })).toBe(true)
    expect(ruleMatches(rule("eq", "a"), { p: "b" })).toBe(false)
    expect(ruleMatches(rule("eq", "3"), { p: 3 })).toBe(true)
    expect(ruleMatches(rule("eq", 3), { p: "3" })).toBe(true)
    expect(ruleMatches(rule("eq", 3), { p: 4 })).toBe(false)
  })

  it("negates equality, and a missing property is not equal", () => {
    expect(ruleMatches(rule("neq", "a"), { p: "b" })).toBe(true)
    expect(ruleMatches(rule("neq", "a"), { p: "a" })).toBe(false)
    expect(ruleMatches(rule("neq", "a"), {})).toBe(true)
    expect(ruleMatches(rule("eq", "a"), {})).toBe(false)
  })

  it("orders numbers", () => {
    expect(ruleMatches(rule("lt", 5), { p: 4 })).toBe(true)
    expect(ruleMatches(rule("lt", 5), { p: 5 })).toBe(false)
    expect(ruleMatches(rule("lte", 5), { p: 5 })).toBe(true)
    expect(ruleMatches(rule("gt", 5), { p: 6 })).toBe(true)
    expect(ruleMatches(rule("gt", 5), { p: 5 })).toBe(false)
    expect(ruleMatches(rule("gte", 5), { p: 5 })).toBe(true)
    expect(ruleMatches(rule("gte", "5"), { p: "7" })).toBe(true)
  })

  it("never matches an ordering on a missing or non-numeric property", () => {
    for (const operation of ["lt", "lte", "gt", "gte"]) {
      expect(ruleMatches(rule(operation, 5), {})).toBe(false)
      expect(ruleMatches(rule(operation, 5), { p: "abc" })).toBe(false)
      expect(ruleMatches(rule(operation, 5), { p: "" })).toBe(false)
      expect(ruleMatches(rule(operation, 5), { p: null })).toBe(false)
    }
  })

  it("never matches an unknown operation", () => {
    expect(ruleMatches(rule("between", 5), { p: 5 })).toBe(false)
  })
})

describe("geojsonModel rules", () => {
  const features: Feature[] = parseGeojson({
    type: "FeatureCollection",
    features: [point({ n: 5 }), point({ n: 50 })],
  })

  it("colors a feature by the first rule that matches", () => {
    const model = geojsonModel(
      features,
      {
        rules: [
          { property: "n", operation: "gte", value: 10, color: "first" },
          { property: "n", operation: "gte", value: 1, color: "second" },
        ],
      },
      paint
    )
    expect(model.features[0].style.stroke).toBe("var(--second)")
    expect(model.features[1].style.stroke).toBe("var(--first)")
  })

  it("fills at half opacity unless the rule sets one", () => {
    const rules = (opacity?: number) => ({
      rules: [
        { property: "n", operation: "gt", value: 0, color: "c", opacity },
      ],
    })
    expect(geojsonModel(features, rules(), paint).features[0].style).toEqual({
      fill: "var(--c)",
      fillOpacity: 0.5,
      stroke: "var(--c)",
      strokeOpacity: 1,
    })
    expect(
      geojsonModel(features, rules(0.2), paint).features[0].style.fillOpacity
    ).toBe(0.2)
  })

  it("draws an outline in the foreground with no fill by default", () => {
    expect(geojsonModel(features, {}, paint).features[0].style).toEqual({
      fill: null,
      fillOpacity: 0,
      stroke: "var(--foreground)",
      strokeOpacity: 0.6,
    })
  })

  it("falls back to the outline when the rule's color cannot be painted", () => {
    const model = geojsonModel(
      features,
      { rules: [{ property: "n", operation: "gt", value: 0, color: 3 }] },
      paint
    )
    expect(model.features[0].style.fill).toBeNull()
  })

  it("ignores rules that are not well formed", () => {
    const model = geojsonModel(
      features,
      { rules: [null, "x", { property: "n" }, { operation: "eq", value: 1 }] },
      paint
    )
    expect(model.features[0].style.fill).toBeNull()
  })
})

describe("geojsonModel drawing", () => {
  it("draws points as dots and lines and polygons as paths", () => {
    const model = geojsonModel(
      [
        ...parseGeojson(point({}, 10, 20)),
        ...parseGeojson({
          type: "LineString",
          coordinates: [
            [0, 0],
            [10, 10],
          ],
        }),
        square(0, 0),
      ],
      {},
      paint
    )
    expect(model.features[0].dots).toEqual([project(10, 20)])
    expect(model.features[0].path).toBe("")
    expect(model.features[1].path).toMatch(/^M[-\d.e]+ [-\d.e]+L/)
    expect(model.features[1].path).not.toContain("Z")
    expect(model.features[2].path.endsWith("Z")).toBe(true)
  })

  it("reports the extent of what it drew as two corners", () => {
    const model = geojsonModel(
      [square(10, 20, 5), square(-30, -5, 5)],
      {},
      paint
    )
    expect(model.corners).toEqual([
      { lon: -30, lat: -5 },
      { lon: 15, lat: 25 },
    ])
  })

  it("gives each feature bounds in the unit square", () => {
    const [drawn] = geojsonModel([square(0, 0, 10)], {}, paint).features
    const low = project(0, 10)
    const high = project(10, 0)
    expect(drawn.bounds).toEqual({
      minX: low.x,
      minY: low.y,
      maxX: high.x,
      maxY: high.y,
    })
  })

  it("names a feature by its name property, else by its place in the list", () => {
    const model = geojsonModel(
      [
        ...parseGeojson(point({ Name: "Paris", population: 2 })),
        ...parseGeojson(point({})),
      ],
      {},
      paint
    )
    expect(model.features[0].tip.label).toBe("Paris")
    expect(model.features[0].tip.details).toEqual([
      { name: "Name", text: "Paris" },
      { name: "population", text: "2" },
    ])
    expect(model.features[1].tip.label).toBe("Feature 2")
  })

  it("clips long property text", () => {
    const [drawn] = geojsonModel(
      parseGeojson(point({ note: "x".repeat(500) })),
      {},
      paint
    ).features
    expect(drawn.tip.details[0].text).toHaveLength(123)
    expect(drawn.tip.details[0].text.endsWith("...")).toBe(true)
  })

  it("skips features with no usable coordinates", () => {
    const model = geojsonModel(
      [
        {
          properties: {},
          geometry: { type: "Point", coordinates: ["a", "b"] as never },
        },
        {
          properties: {},
          geometry: { type: "LineString", coordinates: [] },
        },
      ],
      {},
      paint
    )
    expect(model.features).toEqual([])
    expect(model.corners).toEqual([])
    expect(model.tooLarge).toBe(false)
  })

  it("draws a ring that crosses the antimeridian twice, once on each side", () => {
    const ring = [
      [170, 0],
      [-170, 0],
      [-170, 10],
      [170, 10],
      [170, 0],
    ]
    const [drawn] = geojsonModel(
      [{ properties: {}, geometry: { type: "Polygon", coordinates: [ring] } }],
      {},
      paint
    ).features
    expect(drawn.path.match(/Z/g)).toHaveLength(2)
    expect(drawn.bounds.minX).toBeLessThan(0.01)
    expect(drawn.bounds.maxX).toBeGreaterThan(0.99)
  })

  it("draws a ring that stays on one side once", () => {
    const [drawn] = geojsonModel([square(0, 0)], {}, paint).features
    expect(drawn.path.match(/Z/g)).toHaveLength(1)
  })

  it("starts a new subpath when an open line jumps across the antimeridian", () => {
    const [drawn] = geojsonModel(
      parseGeojson({
        type: "LineString",
        coordinates: [
          [170, 0],
          [-170, 0],
        ],
      }),
      {},
      paint
    ).features
    expect(drawn.path.match(/M/g)).toHaveLength(2)
  })
})

describe("geojsonModel caps", () => {
  it("draws 5,000 features", () => {
    const features = Array.from({ length: maxFeatures }, () => square(0, 0))
    const model = geojsonModel(features, {}, paint)
    expect(model.tooLarge).toBe(false)
    expect(model.features).toHaveLength(maxFeatures)
  })

  it("draws nothing for 5,001 features", () => {
    const features = Array.from({ length: maxFeatures + 1 }, () => square(0, 0))
    expect(geojsonModel(features, {}, paint)).toEqual({
      features: [],
      corners: [],
      tooLarge: true,
    })
  })

  it("draws nothing once the coordinates pass 200,000", () => {
    const line = (count: number): Feature => ({
      properties: {},
      geometry: {
        type: "LineString",
        coordinates: Array.from({ length: count }, (_, i) => [i % 180, 0]),
      },
    })
    expect(geojsonModel([line(maxCoordinates)], {}, paint).tooLarge).toBe(false)
    expect(geojsonModel([line(maxCoordinates), line(1)], {}, paint)).toEqual({
      features: [],
      corners: [],
      tooLarge: true,
    })
  })
})

describe("unwrapRing", () => {
  it("makes longitudes continuous across the antimeridian", () => {
    expect(
      unwrapRing([
        [170, 0],
        [-170, 1],
        [-160, 2],
      ]).map(([lon]) => lon)
    ).toEqual([170, 190, 200])
    expect(
      unwrapRing([
        [-170, 0],
        [170, 1],
      ]).map(([lon]) => lon)
    ).toEqual([-170, -190])
  })

  it("leaves a ring that does not cross alone", () => {
    expect(
      unwrapRing([
        [0, 0],
        [10, 10],
      ])
    ).toEqual([
      [0, 0],
      [10, 10],
    ])
  })
})

describe("geojsonUrlUsable", () => {
  it("accepts remote https URLs only", () => {
    expect(geojsonUrlUsable("https://data.example/a.geojson")).toBe(true)
    expect(geojsonUrlUsable("http://data.example/a.geojson")).toBe(false)
    expect(geojsonUrlUsable("/local.geojson")).toBe(false)
    expect(geojsonUrlUsable("javascript:alert(1)")).toBe(false)
    expect(geojsonUrlUsable("")).toBe(false)
  })
})

describe("retryGeojson", () => {
  it("never retries a 4xx", () => {
    expect(
      retryGeojson(0, new ApiError(403, "outbound_domain_not_allowed", "x"))
    ).toBe(false)
  })

  it("retries other failures twice", () => {
    expect(retryGeojson(0, new ApiError(502, "bad_gateway", "x"))).toBe(true)
    expect(retryGeojson(1, new TypeError("network"))).toBe(true)
    expect(retryGeojson(2, new TypeError("network"))).toBe(false)
  })
})

describe("remoteGeojson", () => {
  const url = "https://Data.Example/a.geojson"
  const loaded = parseGeojson(point({}))

  it("is ready with data", () => {
    expect(
      remoteGeojson({ status: "success", data: loaded, error: null }, url)
    ).toEqual({ state: "ready", features: loaded })
  })

  it("is loading while pending", () => {
    expect(
      remoteGeojson({ status: "pending", data: undefined, error: null }, url)
    ).toEqual({ state: "loading" })
  })

  it("names the host when the allowlist refuses it", () => {
    const error = new ApiError(403, "outbound_domain_not_allowed", "x")
    expect(
      remoteGeojson({ status: "error", data: undefined, error }, url)
    ).toEqual({ state: "blocked", host: "data.example" })
  })

  it("fails for any other error", () => {
    const error = new ApiError(502, "upstream_failed", "x")
    expect(
      remoteGeojson({ status: "error", data: undefined, error }, url)
    ).toEqual({ state: "failed" })
    expect(
      remoteGeojson(
        { status: "error", data: undefined, error: new Error("x") },
        url
      )
    ).toEqual({ state: "failed" })
  })
})
