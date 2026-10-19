import { describe, expect, it } from "vitest"

import type { Comment } from "./queries"
import { groupThreads, openThreadCount, openThreadsByPanel } from "./threads"

const author = { kind: "user", id: "u1", name: "Ada" } as const

function comment(overrides: Partial<Comment> & { id: string }): Comment {
  return {
    dashboard_id: "d1",
    parent_id: null,
    version: 1,
    panel_id: null,
    body: "text",
    status: "open",
    author,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    resolved_at: null,
    resolved_by: null,
    resolved_by_version: null,
    panel_present: true,
    revision: null,
    ...overrides,
  }
}

describe("groupThreads", () => {
  it("orders roots and replies by creation time then id", () => {
    const threads = groupThreads([
      comment({ id: "r2", created_at: "2026-01-02T00:00:00Z" }),
      comment({
        id: "b",
        parent_id: "r1",
        created_at: "2026-01-03T00:00:00Z",
      }),
      comment({ id: "r1", created_at: "2026-01-01T00:00:00Z" }),
      comment({
        id: "a",
        parent_id: "r1",
        created_at: "2026-01-03T00:00:00Z",
      }),
      comment({
        id: "c",
        parent_id: "r1",
        created_at: "2026-01-02T00:00:00Z",
      }),
    ])
    expect(threads.map((thread) => thread.root.id)).toEqual(["r1", "r2"])
    expect(threads[0].replies.map((reply) => reply.id)).toEqual(["c", "a", "b"])
    expect(threads[1].replies).toEqual([])
  })

  it("orders roots with equal timestamps by id", () => {
    const threads = groupThreads([comment({ id: "b" }), comment({ id: "a" })])
    expect(threads.map((thread) => thread.root.id)).toEqual(["a", "b"])
  })

  it("drops a reply whose root is not loaded", () => {
    const threads = groupThreads([
      comment({ id: "r1" }),
      comment({ id: "orphan", parent_id: "missing" }),
    ])
    expect(threads).toHaveLength(1)
    expect(threads[0].replies).toEqual([])
  })
})

describe("openThreadCount", () => {
  it("counts open roots only", () => {
    expect(
      openThreadCount([
        comment({ id: "r1" }),
        comment({ id: "r2", status: "resolved" }),
        comment({ id: "r3", panel_id: "p1" }),
        comment({ id: "reply", parent_id: "r1" }),
      ])
    ).toBe(2)
  })
})

describe("openThreadsByPanel", () => {
  it("counts open panel roots per panel", () => {
    const counts = openThreadsByPanel([
      comment({ id: "r1", panel_id: "p1" }),
      comment({ id: "r2", panel_id: "p1" }),
      comment({ id: "r3", panel_id: "p2", status: "resolved" }),
      comment({ id: "r4", panel_id: null }),
      comment({ id: "reply", parent_id: "r1", panel_id: "p1" }),
    ])
    expect(counts).toEqual(new Map([["p1", 2]]))
  })
})
