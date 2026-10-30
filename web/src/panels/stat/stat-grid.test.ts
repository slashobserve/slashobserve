import { describe, expect, it } from "vitest"

import { isWideTile, tileSize, tilesAcross } from "./stat-grid"

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

describe("tileSize", () => {
  it("splits the width and height across the gaps", () => {
    expect(tileSize(408, 200, 2, 4)).toEqual({ width: 200, height: 96 })
    expect(tileSize(300, 100, 1, 1)).toEqual({ width: 300, height: 100 })
  })

  it("counts a partly filled last row", () => {
    expect(tileSize(300, 208, 3, 4)).toEqual({
      width: 94.66666666666667,
      height: 100,
    })
  })
})

describe("isWideTile", () => {
  it("needs the layout switched on", () => {
    expect(isWideTile(undefined, { width: 400, height: 80 })).toBe(false)
    expect(isWideTile(false, { width: 400, height: 80 })).toBe(false)
    expect(isWideTile(true, { width: 400, height: 80 })).toBe(true)
  })

  it("needs a tile at least twice as wide as tall and 200px wide", () => {
    expect(isWideTile(true, { width: 400, height: 201 })).toBe(false)
    expect(isWideTile(true, { width: 400, height: 200 })).toBe(true)
    expect(isWideTile(true, { width: 199, height: 40 })).toBe(false)
    expect(isWideTile(true, { width: 200, height: 40 })).toBe(true)
  })
})
