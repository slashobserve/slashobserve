import { describe, expect, test } from "vitest"

import { connectClients, isNewConnection } from "./connect-clients"

describe("connectClients", () => {
  test("offers Claude Code with its OAuth command for the server", () => {
    expect(connectClients.map((client) => client.label)).toEqual([
      "Claude Code",
    ])
    expect(connectClients[0]?.snippet("https://observe.example/mcp")).toBe(
      "claude mcp add --transport http slashobserve https://observe.example/mcp"
    )
  })
})

describe("isNewConnection", () => {
  test.each([
    { name: "nothing changed", before: ["a"], after: ["a"], found: null },
    { name: "a grant was added", before: ["a"], after: ["b", "a"], found: "b" },
    {
      name: "the first added grant wins",
      before: [],
      after: ["x", "y"],
      found: "x",
    },
    {
      name: "a grant was removed",
      before: ["a", "b"],
      after: ["a"],
      found: null,
    },
    { name: "both lists empty", before: [], after: [], found: null },
  ])("$name", ({ before, after, found }) => {
    expect(isNewConnection(before, after)).toBe(found)
  })
})
