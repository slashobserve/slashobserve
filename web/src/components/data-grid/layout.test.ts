import { describe, expect, it } from "vitest"

import { fillHeight, gridLayout, minGridHeight } from "./layout"

describe("gridLayout", () => {
  it("grows with its rows up to 100", () => {
    expect(gridLayout(0)).toBe("autoHeight")
    expect(gridLayout(100)).toBe("autoHeight")
  })

  it("virtualises above 100 rows", () => {
    expect(gridLayout(101)).toBe("normal")
    expect(gridLayout(5000)).toBe("normal")
  })
})

describe("fillHeight", () => {
  it("fills the viewport below the grid's top, leaving room for a note", () => {
    expect(fillHeight(1000, 200)).toBe(752)
  })

  it("never grows past the visible height when the top is above it", () => {
    expect(fillHeight(1000, -4000)).toBe(952)
  })

  it("keeps a minimum when little of the viewport is left", () => {
    expect(fillHeight(900, 700)).toBe(minGridHeight)
    expect(fillHeight(900, 2000)).toBe(minGridHeight)
  })
})
