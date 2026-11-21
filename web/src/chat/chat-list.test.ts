import { describe, expect, test } from "vitest"

import type { RequestSummary } from "@/observe/queries"

import {
  chatRoot,
  chatTurns,
  collapseChats,
  groupChats,
  newestChats,
} from "./chat-list"

function summary(
  overrides: Partial<RequestSummary> & { id: string }
): RequestSummary {
  return {
    intent: "apply",
    base_version: null,
    comment_id: null,
    trigger: "person",
    prompt: "Show checkout latency",
    status: "succeeded",
    state: "succeeded",
    result_kind: "none",
    allowed_actions: [],
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
    retried_by: null,
    continue_note: null,
    observation_id: null,
    observation_title: null,
    chat_id: overrides.id,
    origin: "web",
    origin_client: null,
    created_at: "2026-11-06T10:00:00Z",
    started_at: null,
    finished_at: null,
    requested_by: { kind: "user", id: "user-1", name: "Ada" },
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

describe("collapseChats", () => {
  test("shows one row per dashboard conversation, newest turn, titled by its first request", () => {
    const rows = collapseChats([
      summary({
        id: "q",
        intent: "apply",
        base_version: 1,
        dashboard_id: "a",
        title: "why is it red",
      }),
      summary({
        id: "e",
        intent: "apply",
        base_version: 1,
        dashboard_id: "a",
        title: "make it blue",
      }),
      summary({
        id: "b",
        intent: "apply",
        base_version: null,
        dashboard_id: "a",
        title: "Checkout health",
      }),
    ])
    expect(rows.map((row) => [row.id, row.title])).toEqual([
      ["q", "Checkout health"],
    ])
  })

  test("joins the turns of a chat with no dashboard by chat id", () => {
    const rows = collapseChats([
      summary({ id: "t2", chat_id: "t1", title: "and latency" }),
      summary({ id: "other" }),
      summary({ id: "t1", chat_id: "t1", title: "which logs exist" }),
    ])
    expect(rows.map((row) => [row.id, row.title])).toEqual([
      ["t2", "which logs exist"],
      ["other", "Show checkout latency"],
    ])
  })

  test("keeps a request that belongs to no chat in a row of its own", () => {
    const rows = collapseChats([
      summary({ id: "a", chat_id: null }),
      summary({ id: "b", chat_id: null }),
    ])
    expect(rows.map((row) => row.id)).toEqual(["a", "b"])
  })

  test("keeps a stopped request, its retry and later revisions of what the retry built in one row", () => {
    const rows = collapseChats([
      summary({
        id: "u",
        intent: "apply",
        base_version: 1,
        dashboard_id: "d",
        title: "make it blue",
      }),
      summary({
        id: "t",
        chat_id: "s",
        dashboard_id: "d",
        title: "Checkout health",
      }),
      summary({
        id: "s",
        chat_id: "s",
        state: "canceled",
        title: "Checkout health",
      }),
    ])
    expect(rows.map((row) => [row.id, row.title])).toEqual([
      ["u", "Checkout health"],
    ])
  })

  test("keeps investigations and comment revisions as their own rows", () => {
    const rows = collapseChats([
      summary({
        id: "i1",
        kind: "investigate",
        intent: "ask",
        dashboard_id: "a",
      }),
      summary({
        id: "i2",
        kind: "investigate",
        intent: "ask",
        dashboard_id: "a",
      }),
      summary({
        id: "c1",
        intent: "apply",
        base_version: 1,
        comment_id: "c1",
        dashboard_id: "a",
      }),
    ])
    expect(rows.map((row) => row.id)).toEqual(["i1", "i2", "c1"])
  })

  test("keeps pinned order by keeping the first row of each conversation", () => {
    const rows = collapseChats([
      summary({
        id: "p",
        intent: "apply",
        base_version: 1,
        dashboard_id: "a",
        pinned_at: "x",
      }),
      summary({
        id: "o",
        intent: "apply",
        base_version: null,
        dashboard_id: "a",
      }),
    ])
    expect(rows.map((row) => row.id)).toEqual(["p"])
  })
})

describe("chatTurns and chatRoot", () => {
  const list = [
    summary({ id: "t2", intent: "apply", base_version: 1, dashboard_id: "a" }),
    summary({ id: "other", dashboard_id: "b" }),
    summary({
      id: "t1",
      intent: "apply",
      base_version: null,
      dashboard_id: "a",
    }),
  ]

  test("gathers every turn of the conversation a row stands for", () => {
    expect(chatTurns(list, list[0]).map((turn) => turn.id)).toEqual([
      "t2",
      "t1",
    ])
  })

  test("takes a request outside any conversation as its own turn", () => {
    const run = summary({
      id: "i1",
      kind: "investigate",
      intent: "ask",
      dashboard_id: "a",
    })
    expect(chatTurns([...list, run], run)).toEqual([run])
  })

  test("names the first ask as the root of a conversation", () => {
    expect(chatRoot(chatTurns(list, list[0])).id).toBe("t1")
  })
})

describe("newestChats", () => {
  test("puts a newer unpinned chat before an older pinned one", () => {
    const pinned = summary({
      id: "pinned",
      created_at: "2026-11-01T10:00:00Z",
      pinned_at: "2026-11-02T10:00:00Z",
    })
    const newer = summary({ id: "newer", created_at: "2026-11-10T10:00:00Z" })
    expect(newestChats([pinned, newer], 5).map((r) => r.id)).toEqual([
      "newer",
      "pinned",
    ])
  })

  test("cuts the list to the limit", () => {
    const requests = ["a", "b", "c", "d"].map((id, index) =>
      summary({ id, created_at: `2026-11-0${index + 1}T10:00:00Z` })
    )
    expect(newestChats(requests, 2).map((r) => r.id)).toEqual(["d", "c"])
  })

  test("shows a dashboard's build, edit and question as one row named by the first build", () => {
    const dashboard = { dashboard_id: "a" }
    const rows = newestChats(
      [
        summary({
          id: "build",
          ...dashboard,
          intent: "apply",
          base_version: null,
          title: "Checkout health",
          created_at: "2026-11-01T10:00:00Z",
        }),
        summary({
          id: "edit",
          ...dashboard,
          intent: "apply",
          base_version: 1,
          title: "Rename a panel",
          created_at: "2026-11-01T11:00:00Z",
        }),
        summary({
          id: "question",
          ...dashboard,
          intent: "apply",
          base_version: 1,
          title: "What did you rename",
          created_at: "2026-11-01T12:00:00Z",
        }),
        summary({ id: "other", created_at: "2026-10-31T12:00:00Z" }),
      ],
      5
    )
    expect(rows.map((r) => [r.id, r.title])).toEqual([
      ["question", "Checkout health"],
      ["other", "Show checkout latency"],
    ])
  })

  test("fills the limit with distinct chats after collapsing", () => {
    const turns = ["t1", "t2", "t3"].map((id, index) =>
      summary({
        id,
        intent: "apply",
        base_version: 1,
        dashboard_id: "a",
        created_at: `2026-11-0${index + 1}T10:00:00Z`,
      })
    )
    const other = summary({ id: "o", created_at: "2026-10-01T10:00:00Z" })
    expect(newestChats([...turns, other], 2).map((r) => r.id)).toEqual([
      "t3",
      "o",
    ])
  })
})
