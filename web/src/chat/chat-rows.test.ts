import { describe, expect, test } from "vitest"

import {
  chatOptionMeta,
  chatWhen,
  dashboardChoices,
  dashboardLabel,
  pinnedFirst,
  turnsText,
} from "./chat-rows"
import type { Chat } from "./chats-queries"

function chat(id: string, overrides: Partial<Chat> = {}): Chat {
  return {
    id,
    title: id,
    dashboard_id: null,
    observation_id: null,
    pinned_at: null,
    created_at: "2026-11-21T09:00:00",
    last_turn_at: "2026-11-21T10:00:00",
    turns: 1,
    last_turn: null,
    owner: { kind: "user", name: "Ada" },
    read_only: false,
    ...overrides,
  }
}

const titles = new Map([
  ["d1", "Checkout health"],
  ["d2", "API latency"],
])

describe("turnsText", () => {
  test.each([
    [1, "1 turn"],
    [2, "2 turns"],
    [1200, "1,200 turns"],
  ])("%i reads %s", (turns, text) => {
    expect(turnsText(turns)).toBe(text)
  })
})

describe("dashboardLabel", () => {
  test("has none for a chat about no dashboard", () => {
    expect(dashboardLabel(null, titles)).toBeNull()
  })

  test("names the dashboard by its title", () => {
    expect(dashboardLabel("d1", titles)).toBe("Checkout health")
  })

  test("names a dashboard the reader cannot see without its title", () => {
    expect(dashboardLabel("d9", titles)).toBe("A dashboard you cannot see")
  })

  test("says Dashboard while the titles load", () => {
    expect(dashboardLabel("d1", undefined)).toBe("Dashboard")
  })
})

describe("pinnedFirst", () => {
  test("moves pinned chats ahead, most recently pinned first, and keeps the rest in order", () => {
    const ordered = pinnedFirst([
      chat("a"),
      chat("p-old", { pinned_at: "2026-11-01T00:00:00Z" }),
      chat("b"),
      chat("p-new", { pinned_at: "2026-11-10T00:00:00Z" }),
      chat("c"),
    ])
    expect(ordered.map((entry) => entry.id)).toEqual([
      "p-new",
      "p-old",
      "a",
      "b",
      "c",
    ])
  })
})

describe("dashboardChoices", () => {
  test("lists each dashboard once, by title, and skips chats about none", () => {
    expect(
      dashboardChoices(
        [
          chat("a", { dashboard_id: "d1" }),
          chat("b"),
          chat("c", { dashboard_id: "d2" }),
          chat("d", { dashboard_id: "d1" }),
        ],
        titles,
        undefined
      )
    ).toEqual([
      { id: "d2", label: "API latency" },
      { id: "d1", label: "Checkout health" },
    ])
  })

  test("keeps the chosen dashboard when no listed chat is about it", () => {
    expect(dashboardChoices([], titles, "d1")).toEqual([
      { id: "d1", label: "Checkout health" },
    ])
  })
})

describe("chatWhen", () => {
  const now = new Date(2026, 10, 21, 15, 0).getTime()

  test.each([
    [new Date(2026, 10, 21, 14, 12), "Today 14:12"],
    [new Date(2026, 10, 20, 17, 40), "Yesterday 17:40"],
    [new Date(2026, 10, 16, 11, 5), "Mon 11:05"],
    [new Date(2026, 10, 14, 9, 41), "Nov 14 09:41"],
  ])("%s reads %s", (at, text) => {
    expect(chatWhen(at.toISOString(), now)).toBe(text)
  })

  test("joins the time and the turns for a chat option", () => {
    expect(
      chatOptionMeta(
        chat("a", {
          last_turn_at: new Date(2026, 10, 21, 14, 12).toISOString(),
          turns: 2,
        }),
        now
      )
    ).toBe("Today 14:12 · 2 turns")
  })
})
