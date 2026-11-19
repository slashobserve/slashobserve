import { afterEach, describe, expect, it, vi } from "vitest"

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
