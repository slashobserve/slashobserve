import { describe, expect, it } from "vitest"

import { pageCount, pageOf } from "./timeline-paging"

const rows = ["a", "b", "c", "d", "e", "f", "g"]

describe("pageCount", () => {
  it("rounds up to hold the last partial page", () => {
    expect(pageCount(7, 3)).toBe(3)
    expect(pageCount(6, 3)).toBe(2)
    expect(pageCount(1, 3)).toBe(1)
  })

  it("is one page with no rows or no page size", () => {
    expect(pageCount(0, 3)).toBe(1)
    expect(pageCount(7, undefined)).toBe(1)
  })
})

describe("pageOf", () => {
  it("slices the requested page", () => {
    expect(pageOf(rows, 3, 0)).toEqual({
      items: ["a", "b", "c"],
      start: 0,
      page: 0,
      count: 3,
    })
    expect(pageOf(rows, 3, 1)).toMatchObject({
      items: ["d", "e", "f"],
      start: 3,
    })
  })

  it("leaves a short last page", () => {
    expect(pageOf(rows, 3, 2)).toMatchObject({ items: ["g"], start: 6 })
  })

  it("clamps a page outside the range", () => {
    expect(pageOf(rows, 3, 9)).toMatchObject({ page: 2, items: ["g"] })
    expect(pageOf(rows, 3, -4)).toMatchObject({ page: 0, start: 0 })
  })

  it("keeps the page inside the range when rows disappear", () => {
    expect(pageOf(["a", "b"], 3, 2)).toMatchObject({
      page: 0,
      count: 1,
      items: ["a", "b"],
    })
  })

  it("shows every row without a page size", () => {
    expect(pageOf(rows, undefined, 5)).toEqual({
      items: rows,
      start: 0,
      page: 0,
      count: 1,
    })
  })

  it("has one empty page with no rows", () => {
    expect(pageOf([], 3, 0)).toEqual({ items: [], start: 0, page: 0, count: 1 })
  })
})
