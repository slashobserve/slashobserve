import { describe, expect, it } from "vitest"

import {
  pageCountOf,
  pageItems,
  pagesByOffset,
  pageStateOf,
  rangeText,
  validPage,
  validPageSize,
  windowNote,
  type PageMeta,
  type PageState,
} from "./paging"

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

const meta: PageMeta = {
  page: 1,
  page_size: 50,
  total: 1284,
  total_relation: "eq",
  has_next: true,
  has_prev: false,
}

const offsetState: PageState = {
  page: 1,
  pageSize: 50,
  rows: 50,
  total: 1284,
  totalRelation: "eq",
  hasNext: true,
  hasPrev: false,
  byOffset: true,
}

describe("rangeText", () => {
  it.each([
    ["an exact total", offsetState, "1-50 of 1,284 traces in range"],
    [
      "a lower bound",
      { ...offsetState, page: 2, totalRelation: "gte" as const },
      "51-100 of at least 1,284 traces in range",
    ],
    [
      "an unknown total",
      { ...offsetState, page: 2, total: undefined, totalRelation: "unknown" },
      "51-100 traces in range",
    ],
    [
      "an empty first page",
      { ...offsetState, rows: 0, total: 0 },
      "No traces in range",
    ],
  ] as const)("reads %s", (_, state, text) => {
    expect(rangeText(state, "traces")).toBe(text)
  })

  it("ends a short last page at its last row and takes another qualifier", () => {
    expect(
      rangeText(
        { ...offsetState, page: 26, rows: 34 },
        "traces",
        " matching the comparison"
      )
    ).toBe("1,251-1,284 of 1,284 traces matching the comparison")
  })
})

describe("pageStateOf", () => {
  it("reads the page the server answered", () => {
    expect(
      pageStateOf(
        {
          ...meta,
          page: 3,
          has_prev: true,
          next: "n",
          prev: "p",
          window: 10000,
        },
        50,
        true,
        { page: 1, pageSize: 100 }
      )
    ).toEqual({
      page: 3,
      pageSize: 50,
      rows: 50,
      total: 1284,
      totalRelation: "eq",
      hasNext: true,
      hasPrev: true,
      byOffset: true,
      window: 10000,
      next: "n",
      prev: "p",
    })
  })

  it("falls back to the requested page when the server sent no meta.page", () => {
    expect(
      pageStateOf(undefined, 100, false, {
        page: 1,
        pageSize: 100,
        truncated: true,
      })
    ).toEqual({
      page: 1,
      pageSize: 100,
      rows: 100,
      totalRelation: "unknown",
      hasNext: true,
      hasPrev: false,
      byOffset: false,
    })
  })

  it("drops a total the server could not count", () => {
    const state = pageStateOf(
      { ...meta, total: 7, total_relation: "unknown" },
      7,
      false,
      { page: 1, pageSize: 50 }
    )
    expect(state.total).toBeUndefined()
  })
})

describe("pageCountOf", () => {
  it("counts the pages of an exact total", () => {
    expect(pageCountOf(offsetState)).toBe(26)
  })

  it("stops at the window", () => {
    expect(
      pageCountOf({
        ...offsetState,
        pageSize: 100,
        total: 48210,
        window: 10000,
      })
    ).toBe(100)
  })

  it("is null on a cursor store or without a total", () => {
    expect(pageCountOf({ ...offsetState, byOffset: false })).toBeNull()
    expect(
      pageCountOf({
        ...offsetState,
        total: undefined,
        totalRelation: "unknown",
      })
    ).toBeNull()
  })

  it("is one page for no rows", () => {
    expect(pageCountOf({ ...offsetState, total: 0 })).toBe(1)
  })
})

describe("windowNote", () => {
  const capped: PageState = {
    ...offsetState,
    pageSize: 100,
    total: 48210,
    window: 10000,
  }

  it("names the last page and the window", () => {
    expect(windowNote(capped, "Elasticsearch")).toBe(
      "Pages stop at 100, the first 10,000 documents Elasticsearch will page through. Narrow the time range to reach older ones."
    )
  })

  it("is null without a window or when the total fits inside it", () => {
    expect(windowNote({ ...capped, window: undefined }, "Elasticsearch")).toBe(
      null
    )
    expect(windowNote({ ...capped, total: 9000 }, "Elasticsearch")).toBeNull()
  })
})

describe("validPage", () => {
  it.each([
    [1, 1],
    ["3", 3],
    [["4", "5"], 4],
    [100000, 100000],
    [0, undefined],
    ["0", undefined],
    [1.5, undefined],
    ["1.5", undefined],
    ["abc", undefined],
    [100001, undefined],
    ["-2", undefined],
    [undefined, undefined],
  ])("reads %j as %j", (raw, page) => {
    expect(validPage(raw)).toBe(page)
  })
})

describe("validPageSize", () => {
  it.each([
    ["50", 50],
    [100, 100],
    ["250", 250],
    ["500", 500],
    ["25", undefined],
    ["1000", undefined],
    ["many", undefined],
  ])("reads %j as %j", (raw, size) => {
    expect(validPageSize(raw)).toBe(size)
  })
})

describe("pagesByOffset", () => {
  it.each([
    ["clickhouse", true],
    ["elasticsearch", true],
    ["opensearch", true],
    ["loki", false],
    ["tempo", false],
  ])("%s is %s", (type, byOffset) => {
    expect(pagesByOffset(type)).toBe(byOffset)
  })
})
