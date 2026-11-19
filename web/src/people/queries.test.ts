import { describe, expect, it } from "vitest"

import { byNameThenId, type User } from "./queries"

function person(id: string, name: string, email: string, seen: string): User {
  return { id, name, email, last_seen_at: seen } as User
}

describe("byNameThenId", () => {
  it("orders by name and ignores how recently a person was seen", () => {
    const early = person("1", "Zoe", "z@example.com", "2026-11-01T00:00:00Z")
    const late = person("2", "adam", "a@example.com", "2026-11-19T00:00:00Z")
    expect([early, late].sort(byNameThenId).map((u) => u.id)).toEqual([
      "2",
      "1",
    ])
    const later = { ...late, last_seen_at: "2026-11-20T00:00:00Z" }
    expect([later, early].sort(byNameThenId).map((u) => u.id)).toEqual([
      "2",
      "1",
    ])
  })

  it("breaks a tie on email and then id", () => {
    const a = person("2", "Sam", "a@example.com", "")
    const b = person("1", "Sam", "b@example.com", "")
    const c = person("3", "Sam", "b@example.com", "")
    expect([c, b, a].sort(byNameThenId).map((u) => u.id)).toEqual([
      "2",
      "1",
      "3",
    ])
  })
})
