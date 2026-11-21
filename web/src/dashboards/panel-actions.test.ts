import { afterEach, describe, expect, it, vi } from "vitest"

import { useActivityStore } from "@/activity/activity-store"

import type { LayoutPanel } from "@/spec"

import {
  askAboutThisPanel,
  commentOnThisPanel,
  conversationActionsFor,
  focusComment,
} from "./panel-actions"
import type { DashboardSearch } from "./search"
import { useCanvasTool } from "./canvas-tools"

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

describe("askAboutThisPanel", () => {
  it("opens the ask box on the whole panel on the canvas", () => {
    let search: DashboardSearch = { vars: {} }
    const place = askAboutThisPanel(
      (update) => {
        search = update(search)
      },
      "latency",
      true
    )
    expect(place).toBe("canvas")
    expect(search).toEqual({ vars: {} })
    expect(useCanvasTool.getState().draft).toEqual({
      ownerId: "latency",
      tool: "ask",
      anchor: null,
      at: null,
      surface: null,
    })
    useCanvasTool.getState().close()
  })

  it("opens the Chat tab off the canvas on the panel, keeping the open chat", () => {
    useActivityStore.getState().setOpen(false)
    let search: DashboardSearch = {
      vars: {},
      tab: "activity",
      chat: "c1",
      edit: "query",
    }
    const place = askAboutThisPanel(
      (update) => {
        search = update(search)
      },
      "latency",
      false
    )
    expect(place).toBe("pane")
    expect(useActivityStore.getState().open).toBe(true)
    expect(search).toMatchObject({ tab: "chat", chat: "c1", panel: "latency" })
    expect(search.edit).toBeUndefined()
    expect(useCanvasTool.getState().draft).toBeNull()
  })
})

const dataPanel: LayoutPanel = {
  id: "p1",
  type: "timeseries",
  title: "Latency",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  datasource: "prom",
  queries: [{ query: "up" }],
}

const textPanel: LayoutPanel = {
  id: "t1",
  type: "text",
  title: "Notes",
  gridPos: { x: 0, y: 0, w: 12, h: 6 },
  content: "hello",
} as LayoutPanel

describe("conversationActionsFor", () => {
  it.each([
    { panel: dataPanel, canComment: true, agent: true, want: [true, true] },
    { panel: dataPanel, canComment: true, agent: false, want: [true, false] },
    { panel: dataPanel, canComment: false, agent: true, want: [false, false] },
    { panel: textPanel, canComment: true, agent: true, want: [true, false] },
  ])(
    "gives comment and ask $want for $panel.type with comment $canComment and agent $agent",
    ({ panel, canComment, agent, want }) => {
      const actions = conversationActionsFor(panel, canComment, agent)
      expect([actions.comment, actions.ask]).toEqual(want)
    }
  )
})

describe("acting on a panel and focusing the composer", () => {
  function frames() {
    const queue: (() => void)[] = []
    vi.stubGlobal("requestAnimationFrame", (run: () => void) => {
      queue.push(run)
      return queue.length
    })
    return queue
  }

  it("comments on the panel in the activity pane and focuses the composer off the canvas", () => {
    const queue = frames()
    const focus = vi.fn()
    let search: DashboardSearch = { vars: {} }
    const place = commentOnThisPanel(
      (update: (current: DashboardSearch) => DashboardSearch) => {
        search = update(search)
      },
      "latency",
      false,
      { current: { focus } as never }
    )
    expect(place).toBe("pane")
    expect(search).toMatchObject({ tab: "activity", panel: "latency" })
    expect(useCanvasTool.getState().draft).toBeNull()
    queue.shift()?.()
    expect(focus).toHaveBeenCalledTimes(1)
  })

  it("opens the canvas composer on the whole panel when the panel takes canvas comments", () => {
    const queue = frames()
    const focus = vi.fn()
    let search: DashboardSearch = { vars: {} }
    const place = commentOnThisPanel(
      (update: (current: DashboardSearch) => DashboardSearch) => {
        search = update(search)
      },
      "latency",
      true,
      { current: { focus } as never }
    )
    expect(place).toBe("canvas")
    expect(search).toEqual({ vars: {} })
    expect(useCanvasTool.getState().draft).toEqual({
      ownerId: "latency",
      tool: "comment",
      anchor: null,
      at: null,
      surface: null,
    })
    queue.shift()?.()
    expect(focus).not.toHaveBeenCalled()
    useCanvasTool.getState().close()
  })
})
