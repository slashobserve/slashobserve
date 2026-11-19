import { describe, expect, test } from "vitest"

import {
  defaultView,
  filtersApplied,
  pageCountOf,
  pageRange,
  searchOf,
  sortValue,
  validateObservationsSearch,
  viewOf,
} from "./list-view"

describe("validateObservationsSearch", () => {
  test("keeps only values it knows", () => {
    expect(
      validateObservationsSearch({
        state: "resolved",
        q: "  checkout  ",
        mine: "1",
        sort: "title",
        order: "desc",
        page: "3",
        size: "50",
        new: "1",
      })
    ).toEqual({
      state: "resolved",
      q: "checkout",
      mine: "1",
      sort: "title",
      order: "desc",
      page: 3,
      size: 50,
      new: "1",
    })
  })

  test("drops what is unknown, out of range or already the default", () => {
    expect(
      validateObservationsSearch({
        state: "open",
        q: "   ",
        mine: "yes",
        sort: "size",
        order: "desc",
        page: "0",
        size: "7",
      })
    ).toEqual({
      state: undefined,
      q: undefined,
      mine: undefined,
      sort: undefined,
      order: undefined,
      page: undefined,
      size: undefined,
      new: undefined,
    })
    expect(
      validateObservationsSearch({ sort: "title", order: "asc" }).order
    ).toBeUndefined()
    expect(validateObservationsSearch({ page: "1" }).page).toBeUndefined()
  })

  test("cuts a long title search to 200 characters", () => {
    expect(validateObservationsSearch({ q: "a".repeat(300) }).q).toHaveLength(
      200
    )
  })
})

describe("viewOf and searchOf", () => {
  test("the empty address is the default view", () => {
    expect(viewOf({})).toEqual(defaultView)
    expect(searchOf(defaultView, false)).toEqual({})
  })

  test("a title sort starts ascending and says so only when reversed", () => {
    const view = viewOf({ sort: "title" })
    expect(view.order).toBe("asc")
    expect(sortValue(view)).toBe("title:asc")
    expect(searchOf(view, false)).toEqual({ sort: "title" })
    expect(searchOf({ ...view, order: "desc" }, false)).toEqual({
      sort: "title",
      order: "desc",
    })
  })

  test("round trips a full view", () => {
    const view = {
      state: "all" as const,
      q: "billing",
      mine: true,
      sort: "created" as const,
      order: "asc" as const,
      page: 4,
      size: 100 as const,
    }
    expect(viewOf(searchOf(view, true))).toEqual(view)
    expect(searchOf(view, true).new).toBe("1")
  })
})

describe("paging", () => {
  test("counts pages and ranges", () => {
    expect(pageCountOf(0, 20)).toBe(1)
    expect(pageCountOf(41, 20)).toBe(3)
    expect(pageRange(3, 20, 41)).toEqual({ first: 41, last: 41 })
    expect(pageRange(1, 20, 0)).toEqual({ first: 0, last: 0 })
  })

  test("a search, the author filter or another state counts as a filter", () => {
    expect(filtersApplied(defaultView)).toBe(false)
    expect(filtersApplied({ ...defaultView, q: "x" })).toBe(true)
    expect(filtersApplied({ ...defaultView, mine: true })).toBe(true)
    expect(filtersApplied({ ...defaultView, state: "all" })).toBe(true)
  })
})
