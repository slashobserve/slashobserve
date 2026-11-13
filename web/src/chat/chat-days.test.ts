import { describe, expect, test } from "vitest"

import type { RequestSummary } from "@/observe/queries"

import { groupChatsByDay } from "./chat-days"

function summary(overrides: Partial<RequestSummary> = {}): RequestSummary {
  return {
    id: "00000000-0000-4000-8000-000000000001",
    requested_by: {
      kind: "user",
      id: "00000000-0000-4000-8000-0000000000a1",
      name: "Ada Lovelace",
    },
    source: "observe",
    trigger: "person",
    prompt: "checkout health",
    status: "succeeded",
    state: "succeeded",
    result_kind: "version",
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
    title: "Checkout health",
    pinned_at: null,
    continues_id: null,
    continued_by: null,
    continue_note: null,
    observation_id: null,
    origin: "web",
    origin_client: null,
    created_at: new Date(2026, 10, 13, 9).toISOString(),
    started_at: null,
    finished_at: null,
    ...overrides,
  }
}

function at(day: number, hour: number, minute = 0): string {
  return new Date(2026, 10, day, hour, minute).toISOString()
}

const now = new Date(2026, 10, 13, 15)

function shape(groups: ReturnType<typeof groupChatsByDay>) {
  return groups.map((group) => [
    group.label,
    group.requests.map((request) => request.id),
  ])
}

describe("groupChatsByDay", () => {
  test("splits chats at local midnight today and six days earlier", () => {
    const requests = [
      summary({ id: "today", created_at: at(13, 0, 1) }),
      summary({ id: "yesterday", created_at: at(12, 23, 59) }),
      summary({ id: "six-days", created_at: at(7, 0, 0) }),
      summary({ id: "seven-days", created_at: at(6, 23, 59) }),
    ]
    expect(shape(groupChatsByDay(requests, now, false))).toEqual([
      ["Today", ["today"]],
      ["This week", ["yesterday", "six-days"]],
      ["Earlier", ["seven-days"]],
    ])
  })

  test("puts a pinned chat in Pinned first only when asked to", () => {
    const requests = [
      summary({ id: "new", created_at: at(13, 10) }),
      summary({
        id: "old-pinned",
        created_at: at(1, 10),
        pinned_at: at(2, 10),
      }),
    ]
    expect(shape(groupChatsByDay(requests, now, true))).toEqual([
      ["Pinned", ["old-pinned"]],
      ["Today", ["new"]],
    ])
    expect(shape(groupChatsByDay(requests, now, false))).toEqual([
      ["Today", ["new"]],
      ["Earlier", ["old-pinned"]],
    ])
  })

  test("keeps the input order inside a group", () => {
    const requests = [
      summary({ id: "b", created_at: at(13, 12) }),
      summary({ id: "a", created_at: at(13, 8) }),
    ]
    expect(shape(groupChatsByDay(requests, now, true))).toEqual([
      ["Today", ["b", "a"]],
    ])
  })

  test("returns no groups for no chats", () => {
    expect(groupChatsByDay([], now, true)).toEqual([])
  })
})
