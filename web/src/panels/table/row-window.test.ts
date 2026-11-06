import { describe, expect, it } from "vitest"

import { rowWindow, windowAbove, windows } from "./row-window"

const base = { viewport: 250, rowHeight: 25, count: 300, overscan: 10 }

describe("rowWindow", () => {
  it("starts at the first row at the top of the list", () => {
    expect(rowWindow({ ...base, scrollTop: 0 })).toEqual({
      start: 0,
      end: 20,
      before: 0,
      after: 280 * 25,
    })
  })

  it("renders the rows in view and the overscan around them in the middle", () => {
    expect(rowWindow({ ...base, scrollTop: 2500 })).toEqual({
      start: 90,
      end: 120,
      before: 90 * 25,
      after: 180 * 25,
    })
  })

  it("ends at the last row with no space after it at the bottom", () => {
    const range = rowWindow({ ...base, scrollTop: 300 * 25 - 250 })
    expect(range.end).toBe(300)
    expect(range.after).toBe(0)
    expect(range.start).toBe(280)
  })

  it("renders every row when the viewport is taller than the list", () => {
    expect(
      rowWindow({ ...base, count: 5, viewport: 1000, scrollTop: 0 })
    ).toEqual({ start: 0, end: 5, before: 0, after: 0 })
  })

  it("clamps the overscan at both ends", () => {
    expect(rowWindow({ ...base, scrollTop: 50, overscan: 50 }).start).toBe(0)
    expect(rowWindow({ ...base, scrollTop: 7000, overscan: 50 }).end).toBe(300)
  })

  it("clamps a scroll position past a list that shrank", () => {
    const range = rowWindow({ ...base, count: 120, scrollTop: 7000 })
    expect(range.start).toBeLessThanOrEqual(range.end)
    expect(range.end).toBe(120)
    expect(range.after).toBe(0)
    expect(range.before).toBe(range.start * 25)
  })

  it("renders every row with no spacers when the row height is 0", () => {
    expect(rowWindow({ ...base, rowHeight: 0, scrollTop: 400 })).toEqual({
      start: 0,
      end: 300,
      before: 0,
      after: 0,
    })
  })
})

describe("windows", () => {
  const fits = {
    paginate: false,
    count: 300,
    expandedCount: 0,
    grows: false,
    columnsFixed: true,
  }

  it("windows a long table of fixed-height rows", () => {
    expect(windows(fits)).toBe(true)
    expect(windows({ ...fits, count: windowAbove + 1 })).toBe(true)
  })

  it.each([
    ["a paginated table", { paginate: true }],
    ["a table of 100 rows", { count: 100 }],
    ["a table with an expanded row", { expandedCount: 1 }],
    ["a table whose rows grow", { grows: true }],
    ["a table with a column of variable height", { columnsFixed: false }],
  ])("renders %s in full", (_name, change) => {
    expect(windows({ ...fits, ...change })).toBe(false)
  })
})
