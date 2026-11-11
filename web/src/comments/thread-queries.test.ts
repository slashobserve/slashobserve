import { describe, expect, it } from "vitest"

import { groupThreadPages, type Message, type Thread } from "./thread-queries"

const ada = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function thread(id: string, createdAt: string): Thread {
  return {
    id,
    target: { kind: "finding", id: "f1" },
    dashboard_id: null,
    version: null,
    panel_id: null,
    panel_present: true,
    status: "open",
    allowed_actions: ["reply", "resolve"],
    created_at: createdAt,
    updated_at: createdAt,
    resolved_at: null,
    resolved_actor: null,
    resolved_by_version: null,
    revision: null,
  }
}

function message(id: string, threadId: string, createdAt: string): Message {
  return {
    id,
    thread_id: threadId,
    body: id,
    actor: ada,
    created_at: createdAt,
    updated_at: createdAt,
    edited_at: null,
    mentions: [],
    can_edit: false,
  }
}

describe("groupThreadPages", () => {
  it("returns nothing for no pages", () => {
    expect(groupThreadPages([])).toEqual([])
  })

  it("joins a thread that continues on the next page and orders by creation", () => {
    const first = {
      threads: [
        thread("t2", "2026-11-06T11:00:00Z"),
        thread("t1", "2026-11-06T10:00:00Z"),
      ],
      messages: [
        message("t1", "t1", "2026-11-06T10:00:00Z"),
        message("m3", "t2", "2026-11-06T11:00:00Z"),
      ],
    }
    const second = {
      threads: [thread("t1", "2026-11-06T10:00:00Z")],
      messages: [message("m2", "t1", "2026-11-06T10:30:00Z")],
    }
    const grouped = groupThreadPages([first, second])
    expect(grouped.map((entry) => entry.thread.id)).toEqual(["t1", "t2"])
    expect(grouped[0].messages.map((item) => item.id)).toEqual(["t1", "m2"])
    expect(grouped[1].messages.map((item) => item.id)).toEqual(["m3"])
  })

  it("keeps one copy of a message that two pages both carry", () => {
    const page = {
      threads: [thread("t1", "2026-11-06T10:00:00Z")],
      messages: [message("t1", "t1", "2026-11-06T10:00:00Z")],
    }
    const grouped = groupThreadPages([page, page])
    expect(grouped[0].messages).toHaveLength(1)
  })
})
