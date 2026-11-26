import { describe, expect, test } from "vitest"

import {
  chatOptionMeta,
  chatWhen,
  dashboardChoices,
  dashboardLabel,
  subjectLabel,
  turnsText,
} from "./chat-rows"
import type { Chat } from "./chats-queries"

function chat(id: string, overrides: Partial<Chat> = {}): Chat {
  return {
    id,
    title: id,
    dashboard_id: null,
    dashboard_title: null,
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

function about(id: string, title: string | null): Partial<Chat> {
  return { dashboard_id: id, dashboard_title: title }
}

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
    expect(dashboardLabel(chat("a"))).toBeNull()
  })

  test("names the dashboard by the title the server gives", () => {
    expect(dashboardLabel(chat("a", about("d1", "Checkout health")))).toBe(
      "Checkout health"
    )
  })

  test("names a dashboard the server gives no title for", () => {
    expect(dashboardLabel(chat("a", about("d9", null)))).toBe(
      "A dashboard you cannot see"
    )
  })
})

describe("subjectLabel", () => {
  test("has none for a chat about nothing", () => {
    expect(subjectLabel(chat("a"))).toBeNull()
  })

  test("prefers the dashboard", () => {
    expect(
      subjectLabel({
        ...chat("a", about("d1", "Checkout health")),
        observation_id: "o1",
        observation_title: "Checkout errors",
      })
    ).toBe("Checkout health")
  })

  test("names the observation by its title", () => {
    expect(
      subjectLabel({
        ...chat("a", { observation_id: "o1" }),
        observation_title: "Checkout errors",
      })
    ).toBe("Checkout errors")
  })

  test("names an observation whose title is hidden or not given", () => {
    expect(
      subjectLabel({
        ...chat("a", { observation_id: "o1" }),
        observation_title: null,
      })
    ).toBe("An observation you cannot see")
    expect(subjectLabel(chat("a", { observation_id: "o1" }))).toBe(
      "An observation"
    )
  })
})

describe("dashboardChoices", () => {
  test("lists each dashboard once, by title, and skips chats about none", () => {
    expect(
      dashboardChoices(
        [
          chat("a", about("d1", "Checkout health")),
          chat("b"),
          chat("c", about("d2", "API latency")),
          chat("d", about("d1", "Checkout health")),
        ],
        undefined
      )
    ).toEqual([
      { id: "d2", label: "API latency" },
      { id: "d1", label: "Checkout health" },
    ])
  })

  test("keeps the chosen dashboard when no listed chat is about it", () => {
    expect(dashboardChoices([], "d1")).toEqual([
      { id: "d1", label: "Chosen dashboard" },
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
