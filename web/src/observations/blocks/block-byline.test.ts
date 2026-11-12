import { describe, expect, it } from "vitest"

import type { Actor } from "@/lib/author"

import { bylineText } from "./block-byline"

function actor(overrides: Partial<Actor>): Actor {
  return {
    kind: "user",
    id: "u1",
    name: "Priya",
    origin: "web",
    client: null,
    run_id: null,
    label: "Priya",
    ...overrides,
  }
}

describe("bylineText", () => {
  it("credits a person who added the block", () => {
    expect(bylineText({ created_by: actor({}) })).toBe("Added by Priya")
  })

  it("credits an outside agent by its label", () => {
    const mcp = actor({
      origin: "mcp",
      client: "Claude Code",
      label: "Priya (via Claude Code)",
    })
    expect(bylineText({ created_by: mcp })).toBe(
      "Added by Priya (via Claude Code)"
    )
  })

  it("says the block came from Observe Agent when a run wrote it", () => {
    const agent = actor({ run_id: "r1", label: "Observe Agent for Priya" })
    expect(bylineText({ created_by: agent })).toBe("From Observe Agent")
  })

  it("names who kept an agent's block when that is known", () => {
    const agent = actor({ run_id: "r1", label: "Observe Agent for Priya" })
    const keeper = actor({ id: "u2", name: "Sam", label: "Sam" })
    expect(bylineText({ created_by: agent }, keeper)).toBe(
      "From Observe Agent, added by Sam"
    )
  })
})
