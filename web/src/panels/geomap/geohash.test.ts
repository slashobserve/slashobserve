import { describe, expect, it } from "vitest"

import { decodeGeohash } from "./geohash"

describe("decodeGeohash", () => {
  it("returns the center of a long cell", () => {
    const point = decodeGeohash("u4pruydqqvj")
    expect(point?.lat).toBeCloseTo(57.64911, 4)
    expect(point?.lon).toBeCloseTo(10.40744, 4)
  })

  it("returns the center of a short cell", () => {
    const point = decodeGeohash("ezs42")
    expect(point?.lat).toBeCloseTo(42.605, 3)
    expect(point?.lon).toBeCloseTo(-5.603, 3)
  })

  it("accepts upper case and surrounding spaces", () => {
    expect(decodeGeohash(" EZS42 ")).toEqual(decodeGeohash("ezs42"))
  })

  it("rejects characters outside the geohash alphabet", () => {
    expect(decodeGeohash("ezs4a")).toBeNull()
    expect(decodeGeohash("ezs 42")).toBeNull()
  })

  it("rejects empty and overlong text", () => {
    expect(decodeGeohash("")).toBeNull()
    expect(decodeGeohash("u4pruydqqvjkk")).toBeNull()
  })
})
