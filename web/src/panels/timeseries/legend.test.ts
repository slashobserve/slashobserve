import { describe, expect, it } from "vitest"

import { legendMaxHeight } from "./legend"

describe("legendMaxHeight", () => {
  it("rounds down to whole legend rows", () => {
    expect(legendMaxHeight(60, 16)).toBe(48)
    expect(legendMaxHeight(48, 16)).toBe(48)
    expect(legendMaxHeight(47, 16)).toBe(26)
    expect(legendMaxHeight(70, 16)).toBe(70)
    expect(legendMaxHeight(100, 16)).toBe(92)
  })

  it("always leaves one row", () => {
    expect(legendMaxHeight(0, 16)).toBe(26)
    expect(legendMaxHeight(10, 16)).toBe(26)
  })

  it("scales with the root font size", () => {
    expect(legendMaxHeight(70, 20)).toBeCloseTo(60)
  })
})
