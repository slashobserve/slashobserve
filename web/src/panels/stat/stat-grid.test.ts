import { describe, expect, it } from "vitest"

import { tilesAcross } from "./stat-grid"

describe("tilesAcross", () => {
  it("fits as many 120px tiles as the width allows when the height is plentiful", () => {
    expect(tilesAcross("auto", 600, 400, 8)).toBe(4)
    expect(tilesAcross("auto", 247, 400, 2)).toBe(1)
    expect(tilesAcross("auto", 248, 400, 2)).toBe(2)
  })

  it("never makes more columns than tiles", () => {
    expect(tilesAcross("auto", 1200, 400, 3)).toBe(3)
    expect(tilesAcross("auto", 1200, 400, 1)).toBe(1)
    expect(tilesAcross("auto", 1200, 400, 0)).toBe(1)
  })

  it("squeezes tiles narrower so every row fits the height", () => {
    expect(tilesAcross("auto", 362, 109, 3)).toBe(3)
    expect(tilesAcross("auto", 362, 109, 2)).toBe(2)
    expect(tilesAcross("auto", 362, 200, 4)).toBe(2)
    expect(tilesAcross("auto", 362, 200, 7)).toBe(3)
  })

  it("keeps tiles at least 96px wide even when the rows still do not fit", () => {
    expect(tilesAcross("auto", 362, 109, 12)).toBe(3)
    expect(tilesAcross("auto", 200, 109, 6)).toBe(2)
    expect(tilesAcross("auto", 150, 109, 6)).toBe(1)
  })

  it("does not squeeze when the rows already fit", () => {
    expect(tilesAcross("auto", 362, 160, 3)).toBe(2)
    expect(tilesAcross("auto", 362, 80, 2)).toBe(2)
  })

  it("keeps a horizontal row and a vertical column as the orientation says", () => {
    expect(tilesAcross("horizontal", 362, 109, 5)).toBe(5)
    expect(tilesAcross("vertical", 1200, 400, 5)).toBe(1)
  })
})
