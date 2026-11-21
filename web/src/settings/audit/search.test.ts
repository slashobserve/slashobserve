import { describe, expect, test } from "vitest"

import {
  auditSearchOf,
  auditViewOf,
  auditWindow,
  followsNow,
  readAuditSearch,
  sameSearchLater,
} from "./search"

const id = "0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

describe("readAuditSearch", () => {
  test("keeps a query, a time range and an event id", () => {
    expect(
      readAuditSearch({
        q: "outcome:denied",
        from: "now-7d",
        to: "now",
        event: id,
      })
    ).toEqual({ q: "outcome:denied", from: "now-7d", to: "now", event: id })
  })

  test("drops values that are not times or event ids", () => {
    expect(
      readAuditSearch({ q: "", from: "yesterday", to: 5, event: "nope" })
    ).toEqual({
      q: undefined,
      from: undefined,
      to: undefined,
      event: undefined,
    })
  })
})

describe("auditViewOf and auditSearchOf", () => {
  test("default to the last 24 hours", () => {
    expect(auditViewOf({})).toEqual({
      q: "",
      from: "now-24h",
      to: "now",
      event: null,
    })
  })

  test("leave the defaults out of the address", () => {
    expect(auditSearchOf(auditViewOf({}))).toEqual({
      q: undefined,
      from: undefined,
      to: undefined,
      event: undefined,
    })
  })

  test("round-trip a view", () => {
    const view = { q: "origin:mcp", from: "now-7d", to: "now-1d", event: id }
    expect(auditViewOf(auditSearchOf(view))).toEqual(view)
  })
})

describe("followsNow", () => {
  test("is true only for a range that ends now", () => {
    expect(followsNow({ to: "now" })).toBe(true)
    expect(followsNow({ to: "now-1h" })).toBe(false)
  })
})

describe("auditWindow", () => {
  const now = Date.parse("2026-11-21T12:00:00Z")

  test("resolves a relative range against now", () => {
    expect(auditWindow("now-24h", "now", now, "UTC")).toEqual({
      since: "2026-11-20T12:00:00.000Z",
      until: "2026-11-21T12:00:00.000Z",
    })
  })

  test("ends a rounded day at the start of the next one", () => {
    expect(auditWindow("now-1d/d", "now-1d/d", now, "UTC")).toEqual({
      since: "2026-11-20T00:00:00.000Z",
      until: "2026-11-21T00:00:00.000Z",
    })
  })

  test("refuses more than 366 days", () => {
    expect(auditWindow("now-366d", "now", now, "UTC").problem).toBeUndefined()
    expect(auditWindow("now-367d", "now", now, "UTC").problem).toMatch(
      /at most 366 days/
    )
  })

  test("refuses a range that ends before it starts", () => {
    expect(auditWindow("now", "now-1h", now, "UTC").problem).toMatch(
      /ends before it starts/
    )
  })
})

describe("sameSearchLater", () => {
  const day = {
    q: "origin:mcp",
    since: "2026-11-20T06:00:00.000Z",
    until: "2026-11-21T06:00:00.000Z",
  }

  test("holds for the same query and span moved forward", () => {
    expect(
      sameSearchLater(day, {
        ...day,
        since: "2026-11-20T07:00:00.000Z",
        until: "2026-11-21T07:00:00.000Z",
      })
    ).toBe(true)
  })

  test("does not hold for another query, span or an earlier window", () => {
    expect(sameSearchLater(day, { ...day, q: "" })).toBe(false)
    expect(
      sameSearchLater(day, { ...day, since: "2026-11-14T06:00:00.000Z" })
    ).toBe(false)
    expect(
      sameSearchLater(day, {
        ...day,
        since: "2026-11-20T05:00:00.000Z",
        until: "2026-11-21T05:00:00.000Z",
      })
    ).toBe(false)
  })
})
