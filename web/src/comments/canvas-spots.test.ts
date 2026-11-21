import { describe, expect, it } from "vitest"

import { ownsSpot, spotKeyOf } from "./canvas-spots"

const owner = { ownerId: "b1", surfaceId: "s1", copy: false }

describe("ownsSpot", () => {
  it("owns a spot opened on its own surface", () => {
    expect(ownsSpot({ ownerId: "b1", surface: "s1" }, owner)).toBe(true)
    expect(ownsSpot({ ownerId: "b1", surface: "s2" }, owner)).toBe(false)
    expect(ownsSpot({ ownerId: "b2", surface: "s1" }, owner)).toBe(false)
    expect(ownsSpot(null, owner)).toBe(false)
  })

  it("owns a spot opened by key unless it is a repeated copy", () => {
    expect(ownsSpot({ ownerId: "b1", surface: null }, owner)).toBe(true)
    expect(
      ownsSpot({ ownerId: "b1", surface: null }, { ...owner, copy: true })
    ).toBe(false)
  })
})

describe("spotKeyOf", () => {
  it("tells two spots of one owner apart by where they opened", () => {
    expect(
      spotKeyOf({ ownerId: "b1", surface: "s1", at: { x: 1, y: 2 } })
    ).toBe("b1:1:2")
    expect(spotKeyOf({ ownerId: "b1", surface: null, at: null })).toBe("b1::")
  })
})
