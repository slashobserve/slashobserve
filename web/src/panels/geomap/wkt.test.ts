import { describe, expect, it } from "vitest"

import { parseWktPoint } from "./wkt"

describe("parseWktPoint", () => {
  it.each([
    ["POINT (2.35 48.85)", { lon: 2.35, lat: 48.85 }],
    ["point(-74 40.7)", { lon: -74, lat: 40.7 }],
    ["  Point  (  10   -20  )  ", { lon: 10, lat: -20 }],
    ["SRID=4326;POINT (1 2)", { lon: 1, lat: 2 }],
    ["srid=3857; POINT(+1.5e1 .5)", { lon: 15, lat: 0.5 }],
    ["POINT (5. 6)", { lon: 5, lat: 6 }],
  ])("reads %s", (text, expected) => {
    expect(parseWktPoint(text)).toEqual(expected)
  })

  it.each([
    ["LINESTRING (0 0, 1 1)"],
    ["POLYGON ((0 0, 1 0, 1 1, 0 0))"],
    ["MULTIPOINT ((1 2), (3 4))"],
    ["POINT EMPTY"],
    ["POINT (1)"],
    ["POINT (1 2 3)"],
    ["POINT Z (1 2 3)"],
    ["POINT (a b)"],
    ["POINT (1 2"],
    ["POINT (1, 2)"],
    ["SRID=abc;POINT (1 2)"],
    ["10 20"],
    [""],
  ])("refuses %j", (text) => {
    expect(parseWktPoint(text)).toBeNull()
  })

  it("refuses a value that is not text", () => {
    expect(parseWktPoint(null)).toBeNull()
    expect(parseWktPoint(42)).toBeNull()
    expect(parseWktPoint(undefined)).toBeNull()
  })
})
