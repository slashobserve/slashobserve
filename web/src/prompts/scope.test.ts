import { describe, expect, it } from "vitest"

import type { SavedPrompt } from "./queries"
import { scopeOf } from "./scope"

function prompt(ownerId: string, shared: boolean): SavedPrompt {
  return {
    id: "p1",
    name: "Checkout health",
    body: "checkout health",
    shared,
    owner: { id: ownerId, name: "Ada" },
    can_edit: ownerId === "me",
    created_at: "2026-10-30T00:00:00Z",
    updated_at: "2026-10-30T00:00:00Z",
  }
}

describe("scopeOf", () => {
  it("names who can use a prompt from the viewer's side", () => {
    expect(scopeOf(prompt("me", false), "me")).toEqual({
      shared: false,
      text: "Only me",
    })
    expect(scopeOf(prompt("me", true), "me")).toEqual({
      shared: true,
      text: "Everyone",
    })
    expect(scopeOf(prompt("ada", true), "me")).toEqual({
      shared: true,
      text: "By Ada",
    })
  })
})
