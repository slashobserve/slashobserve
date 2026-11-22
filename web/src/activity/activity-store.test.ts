import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"
import type { Chat } from "@/chat/chats-queries"

import { unusableChat } from "./activity-store"

async function storeAt(viewport: number, saved: object | null) {
  vi.resetModules()
  localStorage.clear()
  if (saved) {
    localStorage.setItem(
      "slashobserve-activity",
      JSON.stringify({ state: saved, version: 0 })
    )
  }
  vi.stubGlobal("innerWidth", viewport)
  const { useActivityStore } = await import("./activity-store")
  return useActivityStore.getState()
}

describe("the Activity pane's first state", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it("restores the saved open state on a desktop", async () => {
    expect((await storeAt(1440, { open: false })).open).toBe(false)
    expect((await storeAt(1440, { open: true })).open).toBe(true)
  })

  it("opens on a desktop with nothing saved", async () => {
    expect((await storeAt(1440, null)).open).toBe(true)
  })

  it("stays closed on a phone even when a desktop visit saved it open", async () => {
    expect((await storeAt(390, { open: true })).open).toBe(false)
    expect((await storeAt(900, { open: true })).open).toBe(false)
  })
})

describe("the remembered chat", () => {
  const dashboardId = "d1"
  const chat = {
    id: "c1",
    dashboard_id: dashboardId,
    read_only: false,
  } as Chat

  it("is unusable once it is gone or forbidden, and usable through other failures", () => {
    expect(
      unusableChat(
        new ApiError(404, "not_found", "gone"),
        undefined,
        dashboardId
      )
    ).toBe(true)
    expect(
      unusableChat(new ApiError(403, "forbidden", "no"), undefined, dashboardId)
    ).toBe(true)
    expect(
      unusableChat(
        new ApiError(503, "unavailable", "down"),
        undefined,
        dashboardId
      )
    ).toBe(false)
    expect(unusableChat(new TypeError("offline"), undefined, dashboardId)).toBe(
      false
    )
    expect(unusableChat(null, undefined, dashboardId)).toBe(false)
  })

  it("is unusable when it is read only or about another dashboard", () => {
    expect(unusableChat(null, chat, dashboardId)).toBe(false)
    expect(unusableChat(null, { ...chat, read_only: true }, dashboardId)).toBe(
      true
    )
    expect(
      unusableChat(null, { ...chat, dashboard_id: "d2" }, dashboardId)
    ).toBe(true)
  })

  it("is forgotten only while it is still the one remembered, and all at once on sign-out", async () => {
    vi.resetModules()
    const { useActivityStore } = await import("./activity-store")
    const store = useActivityStore.getState()
    store.setChat("d1", "c1")
    store.setChat("d2", "c2")
    useActivityStore.getState().forgetChat("d1", "c9")
    expect(useActivityStore.getState().chats).toEqual({ d1: "c1", d2: "c2" })
    useActivityStore.getState().forgetChat("d1", "c1")
    expect(useActivityStore.getState().chats).toEqual({ d2: "c2" })
    useActivityStore.getState().forgetChats()
    expect(useActivityStore.getState().chats).toEqual({})
  })
})
