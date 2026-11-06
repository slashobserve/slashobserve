import { describe, expect, test } from "vitest"

import type { RequestSummary } from "@/observe/queries"

import { groupChats } from "./chat-list"

function summary(
  overrides: Partial<RequestSummary> & { id: string }
): RequestSummary {
  return {
    source: "observe",
    prompt: "Show checkout latency",
    status: "succeeded",
    dashboard_id: null,
    dashboard_title: null,
    panel_id: null,
    result_version: null,
    url: null,
    error: null,
    failure_code: null,
    build: null,
    build_saved_at: null,
    kind: "build",
    title: "Show checkout latency",
    pinned_at: null,
    continues_id: null,
    continued_by: null,
    continue_note: null,
    origin: "web",
    origin_client: null,
    created_at: "2026-11-06T10:00:00Z",
    started_at: null,
    finished_at: null,
    ...overrides,
  }
}

function shape(requests: readonly RequestSummary[]) {
  return groupChats(requests).map((group) => ({
    label: group.label,
    dashboardId: group.dashboardId,
    ids: group.requests.map((request) => request.id),
  }))
}

describe("groupChats", () => {
  test("returns no groups for no chats", () => {
    expect(groupChats([])).toEqual([])
  })

  test("groups by dashboard in order of first appearance", () => {
    expect(
      shape([
        summary({ id: "a1", dashboard_id: "a", dashboard_title: "Checkout" }),
        summary({ id: "n1" }),
        summary({ id: "b1", dashboard_id: "b", dashboard_title: "Payments" }),
        summary({ id: "a2", dashboard_id: "a", dashboard_title: "Checkout" }),
        summary({ id: "n2" }),
      ])
    ).toEqual([
      { label: "Checkout", dashboardId: "a", ids: ["a1", "a2"] },
      { label: "No dashboard", dashboardId: null, ids: ["n1", "n2"] },
      { label: "Payments", dashboardId: "b", ids: ["b1"] },
    ])
  })

  test("puts pinned chats first, in server order, and only there", () => {
    expect(
      shape([
        summary({
          id: "p2",
          dashboard_id: "a",
          dashboard_title: "Checkout",
          pinned_at: "2026-11-06T09:00:00Z",
        }),
        summary({ id: "p1", pinned_at: "2026-11-05T09:00:00Z" }),
        summary({ id: "a1", dashboard_id: "a", dashboard_title: "Checkout" }),
      ])
    ).toEqual([
      { label: "Pinned", dashboardId: null, ids: ["p2", "p1"] },
      { label: "Checkout", dashboardId: "a", ids: ["a1"] },
    ])
  })

  test("names a dashboard whose title is hidden", () => {
    expect(
      shape([summary({ id: "h1", dashboard_id: "h", dashboard_title: null })])
    ).toEqual([
      { label: "A dashboard you cannot see", dashboardId: "h", ids: ["h1"] },
    ])
  })

  test("keeps only the pinned group when every chat is pinned", () => {
    expect(
      shape([summary({ id: "p1", pinned_at: "2026-11-06T09:00:00Z" })])
    ).toEqual([{ label: "Pinned", dashboardId: null, ids: ["p1"] }])
  })
})
