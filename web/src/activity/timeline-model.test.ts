import { describe, expect, it } from "vitest"

import type { Comment } from "@/comments/queries"
import type { VersionMeta } from "@/history/use-version-history"
import type { Revision } from "@/revisions/queries"

import {
  buildTimeline,
  currentStep,
  filterTimeline,
  type TimelineEntry,
} from "./timeline-model"

const author = { kind: "user", id: "u1", name: "Ada" } as const

function at(minutes: number, ms = 0): string {
  return new Date(Date.UTC(2026, 0, 1, 0, minutes, 0, ms)).toISOString()
}

function comment(overrides: Partial<Comment> & { id: string }): Comment {
  return {
    dashboard_id: "d1",
    parent_id: null,
    version: 1,
    panel_id: null,
    body: "text",
    status: "open",
    author,
    created_at: at(0),
    updated_at: at(0),
    resolved_at: null,
    resolved_by: null,
    resolved_by_version: null,
    panel_present: true,
    revision: null,
    ...overrides,
  }
}

function request(overrides: Partial<Revision> & { id: string }): Revision {
  return {
    dashboard_id: "d1",
    source: "prompt",
    comment_id: null,
    panel_id: null,
    prompt: "do it",
    base_version: 1,
    status: "succeeded",
    attempts: 1,
    result_version: null,
    error: null,
    requested_by: author,
    origin: "web",
    origin_client: null,
    created_at: at(0),
    started_at: null,
    finished_at: null,
    ...overrides,
  }
}

function version(n: number, createdAt: string): VersionMeta {
  return {
    version: n,
    parent_version: n > 1 ? n - 1 : null,
    restored_from: null,
    summary: `v${n}`,
    author,
    created_at: createdAt,
    origin: "web",
    origin_client: null,
    agent_request_id: null,
  }
}

function keys(entries: readonly TimelineEntry[]): string[] {
  return entries.map((entry) => entry.key)
}

const none = { comments: [], requests: [], versions: [] }

describe("buildTimeline", () => {
  it("orders every kind by time, oldest first", () => {
    const entries = buildTimeline({
      comments: [comment({ id: "c1", created_at: at(2) })],
      requests: [
        request({ id: "r1", created_at: at(3) }),
        request({
          id: "r2",
          created_at: at(5),
          status: "running",
          started_at: at(5),
        }),
      ],
      versions: [version(2, at(4)), version(1, at(1))],
    })
    expect(keys(entries)).toEqual([
      "version:1",
      "thread:c1",
      "request:r1",
      "version:2",
      "request:r2",
      "running:r2",
    ])
  })

  it("never turns a reply into its own entry", () => {
    const entries = buildTimeline({
      ...none,
      comments: [
        comment({ id: "c1", created_at: at(1) }),
        comment({ id: "c2", parent_id: "c1", created_at: at(2) }),
      ],
    })
    expect(keys(entries)).toEqual(["thread:c1"])
    const [entry] = entries
    expect(entry?.kind === "thread" && entry.thread.replies).toHaveLength(1)
  })

  it("attaches the newest thread request to its thread only", () => {
    const entries = buildTimeline({
      ...none,
      comments: [comment({ id: "c1", created_at: at(1) })],
      requests: [
        request({
          id: "old",
          source: "comment",
          comment_id: "c1",
          status: "failed",
          created_at: at(2),
        }),
        request({
          id: "new",
          source: "comment",
          comment_id: "c1",
          created_at: at(3),
        }),
      ],
    })
    expect(keys(entries)).toEqual(["thread:c1"])
    const [entry] = entries
    expect(entry?.kind === "thread" && entry.request?.id).toBe("new")
  })

  it("shows a request whose thread is not loaded as a request entry", () => {
    const entries = buildTimeline({
      ...none,
      comments: [comment({ id: "c1", created_at: at(1) })],
      requests: [
        request({
          id: "r1",
          source: "comment",
          comment_id: "other",
          created_at: at(2),
        }),
      ],
    })
    expect(keys(entries)).toEqual(["thread:c1", "request:r1"])
    const [entry] = entries
    expect(entry?.kind === "thread" && entry.request).toBeNull()
  })

  it("leaves the observe request out and keeps its version", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({ id: "o1", source: "observe", base_version: null }),
        request({ id: "p1", created_at: at(2) }),
      ],
      versions: [version(1, at(1))],
    })
    expect(keys(entries)).toEqual(["version:1", "request:p1"])
  })

  it("shows a running entry for any active request, thread requests included", () => {
    const entries = buildTimeline({
      ...none,
      comments: [comment({ id: "c1", created_at: at(1) })],
      requests: [
        request({
          id: "t1",
          source: "comment",
          comment_id: "c1",
          status: "queued",
          created_at: at(2),
        }),
        request({
          id: "o1",
          source: "observe",
          status: "running",
          created_at: at(3),
          started_at: at(4),
        }),
        request({ id: "done", status: "succeeded", created_at: at(5) }),
      ],
    })
    expect(keys(entries)).toEqual([
      "thread:c1",
      "running:t1",
      "running:o1",
      "request:done",
    ])
  })

  it("puts a running entry one millisecond after its start", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({
          id: "r1",
          status: "running",
          created_at: at(1),
          started_at: at(2),
        }),
        request({ id: "r2", status: "queued", created_at: at(3) }),
      ],
    })
    const running = entries.filter((entry) => entry.kind === "running")
    expect(running.map((entry) => entry.at)).toEqual([at(2, 1), at(3, 1)])
  })

  it("sorts a running entry after its own request", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({
          id: "r1",
          status: "running",
          created_at: at(1),
          started_at: at(1),
        }),
      ],
      versions: [version(1, at(1, 1))],
    })
    expect(keys(entries)).toEqual(["request:r1", "running:r1", "version:1"])
  })

  it("orders ties by kind, then key", () => {
    const entries = buildTimeline({
      comments: [
        comment({ id: "b", created_at: at(1) }),
        comment({ id: "a", created_at: at(1) }),
      ],
      requests: [request({ id: "r1", created_at: at(1) })],
      versions: [version(2, at(1)), version(1, at(1))],
    })
    expect(keys(entries)).toEqual([
      "thread:a",
      "thread:b",
      "request:r1",
      "version:1",
      "version:2",
    ])
  })

  it("orders versions that share a time by number, not by text", () => {
    const entries = buildTimeline({
      ...none,
      versions: [version(10, at(1)), version(2, at(1)), version(9, at(1))],
    })
    expect(keys(entries)).toEqual(["version:2", "version:9", "version:10"])
  })

  it("sorts unparseable dates first and never throws", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({
          id: "bad",
          status: "running",
          created_at: "not a date",
          started_at: null,
        }),
        request({ id: "ok", created_at: at(1) }),
      ],
      versions: [version(1, "")],
    })
    expect(keys(entries)).toEqual([
      "request:bad",
      "running:bad",
      "version:1",
      "request:ok",
    ])
    expect(entries.find((entry) => entry.key === "running:bad")?.at).toBe(
      "not a date"
    )
  })

  it("picks the thread request by a parsed date, not by text", () => {
    const [entry] = buildTimeline({
      ...none,
      comments: [comment({ id: "c1" })],
      requests: [
        request({
          id: "later",
          source: "comment",
          comment_id: "c1",
          created_at: "2026-01-01T01:00:00+01:00",
        }),
        request({
          id: "earlier",
          source: "comment",
          comment_id: "c1",
          created_at: "2026-01-01T00:30:00Z",
        }),
      ],
    })
    expect(entry?.kind === "thread" && entry.request?.id).toBe("earlier")
  })

  it("carries the panel of each entry", () => {
    const entries = buildTimeline({
      comments: [comment({ id: "c1", panel_id: "p1" })],
      requests: [request({ id: "r1", panel_id: "p2", created_at: at(1) })],
      versions: [version(1, at(2))],
    })
    expect(entries.map((entry) => entry.panelId)).toEqual(["p1", "p2", null])
  })
})

