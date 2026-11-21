import { describe, expect, test } from "vitest"

import { parseChatsSearch } from "./chats-search"

const dashboard = "5b0e3c55-6a8e-4e1f-9d0a-0d6f1f6f7f10"

describe("parseChatsSearch", () => {
  test.each([
    ["a query", { q: "checkout" }, { q: "checkout" }],
    ["a query with spaces around it", { q: "  checkout  " }, { q: "checkout" }],
    ["a 200 character query", { q: "a".repeat(200) }, { q: "a".repeat(200) }],
    ["a dashboard", { dashboard }, { dashboard }],
    ["everyone", { scope: "all" }, { scope: "all" }],
    [
      "all three together",
      { q: "p95", dashboard, scope: "all" },
      { q: "p95", dashboard, scope: "all" },
    ],
  ] as const)("keeps %s", (_name, search, expected) => {
    expect(parseChatsSearch(search)).toMatchObject(expected)
  })

  test.each([
    ["a blank query", { q: "   " }],
    ["a 201 character query", { q: "a".repeat(201) }],
    ["a numeric query", { q: 5 }],
    ["a dashboard that is not an id", { dashboard: "checkout" }],
    ["scope mine", { scope: "mine" }],
    ["a numeric scope", { scope: 1 }],
    ["nothing", {}],
  ] as const)("drops %s", (_name, search) => {
    const parsed = parseChatsSearch(search)
    expect(parsed.q).toBeUndefined()
    expect(parsed.dashboard).toBeUndefined()
    expect(parsed.scope).toBeUndefined()
  })
})
