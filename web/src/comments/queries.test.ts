import { describe, expect, it } from "vitest"

import { isPanelThread, type Comment } from "./queries"

const author = { kind: "user", id: "u1", name: "Ada" } as const
const actor = {
  ...author,
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function comment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: "c1",
    dashboard_id: "d1",
    parent_id: null,
    version: 1,
    panel_id: "rps",
    body: "note",
    status: "open",
    allowed_actions: ["reply", "resolve"],
    author,
    actor,
    created_at: "2026-10-19T00:00:00Z",
    updated_at: "2026-10-19T00:00:00Z",
    resolved_at: null,
    resolved_by: null,
    resolved_actor: null,
    edited_at: null,
    mentions: [],
    can_edit: false,
    resolved_by_version: null,
    panel_present: true,
    revision: null,
    ...overrides,
  }
}

describe("isPanelThread", () => {
  it("accepts the first comment of a thread on a panel", () => {
    expect(isPanelThread(comment())).toBe(true)
  })

  it("rejects replies and whole-dashboard comments", () => {
    expect(isPanelThread(comment({ parent_id: "c0" }))).toBe(false)
    expect(isPanelThread(comment({ panel_id: null }))).toBe(false)
  })
})
