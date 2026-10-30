import { describe, expect, it } from "vitest"

import { parseNames, parseNumber, parseTags } from "./parse"
import type { NumberRule } from "./types"

const bucketSize: NumberRule = { min: 0, minExclusive: true, max: Infinity }
const bucketCount: NumberRule = { min: 1, max: 1000, integer: true }
const latitude: NumberRule = { min: -85, max: 85 }

describe("parseNumber with a positive size rule", () => {
  it.each([
    ["5", 5],
    ["0.5", 0.5],
    [".5", 0.5],
    ["5.", 5],
    ["1e3", 1000],
    ["2.5E-1", 0.25],
  ])("accepts %s", (entry, expected) => {
    expect(parseNumber(entry, bucketSize)).toBe(expected)
  })

  it.each(["0", "-1", "", "abc", "1,5", "1e999", "0x10", "1 2", "+1"])(
    "refuses %j",
    (entry) => {
      expect(parseNumber(entry, bucketSize)).toBeNull()
    }
  )
})

describe("parseNumber with a whole number rule", () => {
  it.each([
    ["1", 1],
    ["1000", 1000],
    ["250", 250],
  ])("accepts %s", (entry, expected) => {
    expect(parseNumber(entry, bucketCount)).toBe(expected)
  })

  it.each(["0", "1001", "1.5", "-3", "1e2", "", "x"])("refuses %j", (entry) => {
    expect(parseNumber(entry, bucketCount)).toBeNull()
  })
})

describe("parseNumber with a signed range", () => {
  it("accepts a negative value inside the range", () => {
    expect(parseNumber("-33.8", latitude)).toBe(-33.8)
  })

  it("accepts the bounds and refuses values beyond them", () => {
    expect(parseNumber("85", latitude)).toBe(85)
    expect(parseNumber("-85", latitude)).toBe(-85)
    expect(parseNumber("85.1", latitude)).toBeNull()
    expect(parseNumber("-86", latitude)).toBeNull()
  })

  it("reads minus zero as zero", () => {
    expect(Object.is(parseNumber("-0", latitude), 0)).toBe(true)
  })
})

describe("parseTags", () => {
  it("splits on commas, trims and drops empty items", () => {
    expect(parseTags(" prod, ,checkout.v2 ,, eu-west_1")).toEqual([
      "prod",
      "checkout.v2",
      "eu-west_1",
    ])
  })

  it("reads an empty entry as no tags", () => {
    expect(parseTags("")).toEqual([])
    expect(parseTags(" , ")).toEqual([])
  })

  it("keeps the first of repeated tags", () => {
    expect(parseTags("a, b, a")).toEqual(["a", "b"])
  })

  it.each(["Prod", "ok, Bad", "-lead", "has space", "a".repeat(41), "x;y"])(
    "refuses %j",
    (entry) => {
      expect(parseTags(entry)).toBeNull()
    }
  )

  it("accepts a tag of 40 characters", () => {
    expect(parseTags("a".repeat(40))).toEqual(["a".repeat(40)])
  })
})

describe("parseNames", () => {
  it("keeps the case and punctuation of field names", () => {
    expect(parseNames(" Pod, http_requests_total ,Zone A ,, pod")).toEqual([
      "Pod",
      "http_requests_total",
      "Zone A",
      "pod",
    ])
  })

  it("reads an empty entry as no names", () => {
    expect(parseNames(" , ")).toEqual([])
  })

  it("keeps the first of repeated names", () => {
    expect(parseNames("Pod, Zone, Pod")).toEqual(["Pod", "Zone"])
  })

  it("accepts a name of 200 characters and refuses 201", () => {
    expect(parseNames("A".repeat(200))).toEqual(["A".repeat(200)])
    expect(parseNames("A".repeat(201))).toBeNull()
  })
})
