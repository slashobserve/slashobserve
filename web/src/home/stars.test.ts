import { describe, expect, test } from "vitest"

import { withStar, type Star } from "./stars"

const now = new Date("2026-11-06T10:00:00Z")

function star(id: string, starredAt: string): Star {
  return {
    id,
    title: `Dashboard ${id}`,
    folder_id: "f1",
    folder_title: "General",
    starred_at: starredAt,
  }
}

const items = [
  star("a", "2026-11-06T09:00:00.000Z"),
  star("b", "2026-11-05T09:00:00.000Z"),
]
const target = {
  id: "c",
  title: "Checkout",
  folder_id: "f2",
  folder_title: "Shop",
}

describe("withStar", () => {
  test("puts a new star first with the current time", () => {
    expect(withStar(items, target, true, now)).toEqual([
      { ...target, starred_at: "2026-11-06T10:00:00.000Z" },
      ...items,
    ])
  })

  test("leaves a star that already exists where it is", () => {
    expect(withStar(items, { ...target, id: "b" }, true, now)).toEqual(items)
  })

  test("removes an unstarred dashboard and keeps the others in order", () => {
    expect(withStar(items, { ...target, id: "a" }, false, now)).toEqual([
      items[1],
    ])
    expect(withStar(items, target, false, now)).toEqual(items)
  })
})
