import { describe, expect, it } from "vitest"

import { hitTest, type TableAnchorLayout } from "./anchors"

const layout: TableAnchorLayout = {
  body: { width: 400, height: 200 },
  header: { y: 0, height: 32 },
  columns: [
    { name: "service", left: 0, width: 100, dimension: true },
    { name: "span", left: 100, width: 150, dimension: true },
    { name: "errors", left: 250, width: 100, dimension: false },
  ],
  rows: [
    { top: 32, height: 25, cells: ["frontend", "HTTP POST", "1,318"] },
    { top: 57, height: 25, cells: ["payments", "authorize_card", "440"] },
  ],
}

describe("table hitTest", () => {
  it("names a column from its header", () => {
    expect(hitTest(layout, { x: 300, y: 10 })).toEqual({
      kind: "column",
      pos: { x: 0.75, y: 0.05 },
      column: "errors",
    })
  })

  it("names a row by its dimension cells", () => {
    expect(hitTest(layout, { x: 20, y: 60 })).toEqual({
      kind: "row",
      pos: { x: 0.05, y: 0.3 },
      row: { service: "payments", span: "authorize_card" },
    })
  })

  it("keeps the value of a measure cell under the pointer as shown", () => {
    expect(hitTest(layout, { x: 300, y: 60 })).toMatchObject({
      kind: "row",
      row: { service: "payments", span: "authorize_card" },
      shown: "440",
    })
  })

  it("uses every column when none is a dimension", () => {
    const flat = {
      ...layout,
      columns: layout.columns.map((column) => ({
        ...column,
        dimension: false,
      })),
    }
    expect(hitTest(flat, { x: 20, y: 40 })).toMatchObject({
      row: { service: "frontend", span: "HTTP POST", errors: "1,318" },
    })
  })

  it("returns null below the last row and beside the columns", () => {
    expect(hitTest(layout, { x: 20, y: 150 })).toBeNull()
    expect(hitTest(layout, { x: 380, y: 10 })).toBeNull()
  })

  it("reads a body row when the table has no header", () => {
    const bare = { ...layout, header: null }
    expect(hitTest(bare, { x: 20, y: 40 })?.kind).toBe("row")
  })
})
