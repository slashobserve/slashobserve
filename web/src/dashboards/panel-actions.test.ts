import { afterEach, describe, expect, it, vi } from "vitest"

import { useActivityStore } from "@/activity/activity-store"

import { planSend } from "@/chat/send-plan"

import { askAboutPanel, focusComment } from "./panel-actions"
import type { DashboardSearch } from "./search"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("focusComment", () => {
  it("opens the activity pane and focuses the box once it has mounted", () => {
    const frames: (() => void)[] = []
    vi.stubGlobal("requestAnimationFrame", (run: () => void) => {
      frames.push(run)
      return frames.length
    })
    useActivityStore.getState().setOpen(false)
    const field = {
      current: null as { focus: () => void } | null,
    }
    const focus = vi.fn()

    let search: DashboardSearch = { vars: {}, tab: "chat", chat: "c1" }
    focusComment((update) => {
      search = update(search)
    }, field as never)
    expect(useActivityStore.getState().open).toBe(true)
    expect(search).toMatchObject({ tab: "activity", chat: "c1" })
    frames.shift()?.()
    expect(focus).not.toHaveBeenCalled()
    field.current = { focus }
    frames.shift()?.()
    expect(focus).toHaveBeenCalledTimes(1)
    expect(frames).toHaveLength(0)
  })

  it("gives up when no box ever mounts", () => {
    const frames: (() => void)[] = []
    vi.stubGlobal("requestAnimationFrame", (run: () => void) => {
      frames.push(run)
      return frames.length
    })
    focusComment(() => undefined, { current: null })
    let guard = 0
    while (frames.length > 0 && guard < 100) {
      frames.shift()?.()
      guard += 1
    }
    expect(frames).toHaveLength(0)
    expect(guard).toBeLessThan(100)
  })
})

describe("askAboutPanel", () => {
  function asking(start: DashboardSearch, panelId: string) {
    let state = start
    askAboutPanel((update) => {
      state = update(state)
    }, panelId)
    return state
  }

  function requestFor(state: DashboardSearch) {
    const plan = planSend({
      command: "investigate",
      text: "why is this slow",
      dashboardId: "dashboard",
      panelId: state.panel,
    })
    if ("error" in plan) throw new Error(plan.error)
    return plan.input
  }

  it("keeps the panel as the target of the investigation request", () => {
    useActivityStore.getState().setOpen(false)
    useActivityStore.getState().setAgentKind("revise")
    const state = asking({ vars: {}, edit: "query" }, "latency")
    expect(useActivityStore.getState().open).toBe(true)
    expect(useActivityStore.getState().agentKind).toBe("investigate")
    expect(state).toMatchObject({ panel: "latency", edit: undefined })
    expect(requestFor(state)).toMatchObject({
      kind: "investigate",
      dashboardId: "dashboard",
      panelId: "latency",
    })
  })

  it("switches the side panel from the Chat tab to Activity", () => {
    const state = asking({ vars: {}, tab: "chat", chat: "c1" }, "latency")
    expect(state).toMatchObject({ tab: "activity", panel: "latency" })
  })

  it("moves the target when another panel is asked about next", () => {
    const state = asking(asking({ vars: {} }, "latency"), "errors")
    expect(requestFor(state).panelId).toBe("errors")
  })
})
