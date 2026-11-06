import { describe, expect, it } from "vitest"

import {
  listOffset,
  pageCountOf,
  pageOutOfRange,
  pageRange,
  withOffset,
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

describe("pageOutOfRange", () => {
  it("names the last page for a page past the end", () => {
    expect(pageOutOfRange(3, 100)).toBe(2)
    expect(pageOutOfRange(2, 100)).toBeNull()
    expect(pageOutOfRange(5, 0)).toBe(1)
  })
})

describe("withOffset", () => {
  it("keeps the list and records the offset it was fetched at", () => {
    const list = { dashboards: ["a"], total_count: 120 }
    expect(withOffset(list, 0)).toEqual({ ...list, offset: 0 })
    expect(withOffset(list, listOffset(3))).toEqual({ ...list, offset: 100 })
  })
})
