import { describe, expect, it } from "vitest"

import { pixelHeight, rowsFor, stackedHeight } from "./stack-heights"

describe("stackedHeight", () => {
  it.each([
    ["keeps a saved height taller than the minimum", 8, 240, 310],
    ["raises a short panel to its type's minimum", 4, 280, 280],
    ["keeps the saved height when the type has no minimum", 2, undefined, 70],
    ["keeps an exact match", 8, 310, 310],
  ])("%s", (_name, h, minHeight, expected) => {
    expect(stackedHeight(h, minHeight)).toBe(expected)
  })
})

describe("rowsFor", () => {
  it.each([
    [280, 8],
    [310, 8],
    [311, 9],
    [70, 2],
    [1, 1],
    [0, 1],
  ])("covers %d pixels with %d grid rows", (pixels, rows) => {
    expect(rowsFor(pixels)).toBe(rows)
    expect(pixelHeight(rows)).toBeGreaterThanOrEqual(pixels)
  })

  it("answers the same rows for a saved height", () => {
    for (let h = 1; h <= 40; h++) expect(rowsFor(pixelHeight(h))).toBe(h)
  })
})
