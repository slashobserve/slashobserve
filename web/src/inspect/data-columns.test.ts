import { describe, expect, it } from "vitest"

import { dataColumnWidth } from "./data-columns"

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
