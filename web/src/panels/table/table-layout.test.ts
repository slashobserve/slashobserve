import { describe, expect, it } from "vitest"

import {
  alignOf,
  columnWidths,
  frozenCount,
  pageRows,
  pageSizeFor,
  rowHeightOf,
  stickyOffsets,
  typeWord,
} from "./table-layout"

describe("alignOf", () => {
  it.each([
    ["auto", "number", "right"],
    ["auto", "string", "left"],
    ["auto", "time", "left"],
    ["left", "number", "left"],
    ["center", "string", "center"],
    ["right", "string", "right"],
  ] as const)("%s on a %s column is %s", (align, type, want) => {
    expect(alignOf(align, type)).toBe(want)
  })
})

describe("columnWidths", () => {
  it("keeps the browser's sizing when nothing is sized", () => {
    expect(columnWidths([{}, {}], 600, false)).toBeNull()
    expect(columnWidths([], 600, true)).toBeNull()
  })

  it("gives sized columns their width and shares the rest equally", () => {
    expect(columnWidths([{ width: 100 }, {}, {}], 500, false)).toEqual({
      widths: [100, 200, 200],
      total: 500,
    })
  })

  it("never shrinks a shared column below its minimum width", () => {
    expect(
      columnWidths([{ width: 300 }, { minWidth: 150 }, {}], 500, false)
    ).toEqual({ widths: [300, 150, 100], total: 550 })
  })

  it("keeps a free column above the floor so the table scrolls sideways", () => {
    const layout = columnWidths([{ width: 400 }, {}], 420, false)
    expect(layout?.widths[1]).toBeGreaterThanOrEqual(64)
    expect(layout?.total).toBeGreaterThan(420)
  })

  it("lets a minimum widen a fixed width", () => {
    expect(columnWidths([{ width: 80, minWidth: 120 }], 500, false)).toEqual({
      widths: [120],
      total: 120,
    })
  })

  it("fixes the layout when columns are frozen even if none is sized", () => {
    expect(columnWidths([{}, {}], 400, true)).toEqual({
      widths: [200, 200],
      total: 400,
    })
  })
})

describe("stickyOffsets", () => {
  it("accumulates the widths before each frozen column", () => {
    expect(stickyOffsets([100, 50, 80], 2)).toEqual([0, 100])
  })

  it("stops at the number of columns", () => {
    expect(stickyOffsets([100], 3)).toEqual([0])
    expect(stickyOffsets([100, 50], 0)).toEqual([])
  })
})

describe("frozenCount", () => {
  it.each([
    [{ frozenColumns: 2 }, 2],
    [{ frozenColumns: 0 }, 0],
    [{ frozenColumns: -1 }, 0],
    [{ frozenColumns: "2" }, 0],
    [{}, 0],
  ])("reads %j as %i", (display, want) => {
    expect(frozenCount(display)).toBe(want)
  })
})

describe("rowHeightOf", () => {
  it.each([
    [{ cellHeight: "md" }, "md"],
    [{ cellHeight: "lg" }, "lg"],
    [{ cellHeight: "xl" }, "sm"],
    [{}, "sm"],
  ])("reads %j as %s", (display, want) => {
    expect(rowHeightOf(display)).toBe(want)
  })
})

describe("pageSizeFor", () => {
  it("fits whole rows between the header, footer and page controls", () => {
    expect(pageSizeFor(400, "sm", { header: true, footerRows: 0 })).toBe(13)
    expect(pageSizeFor(400, "lg", { header: false, footerRows: 2 })).toBe(7)
  })

  it("always shows at least one row", () => {
    expect(pageSizeFor(10, "lg", { header: true, footerRows: 3 })).toBe(1)
  })
})

describe("pageRows", () => {
  const rows = [1, 2, 3, 4, 5, 6, 7]

  it("slices the requested page and counts the pages", () => {
    expect(pageRows(rows, 1, 3)).toEqual({
      rows: [4, 5, 6],
      page: 1,
      pageCount: 3,
    })
  })

  it("clamps a page past the end or before the start", () => {
    expect(pageRows(rows, 9, 3).rows).toEqual([7])
    expect(pageRows(rows, -2, 3).page).toBe(0)
  })

  it("has one empty page for no rows", () => {
    expect(pageRows([], 0, 5)).toEqual({ rows: [], page: 0, pageCount: 1 })
  })
})

describe("typeWord", () => {
  it("names each column type for screen readers", () => {
    expect(typeWord("string")).toBe("text")
    expect(typeWord("bool")).toBe("boolean")
    expect(typeWord("number")).toBe("number")
  })
})
