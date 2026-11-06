import { describe, expect, test } from "vitest"

import {
  parseRecent,
  recentLimit,
  recentToShow,
  recordRecent,
  type RecentDashboard,
} from "./recent"

const now = new Date("2026-11-06T10:00:00Z")

function entry(id: string, viewedAt = "2026-11-05T09:00:00.000Z") {
  return { id, title: `Dashboard ${id}`, viewed_at: viewedAt }
}

describe("recordRecent", () => {
  test("puts the viewed dashboard first with the current time", () => {
    const list = [entry("a"), entry("b")]
    expect(recordRecent(list, { id: "c", title: "Checkout" }, now)).toEqual([
      { id: "c", title: "Checkout", viewed_at: "2026-11-06T10:00:00.000Z" },
      entry("a"),
      entry("b"),
    ])
  })

  test("moves a dashboard viewed again to the front with its new title", () => {
    const list = [entry("a"), entry("b"), entry("c")]
    expect(
      recordRecent(list, { id: "b", title: "Renamed" }, now).map((r) => [
        r.id,
        r.title,
      ])
    ).toEqual([
      ["b", "Renamed"],
      ["a", "Dashboard a"],
      ["c", "Dashboard c"],
    ])
  })

  test("keeps at most twenty and drops the oldest", () => {
    const list = Array.from({ length: recentLimit }, (_, i) => entry(`d${i}`))
    const next = recordRecent(list, { id: "new", title: "New" }, now)
    expect(next).toHaveLength(20)
    expect(next[0].id).toBe("new")
    expect(next.at(-1)?.id).toBe("d18")
  })

  test("does not change the list it was given", () => {
    const list = [entry("a")]
    recordRecent(list, { id: "b", title: "B" }, now)
    expect(list).toEqual([entry("a")])
  })
})

describe("parseRecent", () => {
  test.each([
    ["nothing stored", null],
    ["not JSON", "{oops"],
    ["not a list", JSON.stringify({ id: "a" })],
    ["an empty list", "[]"],
  ])("%s gives no recents", (_name, raw) => {
    expect(parseRecent(raw)).toEqual([])
  })

  test("drops malformed entries and keeps the valid ones in order", () => {
    const raw = JSON.stringify([
      entry("a"),
      null,
      "b",
      { id: "", title: "Empty id", viewed_at: now.toISOString() },
      { id: "c", title: 7, viewed_at: now.toISOString() },
      { id: "d", title: "No time" },
      { id: "e", title: "Bad time", viewed_at: "yesterday" },
      { ...entry("f"), extra: true },
      entry("a", "2026-11-01T00:00:00.000Z"),
    ])
    expect(parseRecent(raw)).toEqual([entry("a"), entry("f")])
  })

  test("keeps at most twenty", () => {
    const raw = JSON.stringify(
      Array.from({ length: 25 }, (_, i) => entry(`d${i}`))
    )
    expect(parseRecent(raw)).toHaveLength(20)
  })
})

describe("recentToShow", () => {
  test("leaves out starred dashboards and keeps the newest first", () => {
    const list: RecentDashboard[] = [entry("a"), entry("b"), entry("c")]
    expect(recentToShow(list, new Set(["b"]), 5).map((r) => r.id)).toEqual([
      "a",
      "c",
    ])
    expect(recentToShow(list, new Set(), 2).map((r) => r.id)).toEqual([
      "a",
      "b",
    ])
  })
})
