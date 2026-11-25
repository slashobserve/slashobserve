import { describe, expect, it } from "vitest"

import { dataColumnWidth, fitColumnWidths } from "./data-columns"

describe("dataColumnWidth", () => {
  it("fits the longest of the header and the first rows", () => {
    const rows = [["2026-11-21 10:00:00"], ["2026-11-21 10:00:15"]]
    expect(dataColumnWidth("Time", rows, 0)).toBe(Math.ceil(19 * 7.5) + 28)
    expect(dataColumnWidth("A much longer header text", [["1"]], 0)).toBe(
      Math.ceil(25 * 7.5) + 28
    )
  })

  it("keeps a column between a floor and a ceiling", () => {
    expect(dataColumnWidth("v", [["1"]], 0)).toBe(64)
    expect(dataColumnWidth("v", [["x".repeat(500)]], 0)).toBe(320)
  })

  it("reads only the first fifty rows and treats a missing value as empty", () => {
    const rows = [
      ...Array.from({ length: 50 }, () => ["short"]),
      ["x".repeat(500)],
    ]
    expect(dataColumnWidth("v", rows, 0)).toBe(Math.ceil(5 * 7.5) + 28)
    expect(dataColumnWidth("Value", [[]], 0)).toBe(Math.ceil(5 * 7.5) + 28)
  })
})

describe("fitColumnWidths", () => {
  it("keeps the natural widths when they already fit", () => {
    expect(fitColumnWidths([100, 150], 400)).toEqual([100, 150])
    expect(fitColumnWidths([100, 150], 250)).toEqual([100, 150])
  })

  it("keeps the natural widths until the panel has been measured", () => {
    expect(fitColumnWidths([300, 300], 0)).toEqual([300, 300])
  })

  it("squeezes the wide columns to fit when the narrow ones leave room", () => {
    const fitted = fitColumnWidths([64, 320, 320], 480)
    expect(fitted.reduce((sum, width) => sum + width, 0)).toBeLessThanOrEqual(
      480
    )
    expect(fitted[0]).toBe(64)
    expect(Math.min(...fitted)).toBeGreaterThanOrEqual(64)
    expect(fitted[1]).toBeLessThan(320)
  })

  it("keeps the natural widths and scrolls when even squeezed columns overflow", () => {
    expect(fitColumnWidths([200, 200, 200, 200, 200], 400)).toEqual([
      200, 200, 200, 200, 200,
    ])
  })
})
