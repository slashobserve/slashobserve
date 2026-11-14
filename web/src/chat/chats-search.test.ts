import { describe, expect, test } from "vitest"

import { parseChatsSearch } from "./chats-search"

describe("parseChatsSearch", () => {
  test.each([
    ["a query", { q: "checkout" }, { q: "checkout" }],
    ["a query with spaces around it", { q: "  checkout  " }, { q: "checkout" }],
    ["a 200 character query", { q: "a".repeat(200) }, { q: "a".repeat(200) }],
    ["running", { state: "running" }, { state: "running" }],
    ["stopped", { state: "stopped" }, { state: "stopped" }],
    ["done", { state: "done" }, { state: "done" }],
    ["everyone", { scope: "all" }, { scope: "all" }],
    [
      "all three together",
      { q: "p95", state: "done", scope: "all" },
      { q: "p95", state: "done", scope: "all" },
    ],
  ] as const)("keeps %s", (_name, search, expected) => {
    expect(parseChatsSearch(search)).toMatchObject(expected)
  })

  test.each([
    ["a blank query", { q: "   " }],
    ["a 201 character query", { q: "a".repeat(201) }],
    ["a numeric query", { q: 5 }],
    ["an unknown state", { state: "queued" }],
    ["scope mine", { scope: "mine" }],
    ["a numeric scope", { scope: 1 }],
    ["nothing", {}],
  ] as const)("drops %s", (_name, search) => {
    const parsed = parseChatsSearch(search)
    expect(parsed.q).toBeUndefined()
    expect(parsed.state).toBeUndefined()
    expect(parsed.scope).toBeUndefined()
  })
})
