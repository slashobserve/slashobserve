import { describe, expect, it } from "vitest"

import { gridLines } from "./lines"

const rows = [{ id: "a" }, { id: "b" }]
const rowId = (row: { id: string }) => row.id

describe("gridLines", () => {
  it("is one line per row when no detail is open", () => {
    expect(gridLines(rows, rowId, undefined)).toEqual([
      { id: "a", row: rows[0], detail: false },
      { id: "b", row: rows[1], detail: false },
    ])
  })

  it("puts a detail line directly below each open row", () => {
    expect(
      gridLines(rows, rowId, new Set(["a"])).map((line) => [
        line.id,
        line.detail,
      ])
    ).toEqual([
      ["a", false],
      ["a#detail", true],
      ["b", false],
    ])
  })

  it("ignores an open id that no row has", () => {
    expect(gridLines(rows, rowId, new Set(["z"]))).toHaveLength(2)
  })
})
