import { afterEach, describe, expect, it, vi } from "vitest"

import { useActivityStore } from "@/activity/activity-store"

import { focusComment } from "./panel-actions"

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

    focusComment(field as never)
    expect(useActivityStore.getState().open).toBe(true)
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
    focusComment({ current: null })
    let guard = 0
    while (frames.length > 0 && guard < 100) {
      frames.shift()?.()
      guard += 1
    }
    expect(frames).toHaveLength(0)
    expect(guard).toBeLessThan(100)
  })
})
