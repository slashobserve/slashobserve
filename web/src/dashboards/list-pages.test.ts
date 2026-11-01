import { describe, expect, it } from "vitest"

import {
  listOffset,
  pageCountOf,
  pageItems,
  pageOutOfRange,
  pageRange,
} from "./list-pages"

describe("pageCountOf", () => {
  it("is at least one page and rounds up", () => {
    expect(pageCountOf(0)).toBe(1)
    expect(pageCountOf(50)).toBe(1)
    expect(pageCountOf(51)).toBe(2)
    expect(pageCountOf(312)).toBe(7)
  })
})

describe("pageRange", () => {
  it("gives the first and last dashboard number on the page", () => {
    expect(pageRange(2, 312)).toEqual({ first: 51, last: 100 })
    expect(pageRange(7, 312)).toEqual({ first: 301, last: 312 })
    expect(pageRange(1, 0)).toEqual({ first: 0, last: 0 })
  })
})

describe("listOffset", () => {
  it("skips the pages before", () => {
    expect(listOffset(1)).toBe(0)
    expect(listOffset(3)).toBe(100)
  })
})

describe("pageItems", () => {
  it("lists every page up to seven", () => {
    expect(pageItems(1, 1)).toEqual([1])
    expect(pageItems(4, 7)).toEqual([1, 2, 3, 4, 5, 6, 7])
  })

  it("keeps seven slots with an ellipsis above seven pages", () => {
    expect(pageItems(1, 8)).toEqual([1, 2, 3, 4, 5, "end-ellipsis", 8])
    expect(pageItems(4, 8)).toEqual([1, 2, 3, 4, 5, "end-ellipsis", 8])
    expect(pageItems(5, 8)).toEqual([1, "start-ellipsis", 4, 5, 6, 7, 8])
    expect(pageItems(8, 8)).toEqual([1, "start-ellipsis", 4, 5, 6, 7, 8])
  })

  it("puts the current page in the middle of twenty pages", () => {
    expect(pageItems(5, 20)).toEqual([
      1,
      "start-ellipsis",
      4,
      5,
      6,
      "end-ellipsis",
      20,
    ])
    expect(pageItems(10, 20)).toEqual([
      1,
      "start-ellipsis",
      9,
      10,
      11,
      "end-ellipsis",
      20,
    ])
    expect(pageItems(16, 20)).toEqual([
      1,
      "start-ellipsis",
      15,
      16,
      17,
      "end-ellipsis",
      20,
    ])
    expect(pageItems(17, 20)).toEqual([1, "start-ellipsis", 16, 17, 18, 19, 20])
  })

  it("always has seven slots, the current, first and last page", () => {
    for (const page of [1, 4, 5, 10, 16, 17, 20]) {
      const items = pageItems(page, 20)
      expect(items).toHaveLength(7)
      expect(items).toContain(page)
      expect(items[0]).toBe(1)
      expect(items[6]).toBe(20)
    }
  })
})

describe("pageOutOfRange", () => {
  it("names the last page for a page past the end", () => {
    expect(pageOutOfRange(3, 100)).toBe(2)
    expect(pageOutOfRange(2, 100)).toBeNull()
    expect(pageOutOfRange(5, 0)).toBe(1)
  })
})