describe("filterTimeline", () => {
  const entries = buildTimeline({
    comments: [
      comment({ id: "open-p1", panel_id: "p1", created_at: at(1) }),
      comment({
        id: "done-p2",
        panel_id: "p2",
        status: "resolved",
        created_at: at(2),
      }),
      comment({ id: "open-all", created_at: at(3) }),
    ],
    requests: [
      request({ id: "r-p1", panel_id: "p1", created_at: at(4) }),
      request({
        id: "r-p2",
        panel_id: "p2",
        status: "running",
        created_at: at(5),
        started_at: at(5),
      }),
    ],
    versions: [version(1, at(6))],
  })

  it("keeps everything", () => {
    expect(filterTimeline(entries, "everything", null)).toEqual(entries)
  })

  it("keeps only open threads", () => {
    expect(keys(filterTimeline(entries, "open", null))).toEqual([
      "thread:open-p1",
      "thread:open-all",
    ])
  })

  it("keeps requests, running entries and versions as changes", () => {
    expect(keys(filterTimeline(entries, "changes", null))).toEqual([
      "request:r-p1",
      "request:r-p2",
      "running:r-p2",
      "version:1",
    ])
  })

  it("keeps a thread that has an agent request, whatever its status, as a change", () => {
    const withRequests = buildTimeline({
      ...none,
      comments: [
        comment({ id: "asked", created_at: at(1) }),
        comment({ id: "failed", created_at: at(2) }),
        comment({ id: "plain", created_at: at(3) }),
      ],
      requests: [
        request({
          id: "r1",
          source: "comment",
          comment_id: "asked",
          created_at: at(4),
        }),
        request({
          id: "r2",
          source: "comment",
          comment_id: "failed",
          status: "failed",
          created_at: at(5),
        }),
      ],
    })
    expect(keys(filterTimeline(withRequests, "changes", null))).toEqual([
      "thread:asked",
      "thread:failed",
    ])
  })

  it("drops other panels and every version when scoped to a panel", () => {
    expect(keys(filterTimeline(entries, "everything", "p1"))).toEqual([
      "thread:open-p1",
      "request:r-p1",
    ])
    expect(keys(filterTimeline(entries, "changes", "p2"))).toEqual([
      "request:r-p2",
      "running:r-p2",
    ])
  })

  it("combines the show filter with the panel scope", () => {
    expect(keys(filterTimeline(entries, "open", "p2"))).toEqual([])
    expect(keys(filterTimeline(entries, "open", "p1"))).toEqual([
      "thread:open-p1",
    ])
  })
})

describe("currentStep", () => {
  const step = (attempt: number, minutes: number) => ({
    attempt,
    at: at(minutes),
  })

  it("is zero without steps", () => {
    expect(currentStep([])).toBe(0)
  })

  it("counts every step of a single attempt", () => {
    expect(currentStep([step(1, 1), step(1, 2), step(1, 3)])).toBe(3)
  })

  it("ignores earlier failed attempts", () => {
    expect(currentStep([step(1, 1), step(1, 2), step(2, 3), step(2, 4)])).toBe(
      2
    )
  })

  it("starts over when a re-claimed run reuses the attempt number", () => {
    expect(currentStep([step(1, 5), step(1, 6), step(1, 1), step(1, 2)])).toBe(
      2
    )
  })

  it("keeps steps that share a timestamp together", () => {
    expect(currentStep([step(1, 1), step(1, 1), step(1, 1)])).toBe(3)
  })
})
