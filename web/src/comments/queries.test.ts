import { describe, expect, it } from "vitest"

import { isPanelThread, withStatus, type Comment } from "./queries"

const author = { kind: "user", id: "u1", name: "Ada" } as const

function comment(overrides: Partial<Comment> = {}): Comment {
  return {
    id: "c1",
    dashboard_id: "d1",
    parent_id: null,
    version: 1,
    panel_id: "rps",
    body: "note",
    status: "open",
    author,
    created_at: "2026-10-19T00:00:00Z",
    updated_at: "2026-10-19T00:00:00Z",
    resolved_at: null,
    resolved_by: null,
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

describe("withStatus", () => {
  const now = "2026-10-19T01:00:00Z"

  it("resolves a thread's first comment", () => {
    const next = withStatus(comment(), "resolved", now)
    expect(next.status).toBe("resolved")
    expect(next.resolved_at).toBe(now)
  })

  it("reopens a thread's first comment and clears the resolved fields", () => {
    const resolved = comment({
      status: "resolved",
      resolved_at: now,
      resolved_by: author,
      resolved_by_version: 2,
    })
    expect(withStatus(resolved, "open", now)).toMatchObject({
      status: "open",
      resolved_at: null,
      resolved_by: null,
      resolved_by_version: null,
    })
  })

  it("changes only the status of a reply", () => {
    const reply = comment({ parent_id: "c0" })
    expect(withStatus(reply, "resolved", now)).toEqual({
      ...reply,
      status: "resolved",
    })
  })
})
