import { afterEach, describe, expect, it, vi } from "vitest"

import { chatKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import { refreshChat, type Chat } from "./chats-queries"

const dashboardId = "11111111-1111-4111-8111-111111111111"

function chat(over: Partial<Chat> = {}): Chat {
  return {
    id: "c1",
    title: "Checkout health",
    dashboard_id: null,
    dashboard_title: null,
    observation_id: null,
    pinned_at: null,
    created_at: "2026-11-20T10:00:00Z",
    last_turn_at: "2026-11-20T10:05:00Z",
    turns: 1,
    last_turn: {
      id: "c1",
      state: "running",
      prompt: "x",
      investigation: false,
    },
    owner: { kind: "user", name: "Ada" },
    read_only: false,
    ...over,
  }
}

afterEach(() => vi.unstubAllGlobals())

describe("refreshChat", () => {
  it("replaces a chat cached before its build saved a dashboard", async () => {
    const queryClient = testQueryClient()
    queryClient.setQueryData(chatKeys.detail("c1"), {
      chat: chat(),
      turns: [],
    })
    const saved = chat({
      dashboard_id: dashboardId,
      dashboard_title: "Checkout health",
      last_turn: {
        id: "c1",
        state: "succeeded",
        prompt: "x",
        investigation: false,
      },
    })
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ chat: saved, turns: [] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
      )
    )
    await refreshChat(queryClient, "c1")
    expect(
      queryClient.getQueryData<{ chat: Chat }>(chatKeys.detail("c1"))?.chat
        .dashboard_id
    ).toBe(dashboardId)
  })
})
