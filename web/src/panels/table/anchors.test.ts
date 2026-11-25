import { describe, expect, it } from "vitest"

import { edgeHeight } from "../list-anchors"

import { hitTest, locate, reveal, type TableAnchorLayout } from "./anchors"

const layout: TableAnchorLayout = {
  body: { width: 400, height: 200 },
  gridBottom: 160,
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

describe("table locate", () => {
  const pos = { x: 0, y: 0 }

  it("finds a row by its dimension labels", () => {
    expect(
      locate(layout, {
        kind: "row",
        pos,
        row: { service: "payments", span: "authorize_card" },
      })
    ).toEqual({ x: 0, y: 57, width: 400, height: 25 })
  })

  it("finds a cell and a column", () => {
    expect(
      locate(layout, {
        kind: "cell",
        pos,
        row: { service: "frontend" },
        column: "errors",
      })
    ).toEqual({ x: 250, y: 32, width: 100, height: 25 })
    expect(locate(layout, { kind: "column", pos, column: "span" })).toEqual({
      x: 100,
      y: 0,
      width: 150,
      height: 32,
    })
  })

  it("points at the grid bottom edge, above the pagination bar, for a row on another page", () => {
    const scrolled = {
      ...layout,
      elsewhere: [{ key: "r9", cells: ["search", "query", "12"] }],
    }
    expect(
      locate(scrolled, { kind: "row", pos, row: { service: "search" } })
    ).toEqual({ x: 0, y: 160 - edgeHeight, width: 400, height: edgeHeight })
    const edge = locate(scrolled, {
      kind: "row",
      pos,
      row: { service: "search" },
    })
    expect(edge ? edge.y + edge.height : 0).toBeLessThanOrEqual(
      scrolled.gridBottom
    )
  })

  it("returns null when the row or column is gone", () => {
    expect(
      locate(layout, { kind: "row", pos, row: { service: "search" } })
    ).toBeNull()
    expect(
      locate(layout, { kind: "column", pos, column: "latency" })
    ).toBeNull()
  })
})

describe("table reveal", () => {
  const pos = { x: 0, y: 0 }

  it("asks the table to show a row that is scrolled out of view", () => {
    const shown: string[] = []
    const scrolled: TableAnchorLayout = {
      ...layout,
      elsewhere: [{ key: "r9", cells: ["search", "query", "12"] }],
      reveal: (key) => shown.push(key),
    }
    expect(
      reveal(scrolled, { kind: "row", pos, row: { service: "search" } })
    ).toBe(true)
    expect(shown).toEqual(["r9"])
  })

  it("does nothing for a row in view or a row that is gone", () => {
    const shown: string[] = []
    const scrolled: TableAnchorLayout = {
      ...layout,
      elsewhere: [],
      reveal: (key) => shown.push(key),
    }
    expect(
      reveal(scrolled, { kind: "row", pos, row: { service: "payments" } })
    ).toBe(false)
    expect(
      reveal(scrolled, { kind: "row", pos, row: { service: "billing" } })
    ).toBe(false)
    expect(shown).toEqual([])
  })
})
