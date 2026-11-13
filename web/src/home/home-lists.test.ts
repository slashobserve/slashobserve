import { describe, expect, test } from "vitest"

import type { InboxItem } from "@/inbox/inbox-item"
import type { RequestSummary } from "@/observe/queries"

import {
  activeRuns,
  createdBy,
  needsYou,
  newestChats,
  pickUp,
} from "./home-lists"

function request(
  id: string,
  createdAt: string,
  state: RequestSummary["state"] = "succeeded",
  pinnedAt: string | null = null
): RequestSummary {
  return {
    id,
    state,
    created_at: createdAt,
    pinned_at: pinnedAt,
  } as RequestSummary
}

function inboxItem(seq: number, reason: InboxItem["reason"]): InboxItem {
  return { seq, reason } as InboxItem
}

function viewed(id: string, viewedAt: string) {
  return { id, title: id, viewed_at: viewedAt }
}

describe("newestChats", () => {
  test("puts a newer unpinned chat before an older pinned one", () => {
    const pinned = request(
      "pinned",
      "2026-11-01T10:00:00Z",
      "succeeded",
      "2026-11-02T10:00:00Z"
    )
    const newer = request("newer", "2026-11-10T10:00:00Z")
    expect(newestChats([pinned, newer], 5).map((r) => r.id)).toEqual([
      "newer",
      "pinned",
    ])
  })

  test("cuts the list to the limit", () => {
    const requests = ["a", "b", "c", "d"].map((id, index) =>
      request(id, `2026-11-0${index + 1}T10:00:00Z`)
    )
    expect(newestChats(requests, 2).map((r) => r.id)).toEqual(["d", "c"])
  })
})

describe("activeRuns", () => {
  test("keeps queued, running and canceling runs, newest first", () => {
    const requests = [
      request("queued", "2026-11-01T10:00:00Z", "queued"),
      request("succeeded", "2026-11-02T10:00:00Z", "succeeded"),
      request("running", "2026-11-03T10:00:00Z", "running"),
      request("failed", "2026-11-04T10:00:00Z", "failed"),
      request("canceling", "2026-11-05T10:00:00Z", "canceling"),
      request("canceled", "2026-11-06T10:00:00Z", "canceled"),
    ]
    expect(activeRuns(requests).map((r) => r.id)).toEqual([
      "canceling",
      "running",
      "queued",
    ])
  })
})

describe("needsYou", () => {
  test("drops items with no reason and cuts to the limit", () => {
    const items = [
      inboxItem(1, null),
      inboxItem(2, "mention"),
      inboxItem(3, "reply"),
      inboxItem(4, null),
      inboxItem(5, "proposal"),
      inboxItem(6, "run_failed"),
    ]
    expect(needsYou(items, 3).map((item) => item.seq)).toEqual([2, 3, 5])
  })
})

describe("createdBy", () => {
  test("keeps only the person's own items and cuts to the limit", () => {
    const items = [
      { id: "a", created_by: { id: "me" } },
      { id: "b", created_by: { id: "other" } },
      { id: "c", created_by: { id: "me" } },
      { id: "d", created_by: { id: "me" } },
    ]
    expect(createdBy(items, "me", 2).map((item) => item.id)).toEqual(["a", "c"])
  })
})

describe("pickUp", () => {
  test("interleaves dashboards and observations by when they were viewed", () => {
    const dashboards = [
      viewed("d1", "2026-11-06T12:00:00Z"),
      viewed("d2", "2026-11-06T08:00:00Z"),
      viewed("d3", "2026-11-05T08:00:00Z"),
    ]
    const observations = [
      viewed("o1", "2026-11-06T10:00:00Z"),
      viewed("o2", "2026-11-06T06:00:00Z"),
      viewed("o3", "2026-11-04T06:00:00Z"),
    ]
    expect(
      pickUp(dashboards, observations, 5).map((e) => [e.kind, e.id])
    ).toEqual([
      ["dashboard", "d1"],
      ["observation", "o1"],
      ["dashboard", "d2"],
      ["observation", "o2"],
      ["dashboard", "d3"],
    ])
  })
})
