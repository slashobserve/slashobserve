import { describe, expect, test } from "vitest"

import { withPin, type Pin } from "./pins"

const now = new Date("2026-11-06T10:00:00Z")

function pin(id: string, starredAt: string): Pin {
  return {
    id,
    title: `Dashboard ${id}`,
    folder_id: "f1",
    folder_title: "General",
    starred_at: starredAt,
  }
}

const items = [
  pin("a", "2026-11-06T09:00:00.000Z"),
  pin("b", "2026-11-05T09:00:00.000Z"),
]
const target = {
  id: "c",
  title: "Checkout",
  folder_id: "f2",
  folder_title: "Shop",
}

describe("withPin", () => {
  test("puts a new pin first with the current time", () => {
    expect(withPin(items, target, true, now)).toEqual([
      { ...target, starred_at: "2026-11-06T10:00:00.000Z" },
      ...items,
    ])
  })

  test("leaves a pin that already exists where it is", () => {
    expect(withPin(items, { ...target, id: "b" }, true, now)).toEqual(items)
  })

  test("removes an unpinned dashboard and keeps the others in order", () => {
    expect(withPin(items, { ...target, id: "a" }, false, now)).toEqual([
      items[1],
    ])
    expect(withPin(items, target, false, now)).toEqual(items)
  })
})
