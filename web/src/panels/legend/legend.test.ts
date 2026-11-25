import { describe, expect, it } from "vitest"

import { legendMaxHeight, legendTableFit } from "./legend"

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

describe("legendTableFit", () => {
  it("ends on a whole row so no row is cut mid-line", () => {
    expect(legendTableFit(90, 16, 5)).toEqual({
      maxHeight: 86,
      scrolls: true,
      footer: true,
    })
  })

  it("does not flag a table whose rows all fit", () => {
    expect(legendTableFit(200, 16, 3)).toEqual({
      maxHeight: 186,
      scrolls: false,
      footer: false,
    })
  })

  it("shows one row and no footer when only one row fits", () => {
    expect(legendTableFit(0, 16, 3)).toEqual({
      maxHeight: 46,
      scrolls: true,
      footer: false,
    })
    expect(legendTableFit(60, 16, 3)).toEqual({
      maxHeight: 46,
      scrolls: true,
      footer: false,
    })
  })

  it("never exceeds the available height beyond the one-row minimum", () => {
    for (const available of [0, 46, 60, 66, 90, 130, 300]) {
      const { maxHeight } = legendTableFit(available, 16, 9)
      expect(maxHeight).toBeLessThanOrEqual(Math.max(available, 46))
    }
  })
})
